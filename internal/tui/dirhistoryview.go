package tui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ahngbeom/rest-tui/internal/output"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Ahngbeom/rest-tui/internal/dirhistory"
)

type dirEntryItem struct {
	entry dirhistory.Entry
}

func (d dirEntryItem) FilterValue() string { return d.entry.Path }
func (d dirEntryItem) Title() string       { return d.entry.Path }
func (d dirEntryItem) Description() string { return d.entry.Timestamp.Format("2006-01-02 15:04:05") }

type dirHistoryModel struct {
	store *dirhistory.Store

	list    list.Model
	loadErr error

	prompt    textinput.Model
	promptErr string

	width, height int
}

func newDirHistoryModel(store *dirhistory.Store) dirHistoryModel {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Directories"
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)

	ti := textinput.New()
	ti.Prompt = promptLabel

	return dirHistoryModel{store: store, list: l, prompt: ti}
}

const promptLabel = "open: "

func (m dirHistoryModel) isPrompting() bool {
	return m.prompt.Focused()
}

// startPrompt opens the path prompt below the list.
func (m dirHistoryModel) startPrompt() (dirHistoryModel, tea.Cmd) {
	m.prompt.SetValue("")
	m.promptErr = ""
	cmd := m.prompt.Focus()
	return m.SetSize(m.width, m.height), cmd
}

func (m dirHistoryModel) closePrompt() dirHistoryModel {
	m.prompt.Blur()
	m.promptErr = ""
	return m.SetSize(m.width, m.height)
}

// expandHome resolves a leading "~" or "~/" to the user's home directory.
// "~user" forms are left untouched.
func expandHome(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if p == "~" {
		return home, nil
	}
	return filepath.Join(home, p[2:]), nil
}

// submitPrompt validates the typed path and either switches to it or keeps
// the prompt open with an error.
func (m dirHistoryModel) submitPrompt() (dirHistoryModel, tea.Cmd) {
	input := m.prompt.Value()
	shown := output.Sanitize(input)
	p, err := expandHome(input)
	if err == nil {
		p, err = filepath.Abs(p)
	}
	if err != nil {
		m.promptErr = "invalid path: " + shown
		return m, nil
	}
	info, err := os.Stat(p)
	switch {
	case err != nil:
		m.promptErr = "no such directory: " + shown
		return m, nil
	case !info.IsDir():
		m.promptErr = "not a directory: " + shown
		return m, nil
	}
	return m, func() tea.Msg { return switchDirMsg{path: p} }
}

// refresh reloads the most recently touched directories from the store. It
// is called every time the Directory History screen is entered so it always
// reflects the latest -dir runs and switches.
func (m dirHistoryModel) refresh() dirHistoryModel {
	entries, err := m.store.List(50)
	if err != nil {
		m.loadErr = err
		return m
	}
	m.loadErr = nil
	items := make([]list.Item, len(entries))
	for i, e := range entries {
		items[i] = dirEntryItem{entry: e}
	}
	m.list.ResetFilter()
	m.list.SetItems(items)
	return m
}

func (m dirHistoryModel) isFiltering() bool {
	return m.list.FilterState() == list.Filtering
}

func (m dirHistoryModel) SetSize(width, height int) dirHistoryModel {
	m.width, m.height = width, height
	innerWidth := max(0, width-paneFocusedStyle.GetHorizontalFrameSize())
	innerHeight := max(0, height-paneFocusedStyle.GetVerticalFrameSize())
	if m.isPrompting() {
		innerHeight = max(0, innerHeight-2)
		m.prompt.Width = max(0, innerWidth-lipgloss.Width(promptLabel)-1)
	}
	m.list.SetSize(innerWidth, innerHeight)
	return m
}

func (m dirHistoryModel) Update(msg tea.Msg) (dirHistoryModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok && m.isPrompting() {
		switch keyMsg.Type {
		case tea.KeyEnter:
			return m.submitPrompt()
		case tea.KeyEsc:
			return m.closePrompt(), nil
		}
		var cmd tea.Cmd
		m.prompt, cmd = m.prompt.Update(msg)
		return m, cmd
	}
	if keyMsg, ok := msg.(tea.KeyMsg); ok && !listCapturesKey(m.list, keyMsg) {
		switch {
		case key.Matches(keyMsg, keys.OpenPath):
			return m.startPrompt()
		case key.Matches(keyMsg, keys.Back):
			return m, func() tea.Msg { return backToBrowserMsg{} }
		case key.Matches(keyMsg, keys.Enter):
			if item, ok := m.list.SelectedItem().(dirEntryItem); ok {
				path := item.entry.Path
				return m, func() tea.Msg { return switchDirMsg{path: path} }
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m dirHistoryModel) View() string {
	pane := paneFocusedStyle.Width(max(0, m.width-paneFocusedStyle.GetHorizontalBorderSize()))
	if m.loadErr != nil {
		msg := "failed to load directory history: " + m.loadErr.Error()
		inner := fitLines(msg, max(0, m.width-pane.GetHorizontalFrameSize()), max(0, m.height-pane.GetVerticalFrameSize()))
		return pane.Render(errorTextStyle.Render(inner))
	}
	body := m.list.View()
	if m.isPrompting() {
		innerWidth := max(0, m.width-pane.GetHorizontalFrameSize())
		errLine := ansi.Truncate(m.promptErr, innerWidth, "…")
		body += "\n" + m.prompt.View() + "\n" + errorTextStyle.Render(errLine)
	}
	return pane.Render(body)
}
