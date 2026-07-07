// Package tui implements rest-tui's full-screen terminal application: a
// Browser screen for finding .http files and requests, a Request view for
// resolving variables and sending a request, and a History screen for
// reviewing and re-running past executions.
package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Ahngbeom/rest-tui/internal/dirhistory"
	"github.com/Ahngbeom/rest-tui/internal/history"
	"github.com/Ahngbeom/rest-tui/internal/httpfile"
)

const (
	headerHeight = 1
	footerHeight = 1
)

type screen int

const (
	screenBrowser screen = iota
	screenRequest
	screenHistory
	screenDirHistory
)

// App is the root bubbletea model. It owns the current screen and delegates
// messages to whichever screen is active.
type App struct {
	screen        screen
	width, height int

	browser    browserModel
	request    requestModel
	history    historyModel
	dirHistory dirHistoryModel

	store    *history.Store
	dirStore *dirhistory.Store
	root     string

	showHelp  bool
	statusMsg string
}

// NewApp builds the root model. root is the directory .http files are
// discovered under; store is where executed requests are recorded; dirStore
// is where directories passed via -dir are recorded, so they can be browsed
// and switched to later from the Directory History screen.
func NewApp(root string, store *history.Store, dirStore *dirhistory.Store) App {
	a := App{root: root, store: store, dirStore: dirStore}
	// Recording failures (e.g. an unwritable config dir) shouldn't stop the
	// app from starting -- directory history is a convenience, not core
	// functionality.
	_, _ = dirStore.Touch(root)
	a.browser = newBrowserModel(root, store)
	a.request = newRequestModel("", &httpfile.File{}, httpfile.Request{}, store)
	a.history = newHistoryModel(store)
	a.dirHistory = newDirHistoryModel(dirStore)
	return a
}

func (a App) Init() tea.Cmd {
	return nil
}

// activeScreenIsEditing reports whether the active screen is currently
// capturing raw text input (the Request view's raw-text request editor), in
// which case single-character global shortcuts (h/d/q/?) must be routed to
// the screen's own Update instead of being intercepted here.
func (a App) activeScreenIsEditing() bool {
	return a.screen == screenRequest && a.request.isEditing()
}

func (a App) contentHeight() int {
	h := a.height - headerHeight - footerHeight
	if h < 0 {
		h = 0
	}
	return h
}

// applyStoreWarning copies any newly-detected history or directory-history
// store warning (e.g. a corrupted file being backed up) into the status
// line. Corruption is only discovered lazily, the first time something
// actually reads the file, so this is checked here rather than once at
// startup.
func (a App) applyStoreWarning() App {
	if w := a.store.Warning(); w != "" {
		a.statusMsg = w
	}
	if w := a.dirStore.Warning(); w != "" {
		a.statusMsg = w
	}
	return a
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		ch := a.contentHeight()
		a.browser = a.browser.SetSize(msg.Width, ch)
		a.request = a.request.SetSize(msg.Width, ch)
		a.history = a.history.SetSize(msg.Width, ch)
		a.dirHistory = a.dirHistory.SetSize(msg.Width, ch)
		return a, nil

	case tea.KeyMsg:
		if !a.activeScreenIsEditing() {
			switch {
			case key.Matches(msg, keys.Quit):
				return a, tea.Quit
			case key.Matches(msg, keys.Help):
				a.showHelp = !a.showHelp
				return a, nil
			case key.Matches(msg, keys.ToggleLog):
				a.screen = screenHistory
				a.history = a.history.refresh()
				return a.applyStoreWarning(), nil
			case key.Matches(msg, keys.Directories):
				a.screen = screenDirHistory
				a.dirHistory = a.dirHistory.refresh()
				return a.applyStoreWarning(), nil
			}
		}

	case openRequestMsg:
		a.request = newRequestModel(msg.filePath, msg.file, msg.req, a.store)
		a.request = a.request.SetSize(a.width, a.contentHeight())
		a.screen = screenRequest
		return a, nil

	case rerunMsg:
		var cmd tea.Cmd
		a.request, cmd = newRequestModelFromEntry(msg.entry, a.store)
		a.request = a.request.SetSize(a.width, a.contentHeight())
		a.screen = screenRequest
		return a, cmd

	case openRequestFromEntryMsg:
		a.request = newRequestModelFromEntryNoSend(msg.entry, a.store)
		a.request = a.request.SetSize(a.width, a.contentHeight())
		a.screen = screenRequest
		return a, nil

	case backToBrowserMsg:
		a.screen = screenBrowser
		if a.browser.parsedFile == nil {
			a.browser = a.browser.refreshRecent()
		}
		return a, nil

	case openHistoryMsg:
		a.screen = screenHistory
		a.history = a.history.refresh()
		return a.applyStoreWarning(), nil

	case switchDirMsg:
		a.root = msg.path
		_, _ = a.dirStore.Touch(msg.path)
		a.browser = newBrowserModel(msg.path, a.store)
		a.browser = a.browser.SetSize(a.width, a.contentHeight())
		a.screen = screenBrowser
		return a.applyStoreWarning(), nil
	}

	var cmd tea.Cmd
	switch a.screen {
	case screenBrowser:
		a.browser, cmd = a.browser.Update(msg)
	case screenRequest:
		a.request, cmd = a.request.Update(msg)
	case screenHistory:
		a.history, cmd = a.history.Update(msg)
	case screenDirHistory:
		a.dirHistory, cmd = a.dirHistory.Update(msg)
	}
	return a.applyStoreWarning(), cmd
}

func (a App) breadcrumb() string {
	switch a.screen {
	case screenRequest:
		return "rest-tui › Browser › " + a.request.title()
	case screenHistory:
		if a.history.mode == historyModeDetail {
			return "rest-tui › History › detail"
		}
		return "rest-tui › History"
	case screenDirHistory:
		return "rest-tui › Directories"
	default:
		return "rest-tui › Browser (" + a.root + ")"
	}
}

func (a App) footerHints() string {
	switch a.screen {
	case screenRequest:
		if a.request.isEditing() {
			return "ctrl+s apply  esc cancel"
		}
		return "e cycle env  enter/ctrl+r send  ↑/↓ scroll  c copy  i edit  esc back  h history  d dirs  ? help  q quit"
	case screenHistory:
		if a.history.mode == historyModeDetail {
			return "↑/↓ scroll  r rerun  c copy  esc back  ? help  q quit"
		}
		return "↑/↓ move  enter detail  r rerun  esc back  ? help  q quit"
	case screenDirHistory:
		return "↑/↓ move  enter switch  esc back  ? help  q quit"
	default:
		if a.browser.focus == paneRequests {
			return "↑/↓ move  enter run  tab← files  esc back  h history  d dirs  ? help  q quit"
		}
		return "↑/↓ move  enter open  tab→ requests  h history  d dirs  ? help  q quit"
	}
}

func (a App) helpView() string {
	lines := []string{
		"Global",
		"  q, ctrl+c     quit",
		"  h             jump to History",
		"  d             jump to Directory History",
		"  ?             toggle this help",
		"  esc           back",
		"",
		"Browser",
		"  ↑/↓, j/k      move selection",
		"  tab           switch files/requests pane",
		"  enter         open file / run request",
		"",
		"Request view",
		"  e             cycle environment",
		"  enter, ctrl+r send request",
		"  ↑/↓           scroll response",
		"  c, y          copy request/response",
		"  i             edit request (method/URL/headers/body)",
		"  ctrl+s, esc   apply / cancel edit (while editing)",
		"",
		"History",
		"  enter         view detail",
		"  r             rerun",
		"  c, y          copy request/response (detail view)",
		"",
		"Directory History",
		"  enter         switch to selected directory",
	}
	return paneFocusedStyle.Render(strings.Join(lines, "\n"))
}

func (a App) View() string {
	header := headerStyle.Render(a.breadcrumb())

	var body string
	switch {
	case a.showHelp:
		body = a.helpView()
	case a.screen == screenRequest:
		body = a.request.View()
	case a.screen == screenHistory:
		body = a.history.View()
	case a.screen == screenDirHistory:
		body = a.dirHistory.View()
	default:
		body = a.browser.View()
	}

	footer := footerStyle.Render(a.footerHints())
	if a.statusMsg != "" {
		footer = statusStyle.Render(a.statusMsg) + "\n" + footer
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}
