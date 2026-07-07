package tui

import (
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

	width, height int
}

func newDirHistoryModel(store *dirhistory.Store) dirHistoryModel {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Directories"
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)

	return dirHistoryModel{store: store, list: l}
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
	m.list.SetItems(items)
	return m
}

func (m dirHistoryModel) SetSize(width, height int) dirHistoryModel {
	m.width, m.height = width, height
	innerWidth, innerHeight := width-4, height-2
	if innerWidth < 0 {
		innerWidth = 0
	}
	if innerHeight < 0 {
		innerHeight = 0
	}
	m.list.SetSize(innerWidth, innerHeight)
	return m
}

func (m dirHistoryModel) Update(msg tea.Msg) (dirHistoryModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch {
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
	if m.loadErr != nil {
		return paneFocusedStyle.Render(errorTextStyle.Render("failed to load directory history: " + m.loadErr.Error()))
	}
	return paneFocusedStyle.Render(m.list.View())
}
