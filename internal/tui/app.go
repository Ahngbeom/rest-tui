// Package tui implements rest-tui's full-screen terminal application: a
// Browser screen for finding .http files and requests, a Request view for
// resolving variables and sending a request, and a History screen for
// reviewing and re-running past executions.
package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
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

const quitConfirmMsg = "request in flight — press q again to quit"

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
	lastEnv  string
	opts     Options

	showHelp     bool
	helpViewport viewport.Model
	statusMsg    string
	quitArmed    bool
}

// Options configures behavior that comes from the command line.
type Options struct {
	// Timeout bounds each request; zero means defaultRequestTimeout.
	Timeout time.Duration
}

// NewApp builds the root model. root is the directory .http files are
// discovered under; store is where executed requests are recorded; dirStore
// is where directories passed via -dir are recorded, so they can be browsed
// and switched to later from the Directory History screen.
func NewApp(root string, store *history.Store, dirStore *dirhistory.Store, opts Options) App {
	a := App{root: root, store: store, dirStore: dirStore, opts: opts}
	// Recording failures (e.g. an unwritable config dir) shouldn't stop the
	// app from starting -- directory history is a convenience, not core
	// functionality.
	_, _ = dirStore.Touch(root)
	a.browser = newBrowserModel(root, store)
	a.request = newRequestModel("", "", &httpfile.File{}, httpfile.Request{}, store, "")
	a.history = newHistoryModel(store)
	a.dirHistory = newDirHistoryModel(dirStore)
	a.helpViewport = viewport.New(0, 0)
	a.helpViewport.SetContent(strings.Join(helpLines(), "\n"))
	return a
}

// openRequestView makes m the active Request view, applying the app-wide
// request timeout and the current content size.
func (a App) openRequestView(m requestModel) App {
	a.request = m.withTimeout(a.opts.Timeout).SetSize(a.width, a.contentHeight())
	a.screen = screenRequest
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
	if a.statusMsg != "" {
		h--
	}
	return max(0, h)
}

func (a App) resize() App {
	ch := a.contentHeight()
	a.browser = a.browser.SetSize(a.width, ch)
	a.request = a.request.SetSize(a.width, ch)
	a.history = a.history.SetSize(a.width, ch)
	a.dirHistory = a.dirHistory.SetSize(a.width, ch)
	w := max(0, a.width-paneFocusedStyle.GetHorizontalFrameSize())
	a.helpViewport.Width = w
	a.helpViewport.Height = max(0, ch-paneFocusedStyle.GetVerticalFrameSize())
	a.helpViewport.SetContent(wrapToWidth(strings.Join(helpLines(), "\n"), w))
	return a
}

// applyStoreWarning copies any newly-detected history or directory-history
// store warning (e.g. a corrupted file being backed up) into the status
// line. Corruption is only discovered lazily, the first time something
// actually reads the file, so this is checked here rather than once at
// startup.
func (a App) applyStoreWarning() App {
	if a.quitArmed {
		return a
	}
	prev := a.statusMsg
	if w := a.store.Warning(); w != "" {
		a.statusMsg = w
	}
	if w := a.dirStore.Warning(); w != "" {
		a.statusMsg = w
	}
	if a.statusMsg != prev {
		a = a.resize()
	}
	return a
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		return a.resize(), nil

	case tea.KeyMsg:
		if msg.String() != "q" || a.activeScreenIsEditing() {
			a = a.disarmQuit()
		}
		if a.showHelp {
			return a.updateHelp(msg)
		}
		if !a.activeScreenIsEditing() {
			switch {
			case key.Matches(msg, keys.Quit):
				return a.handleQuit(msg)
			case key.Matches(msg, keys.Help):
				a.showHelp = true
				a.helpViewport.GotoTop()
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

	case execResultMsg, spinner.TickMsg:
		var cmd tea.Cmd
		a.request, cmd = a.request.Update(msg)
		if !a.request.isSending() {
			a = a.disarmQuit()
		}
		return a.applyStoreWarning(), cmd

	case openRequestMsg:
		return a.openRequestView(newRequestModel(msg.filePath, a.root, msg.file, msg.req, a.store, a.lastEnv)), nil

	case rerunMsg:
		a = a.openRequestView(newRequestModelFromEntryNoSend(msg.entry, a.store))
		if !a.request.canSend() {
			return a, nil
		}
		var cmd tea.Cmd
		a.request, cmd = a.request.startSend()
		return a, cmd

	case openRequestFromEntryMsg:
		return a.openRequestView(newRequestModelFromEntryNoSend(msg.entry, a.store)), nil

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
		before := a.request.currentEnv()
		a.request, cmd = a.request.Update(msg)
		if env := a.request.currentEnv(); env != "" && env != before {
			a.lastEnv = env
		}
	case screenHistory:
		a.history, cmd = a.history.Update(msg)
	case screenDirHistory:
		a.dirHistory, cmd = a.dirHistory.Update(msg)
	}
	return a.applyStoreWarning(), cmd
}

func (a App) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Help), msg.Type == tea.KeyEsc:
		a.showHelp = false
	case key.Matches(msg, keys.Quit):
		return a.handleQuit(msg)
	case key.Matches(msg, keys.Up):
		a.helpViewport.ScrollUp(1)
	case key.Matches(msg, keys.Down):
		a.helpViewport.ScrollDown(1)
	}
	return a, nil
}

func (a App) handleQuit(msg tea.KeyMsg) (App, tea.Cmd) {
	if msg.String() == "q" && a.request.isSending() && !a.quitArmed {
		a.quitArmed = true
		a.statusMsg = quitConfirmMsg
		return a.resize(), nil
	}
	return a, tea.Quit
}

func (a App) disarmQuit() App {
	a.quitArmed = false
	if a.statusMsg == quitConfirmMsg {
		a.statusMsg = ""
		a = a.resize()
	}
	return a
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

func (a App) footerHints() []hint {
	tail := []hint{{"?", "help"}, {"q", "quit"}}
	if a.showHelp {
		return append([]hint{{"↑/↓", "scroll"}, {"esc", "close"}}, tail...)
	}
	switch a.screen {
	case screenRequest:
		if a.request.isEditing() {
			return []hint{{"ctrl+s", "apply"}, {"esc", "cancel"}}
		}
		var hints []hint
		if a.request.canCycleEnv() {
			hints = append(hints, hint{"e", "cycle env"})
		}
		if a.request.canSend() {
			hints = append(hints, hint{"enter/ctrl+r", "send"})
		}
		esc := hint{"esc", "back"}
		if a.request.isSending() {
			esc = hint{"esc", "cancel"}
		}
		hints = append(hints, hint{"↑/↓", "scroll"}, hint{"g/G", "top/bottom"}, hint{"c", "copy"}, hint{"i", "edit"}, esc, hint{"h", "history"}, hint{"d", "dirs"})
		return append(hints, tail...)
	case screenHistory:
		if a.history.mode == historyModeDetail {
			return append([]hint{{"↑/↓", "scroll"}, {"r", "rerun"}, {"c", "copy"}, {"esc", "back"}}, tail...)
		}
		return append([]hint{{"↑/↓", "move"}, {"enter", "detail"}, {"r", "rerun"}, {"esc", "back"}}, tail...)
	case screenDirHistory:
		return append([]hint{{"↑/↓", "move"}, {"enter", "switch"}, {"esc", "back"}}, tail...)
	default:
		if a.browser.focus == paneRequests {
			return append([]hint{{"↑/↓", "move"}, {"enter", "run"}, {"tab", "← files"}, {"esc", "back"}, {"h", "history"}, {"d", "dirs"}}, tail...)
		}
		return append([]hint{{"↑/↓", "move"}, {"enter", "open"}, {"tab", "→ requests"}, {"h", "history"}, {"d", "dirs"}}, tail...)
	}
}

func helpLines() []string {
	return []string{
		"Global",
		"  q, ctrl+c     quit (q twice while a request is in flight)",
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
		"  g/G, home/end top / bottom of response",
		"  c, y          copy request/response",
		"  i             edit request (method/URL/headers/body)",
		"  esc           cancel send (while sending) / back",
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
}

func (a App) helpView() string {
	return paneFocusedStyle.Width(max(0, a.width-paneFocusedStyle.GetHorizontalBorderSize())).Render(a.helpViewport.View())
}

func (a App) View() string {
	header := headerStyle.Render(truncateLine(a.breadcrumb(), a.width-headerStyle.GetHorizontalPadding()))

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

	footer := footerStyle.Render(renderHints(a.footerHints(), a.width-footerStyle.GetHorizontalPadding()))
	if a.statusMsg != "" {
		footer = statusStyle.Render(truncateLine(a.statusMsg, a.width-statusStyle.GetHorizontalPadding())) + "\n" + footer
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}
