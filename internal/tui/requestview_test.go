package tui

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ahngbeom/rest-tui/internal/history"
	"github.com/Ahngbeom/rest-tui/internal/httpfile"
)

// ansiEscapePattern strips ANSI SGR escape codes (color/bold from
// output.RenderResponse) so rendered viewport content can be compared as
// plain text in tests.
var ansiEscapePattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

func newTestHistoryStore(t *testing.T) *history.Store {
	t.Helper()
	return history.NewStore(filepath.Join(t.TempDir(), "history.json"))
}

func TestNewRequestModel_ResolvesFileScopedVars(t *testing.T) {
	dir := t.TempDir()
	file := &httpfile.File{Vars: map[string]string{"baseUrl": "https://example.com"}}
	req := httpfile.Request{Method: "GET", URL: "{{baseUrl}}/users/1"}

	m := newRequestModel(filepath.Join(dir, "a.http"), "", file, req, newTestHistoryStore(t), "")

	if m.resolved.URL != "https://example.com/users/1" {
		t.Errorf("resolved.URL = %q", m.resolved.URL)
	}
	if len(m.missingVars) != 0 {
		t.Errorf("missingVars = %v, want empty", m.missingVars)
	}
	if len(m.envNames) != 0 || m.envIndex != -1 {
		t.Errorf("expected no envs, got envNames=%v envIndex=%d", m.envNames, m.envIndex)
	}
}

func TestNewRequestModel_MissingVarsBlockSend(t *testing.T) {
	dir := t.TempDir()
	req := httpfile.Request{Method: "GET", URL: "{{baseUrl}}/users/1"}

	m := newRequestModel(filepath.Join(dir, "a.http"), "", &httpfile.File{}, req, newTestHistoryStore(t), "")
	if len(m.missingVars) == 0 {
		t.Fatal("expected missingVars to be non-empty")
	}

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.executing {
		t.Error("executing should stay false when variables are unresolved")
	}
	if cmd != nil {
		t.Error("expected no command when variables are unresolved")
	}
}

func TestRequestModel_CycleEnvRecomputesResolvedVars(t *testing.T) {
	dir := t.TempDir()
	envJSON := `{
		"dev":  {"baseUrl": "https://dev.example.com"},
		"prod": {"baseUrl": "https://example.com"}
	}`
	if err := os.WriteFile(filepath.Join(dir, "http-client.env.json"), []byte(envJSON), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	req := httpfile.Request{Method: "GET", URL: "{{baseUrl}}/ping"}

	m := newRequestModel(filepath.Join(dir, "a.http"), "", &httpfile.File{}, req, newTestHistoryStore(t), "")
	if len(m.envNames) != 2 || m.envIndex != 0 {
		t.Fatalf("envNames = %v, envIndex = %d", m.envNames, m.envIndex)
	}
	firstURL := m.resolved.URL

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if m.envIndex != 1 {
		t.Fatalf("envIndex after cycle = %d, want 1", m.envIndex)
	}
	if m.resolved.URL == firstURL {
		t.Errorf("resolved.URL did not change after cycling env: %q", m.resolved.URL)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if m.envIndex != 0 {
		t.Fatalf("envIndex after wrapping cycle = %d, want 0", m.envIndex)
	}
	if m.resolved.URL != firstURL {
		t.Errorf("resolved.URL after wrapping = %q, want %q", m.resolved.URL, firstURL)
	}
}

func TestRequestModel_ExecResultMsgStopsExecutingAndSetsEntry(t *testing.T) {
	m := newRequestModel("", "", &httpfile.File{}, httpfile.Request{Method: "GET", URL: "https://example.com"}, newTestHistoryStore(t), "")
	m.executing = true

	entry := history.Entry{Method: "GET", URL: "https://example.com", StatusCode: 200}
	m, cmd := m.Update(execResultMsg{modelID: m.id, entry: entry})

	if m.executing {
		t.Error("executing should be false after execResultMsg")
	}
	if cmd != nil {
		t.Error("expected no follow-up command")
	}
	if m.lastEntry == nil || m.lastEntry.StatusCode != 200 {
		t.Errorf("lastEntry = %+v", m.lastEntry)
	}
}

func TestRequestModel_BackEmitsBackToBrowserMsg(t *testing.T) {
	m := newRequestModel("", "", &httpfile.File{}, httpfile.Request{Method: "GET", URL: "https://example.com"}, newTestHistoryStore(t), "")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command emitting backToBrowserMsg")
	}
	if _, ok := cmd().(backToBrowserMsg); !ok {
		t.Fatalf("expected backToBrowserMsg, got %T", cmd())
	}
}

func TestRequestModel_ExecResultMsgScrollsToResponseStart(t *testing.T) {
	req := httpfile.Request{
		Method: "GET",
		URL:    "https://example.com/users/1",
		Headers: []httpfile.Header{
			{Name: "Accept", Value: "application/json"},
			{Name: "Authorization", Value: "Bearer token"},
			{Name: "X-Trace-Id", Value: "abc123"},
		},
	}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")

	if m.responseLineOffset == 0 {
		t.Fatal("expected responseLineOffset to be past the request header lines")
	}

	// Simulate the user having manually scrolled up into the request section
	// while a previous response was showing.
	m.viewport.SetYOffset(1)

	entry := history.Entry{Method: "GET", URL: req.URL, StatusCode: 200, ResponseBody: "line1\nline2\nline3\nline4\nline5"}
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: entry})

	if m.viewport.YOffset != m.responseLineOffset {
		t.Errorf("YOffset = %d, want responseLineOffset %d", m.viewport.YOffset, m.responseLineOffset)
	}
}

func TestRequestModel_SendScrollsToResponseStartImmediately(t *testing.T) {
	req := httpfile.Request{
		Method: "GET",
		URL:    "https://example.com/users/1",
		Headers: []httpfile.Header{
			{Name: "Accept", Value: "application/json"},
			{Name: "Authorization", Value: "Bearer token"},
		},
	}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")

	// Pretend a prior response was showing and the user had scrolled into it.
	m.viewport.SetYOffset(1)

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.executing {
		t.Error("expected executing = true after Send")
	}
	if cmd == nil {
		t.Fatal("expected a send command")
	}
	if m.viewport.YOffset != m.responseLineOffset {
		t.Errorf("YOffset = %d, want responseLineOffset %d", m.viewport.YOffset, m.responseLineOffset)
	}
}

func TestRequestModel_SetSizePreservesScrollPosition(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com/users/1"}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")

	body := strings.Repeat("line\n", 50)
	entry := history.Entry{Method: "GET", URL: req.URL, StatusCode: 200, ResponseBody: body}
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: entry})

	m.viewport.SetYOffset(3)

	m = m.SetSize(80, 24)

	if m.viewport.YOffset != 3 {
		t.Errorf("YOffset after SetSize = %d, want 3 (scroll position should be preserved)", m.viewport.YOffset)
	}
}

func TestRequestModel_CycleEnvPreservesScrollPosition(t *testing.T) {
	dir := t.TempDir()
	envJSON := `{
		"dev":  {"baseUrl": "https://dev.example.com"},
		"prod": {"baseUrl": "https://example.com"}
	}`
	if err := os.WriteFile(filepath.Join(dir, "http-client.env.json"), []byte(envJSON), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	req := httpfile.Request{Method: "GET", URL: "{{baseUrl}}/ping"}

	m := newRequestModel(filepath.Join(dir, "a.http"), "", &httpfile.File{}, req, newTestHistoryStore(t), "")

	body := strings.Repeat("line\n", 50)
	entry := history.Entry{Method: "GET", URL: "https://dev.example.com/ping", StatusCode: 200, ResponseBody: body}
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: entry})

	m.viewport.SetYOffset(3)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})

	if m.viewport.YOffset != 3 {
		t.Errorf("YOffset after CycleEnv = %d, want 3 (scroll position should be preserved)", m.viewport.YOffset)
	}
}

func TestRequestModel_BuildContentResponseOffsetPointsPastDivider(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")

	content, offset := m.buildContent()
	lines := strings.Split(content, "\n")

	if offset <= 0 || offset >= len(lines) {
		t.Fatalf("offset = %d out of range for %d lines", offset, len(lines))
	}
	if !strings.Contains(lines[offset-2], "─") {
		t.Errorf("lines[offset-2] = %q, want it to contain the divider", lines[offset-2])
	}
	if lines[offset-1] != "" {
		t.Errorf("lines[offset-1] = %q, want a blank line before the response section", lines[offset-1])
	}
	if strings.Contains(lines[offset], "─") {
		t.Errorf("lines[offset] = %q, want the response section, not the divider", lines[offset])
	}
}

func TestRequestModel_LongResponseLineWrapsInsteadOfTruncating(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")
	m = m.SetSize(60, 20) // innerWidth = 56

	longURL := "https://example.com/very/long/path/that/goes/on/and/on/and/should/be/wider/than/the/viewport"
	body := `{"url": "` + longURL + `"}`
	entry := history.Entry{Method: "GET", URL: "https://example.com", StatusCode: 200, ResponseBody: body}
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: entry})

	view := ansiEscapePattern.ReplaceAllString(m.viewport.View(), "")
	var joined strings.Builder
	for _, line := range strings.Split(view, "\n") {
		joined.WriteString(strings.TrimRight(line, " "))
	}
	if !strings.Contains(joined.String(), longURL) {
		t.Errorf("viewport.View() does not contain the full URL unbroken; got:\n%s", view)
	}
}

func TestRequestModel_LongResponseLineIsOneClickableHyperlink(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")
	m = m.SetSize(60, 20) // innerWidth = 56

	longURL := "https://example.com/very/long/path/that/goes/on/and/on/and/should/be/wider/than/the/viewport"
	body := `{"url": "` + longURL + `"}`
	entry := history.Entry{Method: "GET", URL: "https://example.com", StatusCode: 200, ResponseBody: body}
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: entry})

	view := m.viewport.View()
	open := "\x1b]8;;" + longURL + "\x1b\\"
	const closeSeq = "\x1b]8;;\x1b\\"
	// Other incidental URLs in the view (e.g. the request line's own URL)
	// get their own independent hyperlink pair, so only assert that this
	// specific URL's open marker is well-formed and followed by a close.
	if n := strings.Count(view, open); n != 1 {
		t.Errorf("OSC8 open marker count for the response URL = %d, want 1; got:\n%s", n, view)
	}
	openIdx := strings.Index(view, open)
	if openIdx == -1 || !strings.Contains(view[openIdx:], closeSeq) {
		t.Errorf("OSC8 open marker for the response URL has no matching close after it; got:\n%s", view)
	}
}

func TestNewRequestModelFromEntry_SkipsEnvUIAndSendsImmediately(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"replayed":true}`))
	}))
	defer srv.Close()

	store := newTestHistoryStore(t)
	entry := history.Entry{Method: "GET", URL: srv.URL}

	m, cmd := newRequestModelFromEntry(entry, store)
	if len(m.envNames) != 0 {
		t.Errorf("envNames = %v, want empty for a rerun", m.envNames)
	}
	if !m.executing {
		t.Error("expected executing=true immediately for a rerun")
	}
	if m.viewport.YOffset != m.responseLineOffset {
		t.Errorf("YOffset = %d, want responseLineOffset %d", m.viewport.YOffset, m.responseLineOffset)
	}
	if cmd == nil {
		t.Fatal("expected a send command")
	}

	msg, ok := cmd().(execResultMsg)
	if !ok {
		t.Fatalf("expected execResultMsg, got %T", cmd())
	}
	if msg.entry.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", msg.entry.StatusCode)
	}
	if msg.entry.ResponseBody != `{"replayed":true}` {
		t.Errorf("ResponseBody = %q", msg.entry.ResponseBody)
	}
	if msg.modelID != m.id {
		t.Fatalf("msg.modelID = %d, want %d", msg.modelID, m.id)
	}
	m, _ = m.Update(msg)
	if m.lastEntry == nil || m.lastEntry.StatusCode != http.StatusOK {
		t.Errorf("rerun result not shown: lastEntry = %+v", m.lastEntry)
	}
}

func TestRequestModel_CopyTextNotYetSent(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")

	got := m.copyText()
	if !strings.Contains(got, "not yet sent") {
		t.Errorf("copyText() = %q, want it to mention not yet sent", got)
	}
	if strings.Contains(got, "\x1b[") {
		t.Errorf("copyText() = %q, should not contain ANSI escapes", got)
	}
}

func TestRequestModel_CopyTextAfterExecResult(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")

	entry := history.Entry{Method: "GET", URL: req.URL, StatusCode: 200, ResponseBody: `{"ok":true}`}
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: entry})

	got := m.copyText()
	if !strings.Contains(got, "GET https://example.com") {
		t.Errorf("copyText() = %q, want it to contain the request line", got)
	}
	if !strings.Contains(got, `"ok": true`) {
		t.Errorf("copyText() = %q, want it to contain the pretty response body", got)
	}
	if strings.Contains(got, "\x1b[") {
		t.Errorf("copyText() = %q, should not contain ANSI escapes", got)
	}
}

func TestRequestModel_CopyTextAfterError(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")

	entry := history.Entry{Method: "GET", URL: req.URL, Error: "connection refused"}
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: entry})

	got := m.copyText()
	if !strings.Contains(got, "Error: connection refused") {
		t.Errorf("copyText() = %q, want it to contain the error", got)
	}
}

func TestRequestModel_CopyKeyReturnsCommandWithoutInvokingIt(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if cmd == nil {
		t.Fatal("expected a copy command")
	}
	if m.copyToken != 1 {
		t.Errorf("copyToken = %d, want 1", m.copyToken)
	}
	// Do not invoke cmd(): it would write to the real OS clipboard.
}

func TestRequestModel_ClipboardCopyMsgSetsStatus(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")
	m.copyToken = 1

	m, cmd := m.Update(clipboardCopyMsg{token: 1, err: nil})
	if m.copyStatus != "copied to clipboard" || m.copyErr {
		t.Errorf("copyStatus = %q, copyErr = %v", m.copyStatus, m.copyErr)
	}
	if cmd == nil {
		t.Fatal("expected a command to clear the status later")
	}

	m2, _ := m.Update(clipboardCopyMsg{token: 1, err: errors.New("boom")})
	if !m2.copyErr || !strings.Contains(m2.copyStatus, "boom") {
		t.Errorf("copyStatus = %q, copyErr = %v", m2.copyStatus, m2.copyErr)
	}
}

func TestRequestModel_ClipboardCopyMsgIgnoresStaleToken(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")
	m.copyToken = 2

	m, _ = m.Update(clipboardCopyMsg{token: 1, err: nil})
	if m.copyStatus != "" {
		t.Errorf("copyStatus = %q, want empty for a stale token", m.copyStatus)
	}
}

func TestRequestModel_ClipboardCopyExpiredMsgClearsStatus(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")
	m.copyToken = 1
	m.copyStatus = "copied to clipboard"

	m, _ = m.Update(clipboardCopyExpiredMsg{token: 0})
	if m.copyStatus == "" {
		t.Error("stale expiry token should not clear a newer status")
	}

	m, _ = m.Update(clipboardCopyExpiredMsg{token: 1})
	if m.copyStatus != "" {
		t.Errorf("copyStatus = %q, want cleared", m.copyStatus)
	}
}

func TestRequestModel_EditKeySeedsEditorWithSerializedResolved(t *testing.T) {
	req := httpfile.Request{
		Method:  "POST",
		URL:     "https://example.com/users",
		Headers: []httpfile.Header{{Name: "Content-Type", Value: "application/json"}},
		Body:    `{"name":"Ahn"}`,
	}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})

	if !m.editing {
		t.Fatal("expected editing = true after pressing i")
	}
	want := serializeRequest(m.resolved)
	if got := m.editor.Value(); got != want {
		t.Errorf("editor.Value() = %q, want %q", got, want)
	}
	if !strings.HasPrefix(m.editor.Value(), "POST https://example.com/users\n") {
		t.Errorf("editor.Value() = %q, want it to start with the request line", m.editor.Value())
	}
}

func TestRequestModel_ApplyEditValidUpdatesResolvedAndClearsMissingVars(t *testing.T) {
	dir := t.TempDir()
	req := httpfile.Request{Method: "GET", URL: "{{missing}}/old"}
	m := newRequestModel(filepath.Join(dir, "a.http"), "", &httpfile.File{}, req, newTestHistoryStore(t), "")
	if len(m.missingVars) == 0 {
		t.Fatal("precondition failed: expected missingVars to be non-empty")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m.editor.SetValue("GET https://new.example.com/x\nX-Foo: bar\n")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})

	if m.editing {
		t.Fatal("expected editing = false after applying a valid edit")
	}
	if m.resolved.URL != "https://new.example.com/x" {
		t.Errorf("resolved.URL = %q", m.resolved.URL)
	}
	if len(m.resolved.Headers) != 1 || m.resolved.Headers[0].Name != "X-Foo" || m.resolved.Headers[0].Value != "bar" {
		t.Errorf("resolved.Headers = %+v", m.resolved.Headers)
	}
	if len(m.missingVars) != 0 {
		t.Errorf("missingVars = %v, want empty after edit", m.missingVars)
	}
}

func TestRequestModel_ApplyEditParseErrorStaysInEditModeWithMessage(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")
	origResolved := m.resolved

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m.editor.SetValue("GET https://example.com\nbad header line\n")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})

	if !m.editing {
		t.Fatal("expected editing to stay true after a parse error")
	}
	if !strings.HasPrefix(m.editError, "line ") {
		t.Errorf("editError = %q, want it to start with %q", m.editError, "line ")
	}
	if !reflect.DeepEqual(m.resolved, origResolved) {
		t.Errorf("resolved = %+v, want unchanged %+v", m.resolved, origResolved)
	}
}

func TestRequestModel_ApplyEditWrongRequestCountStaysInEditModeWithMessage(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{name: "zero requests", text: "// just a comment\n", want: "expected exactly one request, got 0"},
		{name: "two requests", text: "GET https://a.example.com\n### second\nGET https://b.example.com\n", want: "expected exactly one request, got 2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httpfile.Request{Method: "GET", URL: "https://example.com"}
			m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")

			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
			m.editor.SetValue(tc.text)

			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})

			if !m.editing {
				t.Fatal("expected editing to stay true when request count is wrong")
			}
			if m.editError != tc.want {
				t.Errorf("editError = %q, want %q", m.editError, tc.want)
			}
		})
	}
}

func TestRequestModel_EscCancelsEditWithoutMutatingResolved(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")
	origResolved := m.resolved

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m.editor.SetValue("GET https://changed.example.com\n")

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.editing {
		t.Fatal("expected editing = false after esc")
	}
	if !reflect.DeepEqual(m.resolved, origResolved) {
		t.Errorf("resolved = %+v, want unchanged %+v", m.resolved, origResolved)
	}
	if cmd != nil {
		if _, ok := cmd().(backToBrowserMsg); ok {
			t.Fatal("esc while editing should not emit backToBrowserMsg")
		}
	}
}

func TestRequestModel_BackspaceWhileEditingDeletesCharacterInsteadOfCancelling(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	before := m.editor.Value()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})

	if !m.editing {
		t.Fatal("expected editing to stay true after backspace")
	}
	if len(m.editor.Value()) != len(before)-1 {
		t.Errorf("editor.Value() = %q (len %d), want len %d", m.editor.Value(), len(m.editor.Value()), len(before)-1)
	}
}

func TestRequestModel_IgnoresExecResultFromAnotherModel(t *testing.T) {
	store := newTestHistoryStore(t)
	a := newRequestModel("", "", &httpfile.File{}, httpfile.Request{Method: "GET", URL: "https://a.example"}, store, "")
	b := newRequestModel("", "", &httpfile.File{}, httpfile.Request{Method: "GET", URL: "https://b.example"}, store, "")
	if a.id == 0 || a.id == b.id {
		t.Fatalf("ids = %d, %d, want distinct non-zero", a.id, b.id)
	}
	b, cmd := b.Update(execResultMsg{modelID: a.id, entry: history.Entry{StatusCode: 200}})
	if b.lastEntry != nil || cmd != nil {
		t.Errorf("lastEntry = %+v, cmd = %v, want the foreign result ignored", b.lastEntry, cmd)
	}
}

func TestRequestModel_ViewportWidthMatchesPaneInnerWidth(t *testing.T) {
	m := newRequestModel("", "", &httpfile.File{}, httpfile.Request{Method: "GET", URL: "https://example.com"}, newTestHistoryStore(t), "").SetSize(60, 20)
	if want := 60 - paneFocusedStyle.GetHorizontalFrameSize(); m.viewport.Width != want {
		t.Errorf("viewport.Width = %d, want %d", m.viewport.Width, want)
	}
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: history.Entry{StatusCode: 200, ResponseBody: strings.Repeat("x", 100)}})
	view := m.View()
	assertViewFits(t, view, 60, 20)
	var runs []int
	for _, r := range regexp.MustCompile(`x{10,}`).FindAllString(ansi.Strip(view), -1) {
		runs = append(runs, len(r))
	}
	if !reflect.DeepEqual(runs, []int{56, 44}) {
		t.Errorf("x runs = %v, want [56 44] (wrapped exactly once)", runs)
	}
}

func TestRequestModel_StatusRowIsReservedForCopyAndEditError(t *testing.T) {
	m := newRequestModel("", "", &httpfile.File{}, httpfile.Request{Method: "GET", URL: "https://example.com"}, newTestHistoryStore(t), "").SetSize(80, 24)
	if h := lipgloss.Height(m.View()); h != 24 {
		t.Fatalf("idle height = %d, want 24", h)
	}
	m, _ = m.Update(clipboardCopyMsg{token: m.copyToken})
	if h := lipgloss.Height(m.View()); h != 24 || !strings.Contains(m.statusLine(), "copied to clipboard") {
		t.Errorf("after copy: height = %d, statusLine = %q", h, m.statusLine())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m.editor.SetValue("GET https://example.com\nbad header line\n")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if h := lipgloss.Height(m.View()); h != 24 || !strings.HasPrefix(ansi.Strip(m.statusLine()), "line 2:") {
		t.Errorf("after edit error: height = %d, statusLine = %q", h, m.statusLine())
	}
}

func TestRequestModel_TabIndentedBodyEndStaysReachable(t *testing.T) {
	m := newRequestModel("", "", &httpfile.File{}, httpfile.Request{Method: "GET", URL: "https://example.com"}, newTestHistoryStore(t), "").SetSize(60, 20)
	body := strings.Repeat("\t\t\t<p>"+strings.Repeat("word ", 9)+"</p>\n", 30) + "\t\tTHE-END"
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: history.Entry{StatusCode: 200, Status: "200 OK", ResponseBody: body}})
	m.viewport.GotoBottom()
	if v := ansi.Strip(m.View()); !strings.Contains(v, "THE-END") {
		t.Errorf("last body line unreachable after scrolling to the bottom:\n%s", v)
	}
}

func writeNestedFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func typeRunes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func plainText(s string) string { return ansiEscapePattern.ReplaceAllString(s, "") }

const threeEnvs = `{"dev": {"host": "https://dev.example.com"}, "prod": {"host": "https://prod.example.com"}, "staging": {"host": "https://staging.example.com"}}`

func openAt(t *testing.T, root, rel, preferred string) requestModel {
	t.Helper()
	req := httpfile.Request{Method: "GET", URL: "{{host}}/x"}
	return newRequestModel(filepath.Join(root, rel), root, &httpfile.File{}, req, newTestHistoryStore(t), preferred).SetSize(120, 30)
}

func TestDirLabel(t *testing.T) {
	cases := []struct{ root, dir, want string }{
		{"/r", "/r", "./"},
		{"/r/", "/r", "./"},
		{"/r", "/r/api/v1", "api/v1/"},
		{".", "api", "api/"},
		{"./", ".", "./"},
		{"", "/x", "/x/"},
		{"/r", "/other", "/other/"},
	}
	for _, c := range cases {
		if got := dirLabel(c.root, c.dir); got != c.want {
			t.Errorf("dirLabel(%q, %q) = %q, want %q", c.root, c.dir, got, c.want)
		}
	}
}

func TestRequestModel_EnvLineFromAncestorFolder(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), threeEnvs)

	m := openAt(t, root, "api/users.http", "")

	if m.resolved.URL != "https://dev.example.com/x" {
		t.Errorf("resolved.URL = %q", m.resolved.URL)
	}
	if got, want := plainText(m.envLine()), "env: dev (1/3) · ./http-client.env.json — e cycle"; got != want {
		t.Errorf("envLine = %q, want %q", got, want)
	}
}

func TestRequestModel_EnvLineShowsNearestFileRelativeToRoot(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "api", "http-client.env.json"), threeEnvs)

	m := openAt(t, root, "api/users.http", "")
	if got, want := plainText(m.envLine()), "env: dev (1/3) · api/http-client.env.json — e cycle"; got != want {
		t.Errorf("envLine = %q, want %q", got, want)
	}
}

func TestRequestModel_EnvLineSingleEnvHasNoCycleHint(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), `{"dev": {}}`)

	m := openAt(t, root, "a.http", "")
	if got, want := plainText(m.envLine()), "env: dev (1/1) · ./http-client.env.json"; got != want {
		t.Errorf("envLine = %q, want %q", got, want)
	}
	if m.canCycleEnv() {
		t.Error("canCycleEnv() = true with a single env")
	}
}

func TestRequestModel_EnvLineSharedOnly(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), `{"$shared": {"host": "https://shared.example.com"}}`)

	m := openAt(t, root, "a.http", "")
	if m.resolved.URL != "https://shared.example.com/x" {
		t.Errorf("resolved.URL = %q", m.resolved.URL)
	}
	if got, want := plainText(m.envLine()), "env: (none) · ./http-client.env.json"; got != want {
		t.Errorf("envLine = %q, want %q", got, want)
	}
}

func TestRequestModel_EnvLineListsSearchedFoldersWhenNotFound(t *testing.T) {
	root := t.TempDir()
	m := openAt(t, root, "api/users.http", "")
	if got, want := plainText(m.envLine()), "no http-client.env.json found (searched: api/, ./)"; got != want {
		t.Errorf("envLine = %q, want %q", got, want)
	}
}

func TestRequestModel_EnvLineShowsParseError(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), `{ broken`)

	m := openAt(t, root, "a.http", "")
	if got := plainText(m.envLine()); !strings.HasPrefix(got, "./http-client.env.json: ") {
		t.Errorf("envLine = %q, want prefix %q", got, "./http-client.env.json: ")
	}
}

func TestRequestModel_EnvLineParseErrorNamesAncestorFolder(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "api", "http-client.env.json"), `{ broken`)

	m := openAt(t, root, "api/v1/a.http", "")
	got := plainText(m.envLine())
	if !strings.HasPrefix(got, "api/http-client.env.json: ") || strings.Contains(got, "http-client.env.json: http-client.env.json") {
		t.Errorf("envLine = %q, want prefix %q once", got, "api/http-client.env.json: ")
	}
}

func TestNewRequestModelFromEntry_UnresolvedRerunIsNotSent(t *testing.T) {
	m, cmd := newRequestModelFromEntry(history.Entry{Method: "GET", URL: "https://example.com/{{id}}"}, newTestHistoryStore(t))
	if cmd != nil {
		t.Error("cmd != nil, want no send for an unresolved entry")
	}
	if m.executing {
		t.Error("executing = true, want false")
	}
	if got := plainText(m.SetSize(120, 30).statusLine()); !strings.Contains(got, "cannot send") {
		t.Errorf("statusLine = %q, want cannot-send notice", got)
	}
}

func TestNewRequestModel_PreferredEnvWinsWhenPresent(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), threeEnvs)

	if got := openAt(t, root, "a.http", "staging").currentEnv(); got != "staging" {
		t.Errorf("currentEnv = %q, want staging", got)
	}
	if got := openAt(t, root, "a.http", "qa").currentEnv(); got != "dev" {
		t.Errorf("currentEnv = %q, want dev (alphabetical first)", got)
	}
}

func TestRequestModel_ProdEnvUsesWarningStyle(t *testing.T) {
	for name, warn := range map[string]bool{"prod": true, "Production": true, "preprod-eu": true, "staging": false} {
		root := t.TempDir()
		writeNestedFile(t, filepath.Join(root, "http-client.env.json"), `{"`+name+`": {}}`)
		m := openAt(t, root, "a.http", "")
		if got := m.envLineStyle().GetForeground() == colorWarning; got != warn {
			t.Errorf("%s: warning style = %v, want %v", name, got, warn)
		}
	}
}

func TestRequestModel_UnresolvedVarsBlockSendWithGuidance(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "{{host}}/x", Headers: []httpfile.Header{{Name: "Authorization", Value: "Bearer {{token}}"}}}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "").SetSize(200, 30)
	want := "cannot send: unresolved {{host}}, {{token}} — e switch env · i edit"

	if m.canSend() {
		t.Fatal("canSend() = true with unresolved vars")
	}
	if got := plainText(m.statusLine()); !strings.Contains(got, want) {
		t.Errorf("statusLine = %q, want it to contain %q", got, want)
	}
	if content, _ := m.buildContent(); strings.Contains(content, "press enter to send") || strings.Contains(content, "unresolved variables:") {
		t.Errorf("content still invites sending or repeats the notice: %q", plainText(content))
	}

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.executing || !m.blockedFlash {
		t.Fatalf("after enter: cmd=%v executing=%v blockedFlash=%v", cmd != nil, m.executing, m.blockedFlash)
	}
	if got := plainText(m.statusLine()); !strings.Contains(got, want) {
		t.Errorf("statusLine after enter = %q", got)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.blockedFlash {
		t.Error("blockedFlash should clear on the next key")
	}
}

func TestRequestModel_CanSendAfterSwitchingToCompleteEnv(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), `{"a": {}, "b": {"host": "https://b.example.com"}}`)
	m := openAt(t, root, "x.http", "")
	if m.canSend() {
		t.Fatal("env a lacks host; canSend() should be false")
	}
	m, _ = m.Update(typeRunes("e"))
	if !m.canSend() {
		t.Error("env b defines host; canSend() should be true")
	}
}

func applyTestEdit(t *testing.T, m requestModel, text string) requestModel {
	t.Helper()
	m, _ = m.Update(typeRunes("i"))
	m.editor.SetValue(text)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.editing {
		t.Fatalf("edit not applied: %s", m.editError)
	}
	return m
}

func TestRequestModel_ApplyEditKeepsLeftoverPlaceholdersBlocked(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "{{host}}/x"}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "").SetSize(200, 30)

	m = applyTestEdit(t, m, "GET https://ok.example.com/x\nAuthorization: Bearer {{token}}\n")
	if m.canSend() {
		t.Fatal("canSend() = true; the edit still contains {{token}}")
	}
	if got := plainText(m.statusLine()); !strings.Contains(got, "cannot send: unresolved {{token}} — e switch env · i edit") {
		t.Errorf("statusLine = %q", got)
	}

	m = applyTestEdit(t, m, "GET https://ok.example.com/x\nAuthorization: Bearer abc\n")
	if !m.canSend() {
		t.Errorf("canSend() = false after removing every placeholder; missingVars = %v", m.missingVars)
	}
}

func TestRequestModel_CycleEnvAfterEditNeedsSecondPress(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), threeEnvs)
	m := applyTestEdit(t, openAt(t, root, "a.http", ""), "GET https://edited.example.com/x\n")

	m, _ = m.Update(typeRunes("e"))
	if m.currentEnv() != "dev" || m.resolved.URL != "https://edited.example.com/x" {
		t.Fatalf("first e changed state: env=%q url=%q", m.currentEnv(), m.resolved.URL)
	}
	if got := plainText(m.statusLine()); !strings.Contains(got, "edits will be discarded — press e again") {
		t.Errorf("statusLine = %q", got)
	}

	m, _ = m.Update(typeRunes("e"))
	if m.currentEnv() != "prod" || m.resolved.URL != "https://prod.example.com/x" {
		t.Errorf("second e: env=%q url=%q", m.currentEnv(), m.resolved.URL)
	}
	if got := plainText(m.statusLine()); strings.Contains(got, "edits will be discarded") {
		t.Errorf("prompt still shown after switching: %q", got)
	}
}

func TestRequestModel_OtherKeyDisarmsEditDiscard(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), threeEnvs)
	m := applyTestEdit(t, openAt(t, root, "a.http", ""), "GET https://edited.example.com/x\n")

	m, _ = m.Update(typeRunes("e"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(typeRunes("e"))
	if m.currentEnv() != "dev" {
		t.Errorf("env = %q, want dev (prompt must re-arm after another key)", m.currentEnv())
	}
}

func TestRequestModel_CycleEnvIsNoOpWithSingleEnvKeepsEdit(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), `{"dev": {"host": "https://dev.example.com"}}`)
	m := applyTestEdit(t, openAt(t, root, "a.http", ""), "GET https://edited.example.com/x\n")

	m, _ = m.Update(typeRunes("e"))
	if m.resolved.URL != "https://edited.example.com/x" || !m.edited {
		t.Errorf("e with one env discarded edit: url=%q edited=%v", m.resolved.URL, m.edited)
	}
	if got := plainText(m.statusLine()); strings.Contains(got, "edits will be discarded") {
		t.Errorf("prompt shown with a single env: %q", got)
	}
}

func TestRequestModel_SendRecordsEnvInHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), `{"dev": {"host": "`+srv.URL+`"}}`)
	store := newTestHistoryStore(t)
	m := newRequestModel(filepath.Join(root, "a.http"), root, &httpfile.File{}, httpfile.Request{Method: "GET", URL: "{{host}}/x"}, store, "")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := cmd().(execResultMsg)
	if msg.entry.Env != "dev" {
		t.Errorf("entry.Env = %q, want dev", msg.entry.Env)
	}
	if got, _ := store.List(1); got[0].Env != "dev" {
		t.Errorf("stored Env = %q, want dev", got[0].Env)
	}
}

func TestRequestModel_RerunRecordsOriginalEnv(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	_, cmd := newRequestModelFromEntry(history.Entry{Method: "GET", URL: srv.URL, Env: "staging"}, newTestHistoryStore(t))
	if got := cmd().(execResultMsg).entry.Env; got != "staging" {
		t.Errorf("entry.Env = %q, want staging", got)
	}
}

func TestRequestModel_SentWithEnvShownAfterSwitch(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), threeEnvs)
	m := openAt(t, root, "a.http", "")
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: history.Entry{Env: "dev", StatusCode: 200, Status: "200 OK"}})

	if content, _ := m.buildContent(); strings.Contains(content, "sent with env") {
		t.Errorf("note shown before switching env: %q", plainText(content))
	}
	m, _ = m.Update(typeRunes("e"))
	if content, _ := m.buildContent(); !strings.Contains(plainText(content), "(sent with env: dev)") {
		t.Errorf("content = %q, want (sent with env: dev)", plainText(content))
	}
}

func TestRequestModelFromEntry_EnvLineShowsSendTimeEnv(t *testing.T) {
	store := newTestHistoryStore(t)
	m := newRequestModelFromEntryNoSend(history.Entry{Method: "GET", URL: "https://example.com", Env: "dev"}, store)
	if got, want := plainText(m.envLine()), "resolved at send time (env: dev)"; got != want {
		t.Errorf("envLine = %q, want %q", got, want)
	}
	m, _ = newRequestModelFromEntry(history.Entry{Method: "GET", URL: "https://example.com"}, store)
	if got, want := plainText(m.envLine()), "resolved at send time"; got != want {
		t.Errorf("envLine = %q, want %q", got, want)
	}
}

var uuidV4Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewRequestModel_ResolvesDynamicVariableInPreview(t *testing.T) {
	dir := t.TempDir()
	req := httpfile.Request{Method: "GET", URL: "https://example.com", Headers: []httpfile.Header{{Name: "X-Request-Id", Value: "{{$uuid}}"}}}
	m := newRequestModel(filepath.Join(dir, "a.http"), dir, &httpfile.File{}, req, newTestHistoryStore(t), "")

	if len(m.missingVars) != 0 || !uuidV4Pattern.MatchString(m.resolved.Headers[0].Value) {
		t.Errorf("missingVars = %v, header = %q", m.missingVars, m.resolved.Headers[0].Value)
	}
	if !m.canSend() {
		t.Error("canSend() = false, want true")
	}
}

func TestRequestModel_ApplyEditResolvesDynamicVariables(t *testing.T) {
	m := newRequestModel("", "", &httpfile.File{}, httpfile.Request{Method: "GET", URL: "https://example.com"}, newTestHistoryStore(t), "").SetSize(120, 30)
	m = applyTestEdit(t, m, "GET https://example.com\nX-Request-Id: {{$uuid}}\nAuthorization: Bearer {{token}}\n")

	if got := m.resolved.Headers[0].Value; !uuidV4Pattern.MatchString(got) {
		t.Errorf("X-Request-Id = %q, want the generated UUID that will be sent", got)
	}
	if m.resolved.Headers[1].Value != "Bearer {{token}}" || !reflect.DeepEqual(m.missingVars, []string{"token"}) {
		t.Errorf("Authorization = %q, missingVars = %v; a plain leftover placeholder must stay literal and block", m.resolved.Headers[1].Value, m.missingVars)
	}
}

func TestRequestModel_ApplyEditAddsHTTPScheme(t *testing.T) {
	m := newRequestModel("", "", &httpfile.File{}, httpfile.Request{Method: "GET", URL: "https://example.com"}, newTestHistoryStore(t), "")
	m, _ = m.startEditing()
	m.editor.SetValue("GET localhost:8080/x\n")
	m = m.applyEdit()
	if m.editing || m.resolved.URL != "http://localhost:8080/x" {
		t.Errorf("editing = %v, URL = %q", m.editing, m.resolved.URL)
	}
}
