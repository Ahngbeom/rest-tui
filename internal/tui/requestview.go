package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ahngbeom/rest-tui/internal/env"
	"github.com/Ahngbeom/rest-tui/internal/executor"
	"github.com/Ahngbeom/rest-tui/internal/history"
	"github.com/Ahngbeom/rest-tui/internal/httpfile"
	"github.com/Ahngbeom/rest-tui/internal/output"
)

const requestTimeout = 30 * time.Second

// responseFromEntry reconstructs an *executor.Response from a stored history
// entry so it can be rendered with output.RenderResponse.
func responseFromEntry(e history.Entry) *executor.Response {
	return &executor.Response{
		StatusCode: e.StatusCode,
		Status:     e.Status,
		Headers:    e.ResponseHeaders,
		Body:       []byte(e.ResponseBody),
		Duration:   e.Duration,
	}
}

var requestModelCounter atomic.Uint64

func nextRequestModelID() uint64 { return requestModelCounter.Add(1) }

type requestModel struct {
	id       uint64
	filePath string
	root     string
	file     *httpfile.File
	req      httpfile.Request

	envNames  []string
	envIndex  int
	envSource env.Loaded
	envErr    error

	resolved    httpfile.Request
	missingVars []string

	editing   bool
	editor    textarea.Model
	editError string

	executing   bool
	lastEntry   *history.Entry
	historyWarn string

	copyStatus string
	copyErr    bool
	copyToken  int

	viewport           viewport.Model
	responseLineOffset int
	width, height      int

	store *history.Store
}

// newRequestModel builds a Request view for a request selected in the
// Browser, resolving {{var}} placeholders against the env files nearest the
// request (searching up to root) plus the file's own @-scoped variables.
func newRequestModel(filePath, root string, file *httpfile.File, req httpfile.Request, store *history.Store, preferredEnv string) requestModel {
	m := requestModel{id: nextRequestModelID(), filePath: filePath, root: root, file: file, req: req, store: store, envIndex: -1}
	m.viewport = viewport.New(0, 0)
	m.editor = textarea.New()
	m.editor.ShowLineNumbers = false
	m.editor.Prompt = ""

	if filePath != "" {
		m.envSource, m.envErr = env.LoadNearest(filepath.Dir(filePath), root)
		if m.envErr == nil {
			m.envNames = env.EnvNames(m.envSource.Public, m.envSource.Private)
			if len(m.envNames) > 0 {
				m.envIndex = 0
			}
		}
	}

	m.recompute()
	return m
}

// newRequestModelFromEntry builds a Request view pre-loaded with an
// already-resolved past entry, and returns a command that re-sends it
// immediately.
func newRequestModelFromEntry(entry history.Entry, store *history.Store) (requestModel, tea.Cmd) {
	req := httpfile.Request{
		Name:    "(rerun) " + entry.Method + " " + entry.URL,
		Method:  entry.Method,
		URL:     entry.URL,
		Headers: entry.RequestHeaders,
		Body:    entry.RequestBody,
	}
	m := requestModel{id: nextRequestModelID(), req: req, store: store, envIndex: -1}
	m.viewport = viewport.New(0, 0)
	m.editor = textarea.New()
	m.editor.ShowLineNumbers = false
	m.editor.Prompt = ""
	m.recompute()
	m.executing = true
	m.refreshContent()
	m.scrollToResponse()
	return m, m.sendCmd()
}

// newRequestModelFromEntryNoSend builds a Request view pre-loaded with an
// already-resolved past entry, like newRequestModelFromEntry, but lands on
// the screen without auto-sending. A history entry has no associated file
// path, so no environment files are loaded (envIndex stays -1).
func newRequestModelFromEntryNoSend(entry history.Entry, store *history.Store) requestModel {
	req := httpfile.Request{
		Method:  entry.Method,
		URL:     entry.URL,
		Headers: entry.RequestHeaders,
		Body:    entry.RequestBody,
	}
	m := requestModel{id: nextRequestModelID(), req: req, store: store, envIndex: -1}
	m.viewport = viewport.New(0, 0)
	m.editor = textarea.New()
	m.editor.ShowLineNumbers = false
	m.editor.Prompt = ""
	m.recompute()
	return m
}

func (m *requestModel) recompute() {
	fileVars := map[string]string{}
	if m.file != nil {
		fileVars = m.file.Vars
	}
	envName := ""
	if m.envIndex >= 0 && m.envIndex < len(m.envNames) {
		envName = m.envNames[m.envIndex]
	}
	vars := env.Merge(m.envSource.Public, m.envSource.Private, envName, fileVars)
	m.resolved, m.missingVars = env.ResolveRequest(m.req, vars)
	m.refreshContent()
}

func (m *requestModel) refreshContent() {
	content, responseOffset := m.buildContent()
	m.viewport.SetContent(content)
	m.responseLineOffset = responseOffset
}

// scrollToResponse forces the viewport to the line where the response
// section begins (just below the divider), discarding any prior manual
// scroll position. Call this only at the moments a fresh response should
// take over the view — never from refreshContent, so that resizes and env
// cycling don't disturb wherever the user was reading.
func (m *requestModel) scrollToResponse() {
	m.viewport.SetYOffset(m.responseLineOffset)
}

func (m requestModel) SetSize(width, height int) requestModel {
	m.width, m.height = width, height
	innerWidth := max(0, width-paneFocusedStyle.GetHorizontalFrameSize())
	innerHeight := max(0, height-paneFocusedStyle.GetVerticalFrameSize()-2) // env line + status line
	m.viewport.Width = innerWidth
	m.viewport.Height = innerHeight
	m.editor.SetWidth(innerWidth)
	m.editor.SetHeight(innerHeight)
	m.refreshContent()
	return m
}

// serializeRequest renders req into the same textual shape as a .http file
// request block ("METHOD URL\nHeader-Name: value\n...\n\n<body>"), used to
// seed the raw-text edit buffer. It mirrors the request-rendering portion of
// buildContent but produces plain text with no ANSI/lipgloss styling, since
// this text is edited by hand and then reparsed by httpfile.Parse.
func serializeRequest(req httpfile.Request) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", req.Method, req.URL)
	for _, h := range req.Headers {
		fmt.Fprintf(&b, "%s: %s\n", h.Name, h.Value)
	}
	if req.Body != "" {
		b.WriteString("\n")
		b.WriteString(output.PrettyBody([]byte(req.Body), output.Options{Color: false}))
	}
	return b.String()
}

// startEditing seeds the edit buffer from m.resolved (never m.req, since
// edits act on the final, variable-substituted values) and focuses it.
func (m requestModel) startEditing() (requestModel, tea.Cmd) {
	m.editor.SetValue(serializeRequest(m.resolved))
	cmd := m.editor.Focus()
	m.editing = true
	m.editError = ""
	return m, cmd
}

func (m requestModel) cancelEditing() requestModel {
	m.editing = false
	m.editError = ""
	m.editor.Blur()
	return m
}

// applyEdit reparses the edit buffer via httpfile.Parse and, on success,
// replaces m.resolved and clears m.missingVars (the edited values are final,
// not template placeholders needing resolution). On failure it stays in edit
// mode with the buffer intact and sets m.editError.
func (m requestModel) applyEdit() requestModel {
	f, err := httpfile.Parse([]byte(m.editor.Value()))
	if err != nil {
		m.editError = err.Error()
		return m
	}
	if len(f.Requests) != 1 {
		m.editError = fmt.Sprintf("expected exactly one request, got %d", len(f.Requests))
		return m
	}

	parsed := f.Requests[0]
	m.resolved = httpfile.Request{
		Method:  parsed.Method,
		URL:     parsed.URL,
		Headers: parsed.Headers,
		Body:    parsed.Body,
	}
	m.missingVars = nil
	m.editing = false
	m.editError = ""
	m.editor.Blur()
	m.refreshContent()
	return m
}

func (m requestModel) isEditing() bool { return m.editing }

func (m requestModel) sendCmd() tea.Cmd {
	resolved := m.resolved
	store := m.store
	id := m.id
	return func() tea.Msg {
		resp, err := executor.Execute(context.Background(), resolved, requestTimeout)

		entry := history.Entry{
			Method:         resolved.Method,
			URL:            resolved.URL,
			RequestHeaders: resolved.Headers,
			RequestBody:    resolved.Body,
		}
		if err != nil {
			entry.Error = err.Error()
		} else {
			entry.StatusCode = resp.StatusCode
			entry.Status = resp.Status
			entry.ResponseHeaders = resp.Headers
			entry.ResponseBody = string(resp.Body)
			entry.Duration = resp.Duration
		}

		var historyWarn string
		if stored, storeErr := store.Append(entry); storeErr == nil {
			entry = stored
		} else {
			historyWarn = "could not save to history: " + storeErr.Error()
		}

		return execResultMsg{modelID: id, entry: entry, historyWarn: historyWarn}
	}
}

func (m requestModel) Update(msg tea.Msg) (requestModel, tea.Cmd) {
	switch msg := msg.(type) {
	case execResultMsg:
		if msg.modelID != m.id {
			return m, nil
		}
		m.executing = false
		entry := msg.entry
		m.lastEntry = &entry
		m.historyWarn = msg.historyWarn
		m.refreshContent()
		m.scrollToResponse()
		return m, nil
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
		if m.editing {
			switch {
			case key.Matches(msg, keys.CancelEdit):
				return m.cancelEditing(), nil
			case key.Matches(msg, keys.ApplyEdit):
				return m.applyEdit(), nil
			}
			var cmd tea.Cmd
			m.editor, cmd = m.editor.Update(msg)
			return m, cmd
		}

		switch {
		case key.Matches(msg, keys.Edit):
			return m.startEditing()
		case key.Matches(msg, keys.CycleEnv):
			if len(m.envNames) > 0 {
				m.envIndex = (m.envIndex + 1) % len(m.envNames)
				m.recompute()
			}
			return m, nil
		case key.Matches(msg, keys.Send):
			if m.executing || len(m.missingVars) > 0 {
				return m, nil
			}
			m.executing = true
			m.refreshContent()
			m.scrollToResponse()
			return m, m.sendCmd()
		case key.Matches(msg, keys.Copy):
			m.copyToken++
			return m, copyToClipboardCmd(m.copyText(), m.copyToken)
		case key.Matches(msg, keys.Back):
			return m, func() tea.Msg { return backToBrowserMsg{} }
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// copyText builds the plain-text (no ANSI) request+response block for the
// Copy key, mirroring buildContent's request/response data but omitting
// UI-only notices (missingVars, historyWarn).
func (m requestModel) copyText() string {
	var resp *executor.Response
	var note string
	switch {
	case m.executing:
		note = "Sending request..."
	case m.lastEntry == nil:
		note = "(not yet sent — press enter to send)"
	case m.lastEntry.Error != "":
		note = "Error: " + m.lastEntry.Error
	default:
		resp = responseFromEntry(*m.lastEntry)
	}
	return output.RenderTransaction(m.resolved, resp, note, output.Options{Color: false})
}

func (m requestModel) buildContent() (string, int) {
	var head strings.Builder

	fmt.Fprintf(&head, "%s %s\n", m.resolved.Method, m.resolved.URL)
	for _, h := range m.resolved.Headers {
		fmt.Fprintf(&head, "%s: %s\n", h.Name, h.Value)
	}
	if m.resolved.Body != "" {
		head.WriteString("\n")
		head.WriteString(output.PrettyBody([]byte(m.resolved.Body), output.Options{Color: true}))
		head.WriteString("\n")
	}
	if len(m.missingVars) > 0 {
		head.WriteString("\n")
		head.WriteString(errorTextStyle.Render("unresolved variables: " + strings.Join(m.missingVars, ", ")))
		head.WriteString("\n")
	}

	head.WriteString("\n")
	head.WriteString(mutedTextStyle.Render(strings.Repeat("─", 40)))
	head.WriteString("\n\n")

	headStr := wrapToWidth(head.String(), m.viewport.Width)
	responseOffset := strings.Count(headStr, "\n")

	var tail strings.Builder

	switch {
	case m.executing:
		tail.WriteString("Sending request...")
	case m.lastEntry == nil:
		tail.WriteString(mutedTextStyle.Render("(not yet sent — press enter to send)"))
	case m.lastEntry.Error != "":
		tail.WriteString(errorTextStyle.Render("Error: " + m.lastEntry.Error))
	default:
		tail.WriteString(output.RenderResponse(responseFromEntry(*m.lastEntry), output.Options{Color: true}))
	}

	if m.historyWarn != "" {
		tail.WriteString("\n\n")
		tail.WriteString(mutedTextStyle.Render(m.historyWarn))
	}

	return headStr + wrapToWidth(tail.String(), m.viewport.Width), responseOffset
}

func (m requestModel) envLine() string {
	line := m.untruncatedEnvLine()
	if m.viewport.Width > 0 {
		line = ansi.Truncate(line, m.viewport.Width, "…")
	}
	return line
}

func (m requestModel) untruncatedEnvLine() string {
	src := m.envSource
	switch {
	case m.envErr != nil:
		return errorTextStyle.Render(strings.ReplaceAll(m.envErr.Error(), "\n", " "))
	case src.Dir == "":
		text := "no " + env.PublicFileName + " found"
		if len(src.Searched) > 0 {
			labels := make([]string, len(src.Searched))
			for i, dir := range src.Searched {
				labels[i] = dirLabel(m.root, dir)
			}
			text += " (searched: " + strings.Join(labels, ", ") + ")"
		}
		return mutedTextStyle.Render(text)
	}
	file := dirLabel(m.root, src.Dir) + src.FileName()
	if len(m.envNames) == 0 {
		return "env: (none) · " + file
	}
	line := fmt.Sprintf("env: %s (%d/%d) · %s", m.envNames[m.envIndex], m.envIndex+1, len(m.envNames), file)
	if m.canCycleEnv() {
		line += " — e cycle"
	}
	return line
}

func (m requestModel) canCycleEnv() bool { return len(m.envNames) > 1 }

func dirLabel(root, dir string) string {
	if root != "" {
		if rel, err := filepath.Rel(root, dir); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			if rel == "." {
				return "./"
			}
			return filepath.ToSlash(rel) + "/"
		}
	}
	return filepath.ToSlash(filepath.Clean(dir)) + "/"
}

func (m requestModel) title() string {
	if m.req.Name != "" {
		return m.req.Name
	}
	return m.req.Method + " " + m.req.URL
}

func (m requestModel) statusLine() string {
	text, style := m.copyStatus, copiedTextStyle
	if m.copyErr {
		style = errorTextStyle
	}
	if m.editing {
		text, style = m.editError, errorTextStyle
	}
	if text == "" {
		return ""
	}
	text = strings.ReplaceAll(text, "\n", " ")
	return style.Render(ansi.Truncate(text, m.viewport.Width, "…"))
}

func (m requestModel) View() string {
	body := m.viewport.View()
	if m.editing {
		body = m.editor.View()
	}
	content := ansi.Truncate(m.envLine(), m.viewport.Width, "…") + "\n" + body + "\n" + m.statusLine()
	return paneFocusedStyle.Width(max(0, m.width-paneFocusedStyle.GetHorizontalBorderSize())).Render(content)
}
