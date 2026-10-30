package tui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ahngbeom/rest-tui/internal/executor"
	"github.com/Ahngbeom/rest-tui/internal/history"
	"github.com/Ahngbeom/rest-tui/internal/httpfile"
	"github.com/Ahngbeom/rest-tui/internal/output"
)

type historyMode int

const (
	historyModeList historyMode = iota
	historyModeDetail
)

type entryItem struct {
	entry history.Entry
}

func (e entryItem) FilterValue() string {
	return e.entry.Method + " " + e.entry.Name + " " + e.entry.URL + " " + e.entry.Env
}

func relativeTime(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	}
	return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
}

var rowWhitespace = strings.NewReplacer("\n", " ", "\t", " ")

func oneLine(s string) string {
	return rowWhitespace.Replace(output.Sanitize(s))
}

func formatEntryRow(e history.Entry, now time.Time, width int) string {
	status := fmt.Sprintf("%3d", e.StatusCode)
	if e.Error != "" {
		status = "ERR"
	}
	method := oneLine(e.Method)
	head := statusCodeStyle(e.StatusCode).Render(status) + " " + methodStyle(method).Render(fmt.Sprintf("%-7s", method)) + " "
	if e.Env != "" {
		head += oneLine(e.Env) + " "
	}
	name := e.Name
	if name == "" {
		name = e.URL
	}
	rel := relativeTime(e.Timestamp, now)
	row := head
	if room := width - lipgloss.Width(head) - 2 - lipgloss.Width(rel); room >= 1 {
		row += ansi.Truncate(oneLine(name), room, "…")
	}
	row += "  " + rel
	return ansi.Truncate(row, width, "")
}

type entryRowDelegate struct{}

func (entryRowDelegate) Height() int                         { return 1 }
func (entryRowDelegate) Spacing() int                        { return 0 }
func (entryRowDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (entryRowDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(entryItem)
	if !ok {
		return
	}
	prefix := "  "
	if index == m.Index() {
		prefix = "> "
	}
	fmt.Fprint(w, prefix+formatEntryRow(it.entry, time.Now(), m.Width()-2))
}

// renderEntryDetail formats a full request/response record the same way a
// live Request view would, so history and live execution look consistent.
func renderEntryDetail(e history.Entry) string {
	var b strings.Builder

	if e.Env != "" {
		b.WriteString(mutedTextStyle.Render("env: " + e.Env))
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "%s %s\n", output.Sanitize(e.Method), output.Sanitize(e.URL))
	for _, h := range e.RequestHeaders {
		fmt.Fprintf(&b, "%s: %s\n", output.Sanitize(h.Name), output.Sanitize(h.Value))
	}
	if e.RequestBody != "" {
		b.WriteString("\n")
		b.WriteString(output.PrettyBody([]byte(output.Sanitize(e.RequestBody)), output.Options{Color: true}))
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

// copyEntryPlainText builds the plain-text (no ANSI) request+response block
// for a history entry, for the Copy key in detail mode.
func copyEntryPlainText(e history.Entry) string {
	req := httpfile.Request{
		Method:  e.Method,
		URL:     e.URL,
		Headers: e.RequestHeaders,
		Body:    e.RequestBody,
	}
	var resp *executor.Response
	var note string
	if e.Error != "" {
		note = "Error: " + e.Error
	} else {
		resp = responseFromEntry(e)
	}
	return output.RenderTransaction(req, resp, note, output.Options{Color: false})
}

type historyModel struct {
	store *history.Store

	list   list.Model
	detail viewport.Model
	mode   historyMode

	selected     *history.Entry
	pendingRerun *history.Entry
	loadErr      error

	copyStatus string
	copyErr    bool
	copyToken  int

	width, height int
}

func newHistoryModel(store *history.Store) historyModel {
	l := list.New(nil, entryRowDelegate{}, 0, 0)
	l.Title = "History"
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)

	return historyModel{store: store, list: l, detail: viewport.New(0, 0)}
}

// refresh reloads the most recent entries from the store. It is called every
// time the History screen is entered so it always reflects the latest runs.
func (m historyModel) refresh() historyModel {
	m.pendingRerun = nil
	var keepID string
	if item, ok := m.list.SelectedItem().(entryItem); ok {
		keepID = item.entry.ID
	}
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
	m.list.ResetFilter()
	m.list.SetItems(items)
	for i, it := range items {
		if keepID != "" && it.(entryItem).entry.ID == keepID {
			m.list.Select(i)
			break
		}
	}
	m.mode = historyModeList
	return m.layout()
}

func isSafeMethod(method string) bool {
	switch strings.ToUpper(method) {
	case "GET", "HEAD", "OPTIONS":
		return true
	}
	return false
}

// layout sizes the list and detail viewport, reserving one row for the rerun
// confirmation prompt while one is pending.
func (m historyModel) layout() historyModel {
	innerWidth := max(0, m.width-paneFocusedStyle.GetHorizontalFrameSize())
	innerHeight := max(0, m.height-paneFocusedStyle.GetVerticalFrameSize())
	prompt := 0
	if m.pendingRerun != nil {
		prompt = 1
	}
	m.list.SetSize(innerWidth, max(0, innerHeight-prompt))
	m.detail.Width, m.detail.Height = innerWidth, max(0, innerHeight-1-prompt) // one row: copy status
	return m
}

func (m historyModel) rerunPrompt() string {
	e := m.pendingRerun
	w := max(0, m.width-paneFocusedStyle.GetHorizontalFrameSize())
	method := output.Sanitize(e.Method)
	url := output.Sanitize(e.URL)
	if room := w - lipgloss.Width("rerun "+method+" ") - len("? (y/n)"); lipgloss.Width(url) > room {
		url = ansi.Truncate(url, max(0, room), "…")
	}
	return warnTextStyle.Render(fmt.Sprintf("rerun %s %s? (y/n)", method, url))
}

func (m historyModel) isFiltering() bool {
	return m.mode == historyModeList && m.list.FilterState() == list.Filtering
}

func (m historyModel) SetSize(width, height int) historyModel {
	m.width, m.height = width, height
	m = m.layout()
	innerWidth := m.detail.Width
	if m.mode == historyModeDetail && m.selected != nil {
		m.detail.SetContent(wrapToWidth(renderEntryDetail(*m.selected), innerWidth))
	}
	return m
}

func (m historyModel) Update(msg tea.Msg) (historyModel, tea.Cmd) {
	switch msg := msg.(type) {
	case clipboardCopyMsg:
		if msg.token != m.copyToken {
			return m, nil
		}
		m.copyErr = msg.err != nil
		if msg.err != nil {
			m.copyStatus = "copy failed: " + msg.err.Error()
		} else {
			m.copyStatus = "copied to clipboard"
		}
		return m, clearCopyStatusAfter(msg.token)
	case clipboardCopyExpiredMsg:
		if msg.token == m.copyToken {
			m.copyStatus = ""
		}
		return m, nil
	case tea.KeyMsg:
		if m.mode == historyModeList && listCapturesKey(m.list, msg) {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}
		if m.pendingRerun != nil {
			entry := *m.pendingRerun
			m.pendingRerun = nil
			m = m.layout()
			if key.Matches(msg, keys.Confirm) {
				return m, func() tea.Msg { return rerunMsg{entry: entry} }
			}
			return m, nil
		}
		switch {
		case key.Matches(msg, keys.Back):
			if m.mode == historyModeDetail {
				m.mode = historyModeList
				return m, nil
			}
			return m, func() tea.Msg { return backToBrowserMsg{} }
		case key.Matches(msg, keys.Enter):
			if m.mode == historyModeList {
				if item, ok := m.list.SelectedItem().(entryItem); ok {
					e := item.entry
					m.selected = &e
					m.detail.SetContent(wrapToWidth(renderEntryDetail(e), m.detail.Width))
					m.mode = historyModeDetail
				}
			}
			return m, nil
		case key.Matches(msg, keys.Rerun):
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
			if !isSafeMethod(entry.Method) {
				m.pendingRerun = &entry
				return m.layout(), nil
			}
			return m, func() tea.Msg { return rerunMsg{entry: entry} }
		case key.Matches(msg, keys.Copy):
			if m.mode == historyModeDetail && m.selected != nil {
				m.copyToken++
				return m, copyToClipboardCmd(copyEntryPlainText(*m.selected), m.copyToken)
			}
			return m, nil
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
	pane := paneFocusedStyle.Width(max(0, m.width-paneFocusedStyle.GetHorizontalBorderSize()))
	if m.loadErr != nil {
		msg := "failed to load history: " + m.loadErr.Error()
		inner := fitLines(msg, max(0, m.width-pane.GetHorizontalFrameSize()), max(0, m.height-pane.GetVerticalFrameSize()))
		return pane.Render(errorTextStyle.Render(inner))
	}
	if m.mode == historyModeDetail {
		status := ""
		if m.copyStatus != "" {
			style := copiedTextStyle
			if m.copyErr {
				style = errorTextStyle
			}
			status = style.Render(ansi.Truncate(m.copyStatus, m.detail.Width, "…"))
		}
		out := m.detail.View() + "\n" + status
		if m.pendingRerun != nil {
			out += "\n" + m.rerunPrompt()
		}
		return pane.Render(out)
	}
	if m.pendingRerun != nil {
		return pane.Render(m.list.View() + "\n" + m.rerunPrompt())
	}
	return pane.Render(m.list.View())
}
