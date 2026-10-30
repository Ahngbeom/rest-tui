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
	l.SetFilteringEnabled(true)

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
	m.list.SetSize(innerWidth, innerHeight)
	return m
}

func (m dirHistoryModel) Update(msg tea.Msg) (dirHistoryModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok && !listCapturesKey(m.list, keyMsg) {
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
	pane := paneFocusedStyle.Width(max(0, m.width-paneFocusedStyle.GetHorizontalBorderSize()))
	if m.loadErr != nil {
		msg := "failed to load directory history: " + m.loadErr.Error()
		inner := fitLines(msg, max(0, m.width-pane.GetHorizontalFrameSize()), max(0, m.height-pane.GetVerticalFrameSize()))
		return pane.Render(errorTextStyle.Render(inner))
	}
	return pane.Render(m.list.View())
}
