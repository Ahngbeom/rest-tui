package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/bahn/rest-tui/internal/history"
	"github.com/bahn/rest-tui/internal/output"
)

type historyMode int

const (
	historyModeList historyMode = iota
	historyModeDetail
)

type entryItem struct {
	entry history.Entry
}

func (e entryItem) FilterValue() string { return e.entry.URL }

func (e entryItem) Title() string {
	status := fmt.Sprintf("%d", e.entry.StatusCode)
	if e.entry.Error != "" {
		status = "ERR"
	}
	return fmt.Sprintf("%s  %s %s -> %s", e.entry.Timestamp.Format("15:04:05"), e.entry.Method, e.entry.URL, status)
}

func (e entryItem) Description() string {
	if e.entry.Error != "" {
		return e.entry.Error
	}
	return e.entry.Duration.Round(time.Millisecond).String()
}

// renderEntryDetail formats a full request/response record the same way a
// live Request view would, so history and live execution look consistent.
func renderEntryDetail(e history.Entry) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s %s\n", e.Method, e.URL)
	for _, h := range e.RequestHeaders {
		fmt.Fprintf(&b, "%s: %s\n", h.Name, h.Value)
	}
	if e.RequestBody != "" {
		b.WriteString("\n")
		b.WriteString(output.PrettyBody([]byte(e.RequestBody), output.Options{Color: true}))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(mutedTextStyle.Render(strings.Repeat("─", 40)))
	b.WriteString("\n\n")

	if e.Error != "" {
		b.WriteString(errorTextStyle.Render("Error: " + e.Error))
	} else {
		b.WriteString(output.RenderResponse(responseFromEntry(e), output.Options{Color: true}))
	}

	return b.String()
}

type historyModel struct {
	store *history.Store

	list   list.Model
	detail viewport.Model
	mode   historyMode

	selected *history.Entry
	loadErr  error

	width, height int
}

func newHistoryModel(store *history.Store) historyModel {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "History"
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)

	return historyModel{store: store, list: l, detail: viewport.New(0, 0)}
}

// refresh reloads the most recent entries from the store. It is called every
// time the History screen is entered so it always reflects the latest runs.
func (m historyModel) refresh() historyModel {
	entries, err := m.store.List(50)
	if err != nil {
		m.loadErr = err
		return m
	}
	m.loadErr = nil
	items := make([]list.Item, len(entries))
	for i, e := range entries {
		items[i] = entryItem{entry: e}
	}
	m.list.SetItems(items)
	m.mode = historyModeList
	return m
}

func (m historyModel) SetSize(width, height int) historyModel {
	m.width, m.height = width, height
	innerWidth, innerHeight := width-4, height-2
	if innerWidth < 0 {
		innerWidth = 0
	}
	if innerHeight < 0 {
		innerHeight = 0
	}
	m.list.SetSize(innerWidth, innerHeight)
	m.detail.Width, m.detail.Height = innerWidth, innerHeight
	return m
}

func (m historyModel) Update(msg tea.Msg) (historyModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(keyMsg, keys.Back):
			if m.mode == historyModeDetail {
				m.mode = historyModeList
				return m, nil
			}
			return m, func() tea.Msg { return backToBrowserMsg{} }
		case key.Matches(keyMsg, keys.Enter):
			if m.mode == historyModeList {
				if item, ok := m.list.SelectedItem().(entryItem); ok {
					e := item.entry
					m.selected = &e
					m.detail.SetContent(renderEntryDetail(e))
					m.mode = historyModeDetail
				}
			}
			return m, nil
		case key.Matches(keyMsg, keys.Rerun):
			var entry history.Entry
			switch {
			case m.mode == historyModeDetail && m.selected != nil:
				entry = *m.selected
			default:
				item, ok := m.list.SelectedItem().(entryItem)
				if !ok {
					return m, nil
				}
				entry = item.entry
			}
			return m, func() tea.Msg { return rerunMsg{entry: entry} }
		}
	}

	var cmd tea.Cmd
	if m.mode == historyModeDetail {
		m.detail, cmd = m.detail.Update(msg)
	} else {
		m.list, cmd = m.list.Update(msg)
	}
	return m, cmd
}

func (m historyModel) View() string {
	if m.loadErr != nil {
		return paneFocusedStyle.Render(errorTextStyle.Render("failed to load history: " + m.loadErr.Error()))
	}
	if m.mode == historyModeDetail {
		return paneFocusedStyle.Render(m.detail.View())
	}
	return paneFocusedStyle.Render(m.list.View())
}
