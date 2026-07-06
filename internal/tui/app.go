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

	"github.com/Ahngbeom/rest-tui/internal/history"
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
)

// App is the root bubbletea model. It owns the current screen and delegates
// messages to whichever screen is active.
type App struct {
	screen        screen
	width, height int

	browser browserModel
	request requestModel
	history historyModel

	store *history.Store
	root  string

	showHelp  bool
	statusMsg string
}

// NewApp builds the root model. root is the directory .http files are
// discovered under; store is where executed requests are recorded.
func NewApp(root string, store *history.Store) App {
	a := App{root: root, store: store}
	a.browser = newBrowserModel(root)
	a.history = newHistoryModel(store)
	if w := store.Warning(); w != "" {
		a.statusMsg = w
	}
	return a
}

func (a App) Init() tea.Cmd {
	return nil
}

func (a App) contentHeight() int {
	h := a.height - headerHeight - footerHeight
	if h < 0 {
		h = 0
	}
	return h
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		ch := a.contentHeight()
		a.browser = a.browser.SetSize(msg.Width, ch)
		a.request = a.request.SetSize(msg.Width, ch)
		a.history = a.history.SetSize(msg.Width, ch)
		return a, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return a, tea.Quit
		case key.Matches(msg, keys.Help):
			a.showHelp = !a.showHelp
			return a, nil
		case key.Matches(msg, keys.ToggleLog):
			a.screen = screenHistory
			a.history = a.history.refresh()
			return a, nil
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

	case backToBrowserMsg:
		a.screen = screenBrowser
		return a, nil

	case openHistoryMsg:
		a.screen = screenHistory
		a.history = a.history.refresh()
		return a, nil
	}

	var cmd tea.Cmd
	switch a.screen {
	case screenBrowser:
		a.browser, cmd = a.browser.Update(msg)
	case screenRequest:
		a.request, cmd = a.request.Update(msg)
	case screenHistory:
		a.history, cmd = a.history.Update(msg)
	}
	return a, cmd
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
	default:
		return "rest-tui › Browser"
	}
}

func (a App) footerHints() string {
	switch a.screen {
	case screenRequest:
		return "e cycle env  enter/ctrl+r send  ↑/↓ scroll  esc back  h history  ? help  q quit"
	case screenHistory:
		if a.history.mode == historyModeDetail {
			return "↑/↓ scroll  r rerun  esc back  ? help  q quit"
		}
		return "↑/↓ move  enter detail  r rerun  esc back  ? help  q quit"
	default:
		if a.browser.focus == paneRequests {
			return "↑/↓ move  enter run  tab← files  esc back  h history  ? help  q quit"
		}
		return "↑/↓ move  enter open  tab→ requests  h history  ? help  q quit"
	}
}

func (a App) helpView() string {
	lines := []string{
		"Global",
		"  q, ctrl+c     quit",
		"  h             jump to History",
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
		"",
		"History",
		"  enter         view detail",
		"  r             rerun",
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
	default:
		body = a.browser.View()
	}

	footer := footerStyle.Render(a.footerHints())
	if a.statusMsg != "" {
		footer = statusStyle.Render(a.statusMsg) + "\n" + footer
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}
