package tui

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ahngbeom/rest-tui/internal/env"
	"github.com/Ahngbeom/rest-tui/internal/executor"
	"github.com/Ahngbeom/rest-tui/internal/history"
	"github.com/Ahngbeom/rest-tui/internal/httpfile"
	"github.com/Ahngbeom/rest-tui/internal/output"
)

const defaultRequestTimeout = 30 * time.Second

// responseFromEntry reconstructs an *executor.Response from a stored history
// entry so it can be rendered with output.RenderResponse.
func responseFromEntry(e history.Entry) *executor.Response {
	return &executor.Response{
		StatusCode: e.StatusCode,
		Status:     e.Status,
		Headers:    e.ResponseHeaders,
		Body:       []byte(e.ResponseBody),
		Duration:   e.Duration,
		Redirects:  e.Redirects,
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

	resolved     httpfile.Request
	missingVars  []string
	blockedFlash bool
	edited       bool
	discardArmed bool

	editing   bool
	editor    textarea.Model
	editError string

	executing   bool
	spinner     spinner.Model
	sendStarted time.Time
	cancel      context.CancelFunc
	lastEntry   *history.Entry
	entryEnv    string
	fromEntry   bool
	historyWarn string

	copyStatus string
	copyErr    bool
	copyToken  int

	viewport           viewport.Model
	responseLineOffset int
	width, height      int

	store   *history.Store
	timeout time.Duration

	now func() time.Time
	rng *rand.Rand
}

func newRequestRand() *rand.Rand { return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())) }

// newRequestModel builds a Request view for a request selected in the
// Browser, resolving {{var}} placeholders against the env files nearest the
// request (searching up to root) plus the file's own @-scoped variables.
func newRequestModel(filePath, root string, file *httpfile.File, req httpfile.Request, store *history.Store, preferredEnv string) requestModel {
	m := requestModel{id: nextRequestModelID(), filePath: filePath, root: root, file: file, req: req, store: store, envIndex: -1, now: time.Now, rng: newRequestRand()}
	m.viewport = viewport.New(0, 0)
	m.spinner = spinner.New(spinner.WithSpinner(spinner.MiniDot))
	m.editor = textarea.New()
	m.editor.ShowLineNumbers = false
	m.editor.Prompt = ""

	if filePath != "" {
		m.envSource, m.envErr = env.LoadNearest(filepath.Dir(filePath), root)
		if m.envErr == nil {
			m.envNames = env.EnvNames(m.envSource.Public, m.envSource.Private)
			if len(m.envNames) > 0 {
				m.envIndex = 0
				if i := slices.Index(m.envNames, preferredEnv); i >= 0 {
					m.envIndex = i
				}
			}
		}
	}

	m.recompute()
	return m
}

// newRequestModelFromEntryNoSend builds a Request view pre-loaded with an
// already-resolved past entry, landing on the screen without sending. A
// history entry has no associated file path, so no environment files are
// loaded (envIndex stays -1).
func newRequestModelFromEntryNoSend(entry history.Entry, store *history.Store) requestModel {
	req := httpfile.Request{
		Method:  entry.Method,
		URL:     entry.URL,
		Headers: entry.RequestHeaders,
		Body:    entry.RequestBody,
		Name:    entry.Name,
	}
	m := requestModel{id: nextRequestModelID(), req: req, store: store, envIndex: -1, entryEnv: entry.Env, fromEntry: true, now: time.Now, rng: newRequestRand()}
	m.viewport = viewport.New(0, 0)
	m.spinner = spinner.New(spinner.WithSpinner(spinner.MiniDot))
	m.editor = textarea.New()
	m.editor.ShowLineNumbers = false
	m.editor.Prompt = ""
	m.recompute()
	return m
}

// recompute rebuilds m.resolved from m.req. A history entry was resolved
// when it was first sent, so fromEntry models keep it byte for byte: no
// placeholder blocks the send and no dynamic value is regenerated.
func (m *requestModel) recompute() {
	if m.fromEntry {
		m.resolved, m.missingVars = m.req, nil
		m.refreshContent()
		return
	}
	fileVars := map[string]string{}
	if m.file != nil {
		fileVars = m.file.Vars
	}
	envName := ""
	if m.envIndex >= 0 && m.envIndex < len(m.envNames) {
		envName = m.envNames[m.envIndex]
	}
	vars := env.Merge(m.envSource.Public, m.envSource.Private, envName, fileVars)
	m.resolved, m.missingVars = env.ResolveRequest(m.req, vars, m.resolveOptions())
	m.refreshContent()
}

func (m requestModel) resolveOptions() env.ResolveOptions {
	baseDir := ""
	if m.filePath != "" {
		baseDir = filepath.Dir(m.filePath)
	}
	return env.ResolveOptions{BaseDir: baseDir, Now: m.now(), Rand: m.rng}
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
	innerHeight := max(0, height-paneFocusedStyle.GetVerticalFrameSize()-3) // env, summary and status lines
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
// this text is edited by hand and then reparsed by httpfile.ParseVerbatimBody.
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

// applyEdit reparses the edit buffer via httpfile.ParseVerbatimBody and, on
// success, replaces m.resolved with the edited text, generating any
// {{$dynamic}} variables in it. The buffer was seeded from already-resolved
// values, so `< path` lines are not included again and the original
// Unsupported warnings carry over. Other {{…}} placeholders stay literal and
// are collected into m.missingVars so they block sending. On failure it
// stays in edit mode with the buffer intact and sets m.editError.
func (m requestModel) applyEdit() requestModel {
	f, err := httpfile.ParseVerbatimBody([]byte(m.editor.Value()))
	if err != nil {
		m.editError = err.Error()
		return m
	}
	if len(f.Requests) != 1 {
		m.editError = fmt.Sprintf("expected exactly one request, got %d", len(f.Requests))
		return m
	}

	parsed := f.Requests[0]
	edited := httpfile.Request{
		Method:      parsed.Method,
		URL:         parsed.URL,
		Headers:     parsed.Headers,
		Body:        parsed.Body,
		Unsupported: m.resolved.Unsupported,
	}
	opts := m.resolveOptions()
	opts.BaseDir = ""
	m.resolved, m.missingVars = env.ResolveRequest(edited, nil, opts)
	m.edited = true
	m.editing = false
	m.editError = ""
	m.editor.Blur()
	m.refreshContent()
	return m
}

func (m requestModel) isEditing() bool { return m.editing }

func (m requestModel) withTimeout(d time.Duration) requestModel {
	m.timeout = d
	return m
}

func (m requestModel) sendTimeout() time.Duration {
	if m.timeout <= 0 {
		return defaultRequestTimeout
	}
	return m.timeout
}

func describeSendError(err error, timeout time.Duration) string {
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Sprintf("timed out after %s (use -timeout to change)", timeout)
	}
	return err.Error()
}

// startSend marks the model as executing, scrolls to the response area and
// returns the command that performs the send. Callers check canSend first.
func (m requestModel) startSend() (requestModel, tea.Cmd) {
	m.executing = true
	m.sendStarted = time.Now()
	m.refreshContent()
	m.scrollToResponse()
	cmd := tea.Batch(m.sendCmd(), m.spinner.Tick)
	return m, cmd
}

func (m requestModel) isSending() bool { return m.executing }

func (m *requestModel) sendCmd() tea.Cmd {
	resolved := m.resolved
	timeout := m.sendTimeout()
	store := m.store
	id := m.id
	name := m.req.Name
	envName := m.currentEnv()
	if envName == "" {
		envName = m.entryEnv
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	return func() tea.Msg {
		defer cancel()
		resp, err := executor.Execute(ctx, resolved, timeout)

		entry := history.Entry{
			Method:         resolved.Method,
			URL:            resolved.URL,
			RequestHeaders: resolved.Headers,
			RequestBody:    resolved.Body,
			Env:            envName,
			Name:           name,
		}
		if err != nil {
			entry.Error = describeSendError(err, timeout)
		} else {
			entry.StatusCode = resp.StatusCode
			entry.Status = resp.Status
			entry.ResponseHeaders = resp.Headers
			entry.ResponseBody = string(resp.Body)
			entry.Duration = resp.Duration
			entry.Redirects = resp.Redirects
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
		m.cancel = nil
		entry := msg.entry
		m.lastEntry = &entry
		m.historyWarn = msg.historyWarn
		if m.edited {
			m.refreshContent()
		} else {
			m.recompute()
		}
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
	case spinner.TickMsg:
		if !m.executing {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case clipboardCopyExpiredMsg:
		if msg.token == m.copyToken {
			m.copyStatus = ""
		}
		return m, nil
	case tea.KeyMsg:
		m.blockedFlash = false
		if !key.Matches(msg, keys.CycleEnv) {
			m.discardArmed = false
		}
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
			if !m.canCycleEnv() {
				return m, nil
			}
			if m.edited && !m.discardArmed {
				m.discardArmed = true
				return m, nil
			}
			m.discardArmed = false
			m.edited = false
			m.envIndex = (m.envIndex + 1) % len(m.envNames)
			m.recompute()
			return m, nil
		case key.Matches(msg, keys.Send):
			if m.executing {
				return m, nil
			}
			if !m.canSend() {
				m.blockedFlash = true
				return m, nil
			}
			return m.startSend()
		case key.Matches(msg, keys.Top):
			m.viewport.GotoTop()
			return m, nil
		case key.Matches(msg, keys.Bottom):
			m.viewport.GotoBottom()
			return m, nil
		case key.Matches(msg, keys.Copy):
			m.copyToken++
			return m, copyToClipboardCmd(m.copyText(), m.copyToken)
		case key.Matches(msg, keys.Back):
			if m.executing {
				if m.cancel != nil {
					m.cancel()
				}
				return m, nil
			}
			return m, func() tea.Msg { return backFromRequestMsg{} }
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
	req := m.resolved
	if !m.executing && m.lastEntry != nil {
		req.Method, req.URL = m.lastEntry.Method, m.lastEntry.URL
		req.Headers, req.Body = m.lastEntry.RequestHeaders, m.lastEntry.RequestBody
	}
	return output.RenderTransaction(req, resp, note, output.Options{Color: false})
}

func (m requestModel) buildContent() (string, int) {
	var head strings.Builder

	if line := unsupportedLine(m.resolved.Unsupported); line != "" {
		head.WriteString(warnTextStyle.Render(line))
		head.WriteString("\n")
	}
	method := output.Sanitize(m.resolved.Method)
	fmt.Fprintf(&head, "%s %s\n", methodStyle(method).Render(method), output.Sanitize(m.resolved.URL))
	for _, h := range m.resolved.Headers {
		fmt.Fprintf(&head, "%s: %s\n", output.Sanitize(h.Name), output.Sanitize(h.Value))
	}
	if m.resolved.Body != "" {
		head.WriteString("\n")
		head.WriteString(output.PrettyBody([]byte(output.Sanitize(m.resolved.Body)), output.Options{Color: true}))
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
	case m.lastEntry == nil && !m.canSend():
		tail.WriteString(mutedTextStyle.Render("(not yet sent)"))
	case m.lastEntry == nil:
		tail.WriteString(mutedTextStyle.Render("(not yet sent — press enter to send)"))
	case m.lastEntry.Error != "":
		tail.WriteString(errorTextStyle.Render("Error: " + output.Sanitize(m.lastEntry.Error)))
	default:
		if cur := m.currentEnv(); cur != "" && m.lastEntry.Env != cur {
			tail.WriteString(mutedTextStyle.Render("(sent with env: " + output.Sanitize(m.lastEntry.Env) + ")"))
			tail.WriteString("\n")
		}
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
	case m.fromEntry:
		text := "resolved at send time"
		if m.entryEnv != "" {
			text += " (env: " + m.entryEnv + ")"
		}
		return mutedTextStyle.Render(text)
	case m.envErr != nil:
		return errorTextStyle.Render(dirLabel(m.root, src.Dir) + strings.ReplaceAll(m.envErr.Error(), "\n", " "))
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
	return m.envLineStyle().Render(line)
}

// currentEnv is the selected env name, or "" when no env is available.
func (m requestModel) currentEnv() string {
	if m.envIndex < 0 || m.envIndex >= len(m.envNames) {
		return ""
	}
	return m.envNames[m.envIndex]
}

func (m requestModel) envLineStyle() lipgloss.Style {
	if strings.Contains(strings.ToLower(m.currentEnv()), "prod") {
		return warnTextStyle
	}
	return lipgloss.NewStyle()
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

func (m requestModel) canSend() bool { return len(m.missingVars) == 0 }

func (m requestModel) blockedMessage() string {
	hint := "i edit"
	if m.canCycleEnv() {
		hint = "e switch env · " + hint
	}
	return "cannot send: " + describeUnresolved(m.missingVars) + " — " + hint
}

func (m requestModel) statusLine() string {
	var text string
	var style lipgloss.Style
	switch {
	case m.editing:
		text, style = m.editError, errorTextStyle
	case m.discardArmed:
		text, style = "edits will be discarded — press e again", warnTextStyle
	case m.blockedFlash:
		text, style = m.blockedMessage(), blockedHighlightStyle
	case m.copyStatus != "":
		text, style = m.copyStatus, copiedTextStyle
		if m.copyErr {
			style = errorTextStyle
		}
	case !m.canSend():
		text, style = m.blockedMessage(), errorTextStyle
	}
	text = strings.ReplaceAll(text, "\n", " ")
	w := m.viewport.Width
	if m.editing || m.viewport.TotalLineCount() <= m.viewport.Height {
		if text == "" {
			return ""
		}
		return style.Render(ansi.Truncate(text, w, "…"))
	}
	pct := fmt.Sprintf("%d%%", int(m.viewport.ScrollPercent()*100))
	msg := ""
	if text != "" {
		msg = style.Render(ansi.Truncate(text, max(0, w-len(pct)-1), "…"))
	}
	pad := max(1, w-ansi.StringWidth(msg)-len(pct))
	return msg + strings.Repeat(" ", pad) + mutedTextStyle.Render(pct)
}

func (m requestModel) summaryLine() string {
	w := m.viewport.Width
	switch {
	case m.executing:
		elapsed := time.Since(m.sendStarted).Truncate(100 * time.Millisecond)
		return ansi.Truncate(fmt.Sprintf("%s sending… %s · esc cancel", m.spinner.View(), elapsed), w, "…")
	case m.lastEntry == nil:
		return mutedTextStyle.Render("not sent")
	case m.lastEntry.Error != "":
		text := "ERR · " + output.Sanitize(strings.ReplaceAll(m.lastEntry.Error, "\n", " "))
		return errorTextStyle.Render(ansi.Truncate(text, w, "…"))
	}
	e := m.lastEntry
	status := output.Sanitize(e.Status)
	rest := fmt.Sprintf(" · %s · %s", e.Duration.Round(time.Millisecond), formatSize(len(e.ResponseBody)))
	return ansi.Truncate(statusCodeStyle(e.StatusCode).Render(status)+rest, w, "…")
}

func (m requestModel) View() string {
	body := m.viewport.View()
	if m.editing {
		body = m.editor.View()
	}
	content := ansi.Truncate(m.envLine(), m.viewport.Width, "…") + "\n" + m.summaryLine() + "\n" + body + "\n" + m.statusLine()
	return paneFocusedStyle.Width(max(0, m.width-paneFocusedStyle.GetHorizontalBorderSize())).Render(content)
}

func describeUnresolved(missing []string) string {
	var vars, files []string
	for _, entry := range missing {
		entry = output.Sanitize(entry)
		if strings.HasPrefix(entry, env.MissingFilePrefix) {
			files = append(files, entry)
		} else {
			vars = append(vars, "{{"+entry+"}}")
		}
	}
	var parts []string
	if len(vars) > 0 {
		parts = append(parts, "unresolved "+strings.Join(vars, ", "))
	}
	parts = append(parts, files...)
	return strings.Join(parts, "; ")
}

func unsupportedLine(items []string) string {
	seen := make(map[string]bool, len(items))
	var uniq []string
	for _, item := range items {
		item = output.Sanitize(item)
		if !seen[item] {
			seen[item] = true
			uniq = append(uniq, item)
		}
	}
	if len(uniq) == 0 {
		return ""
	}
	return "unsupported (ignored): " + strings.Join(uniq, ", ")
}
