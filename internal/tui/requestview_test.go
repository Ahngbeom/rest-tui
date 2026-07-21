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

	m := newRequestModel(filepath.Join(dir, "a.http"), file, req, newTestHistoryStore(t))

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

	m := newRequestModel(filepath.Join(dir, "a.http"), &httpfile.File{}, req, newTestHistoryStore(t))
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

	m := newRequestModel(filepath.Join(dir, "a.http"), &httpfile.File{}, req, newTestHistoryStore(t))
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
	m := newRequestModel("", &httpfile.File{}, httpfile.Request{Method: "GET", URL: "https://example.com"}, newTestHistoryStore(t))
	m.executing = true

	entry := history.Entry{Method: "GET", URL: "https://example.com", StatusCode: 200}
	m, cmd := m.Update(execResultMsg{entry: entry})

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
	m := newRequestModel("", &httpfile.File{}, httpfile.Request{Method: "GET", URL: "https://example.com"}, newTestHistoryStore(t))

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
	m := newRequestModel("", &httpfile.File{}, req, newTestHistoryStore(t))

	if m.responseLineOffset == 0 {
		t.Fatal("expected responseLineOffset to be past the request header lines")
	}

	// Simulate the user having manually scrolled up into the request section
	// while a previous response was showing.
	m.viewport.SetYOffset(1)

	entry := history.Entry{Method: "GET", URL: req.URL, StatusCode: 200, ResponseBody: "line1\nline2\nline3\nline4\nline5"}
	m, _ = m.Update(execResultMsg{entry: entry})

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
	m := newRequestModel("", &httpfile.File{}, req, newTestHistoryStore(t))

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
	m := newRequestModel("", &httpfile.File{}, req, newTestHistoryStore(t))

	body := strings.Repeat("line\n", 50)
	entry := history.Entry{Method: "GET", URL: req.URL, StatusCode: 200, ResponseBody: body}
	m, _ = m.Update(execResultMsg{entry: entry})

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

	m := newRequestModel(filepath.Join(dir, "a.http"), &httpfile.File{}, req, newTestHistoryStore(t))

	body := strings.Repeat("line\n", 50)
	entry := history.Entry{Method: "GET", URL: "https://dev.example.com/ping", StatusCode: 200, ResponseBody: body}
	m, _ = m.Update(execResultMsg{entry: entry})

	m.viewport.SetYOffset(3)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})

	if m.viewport.YOffset != 3 {
		t.Errorf("YOffset after CycleEnv = %d, want 3 (scroll position should be preserved)", m.viewport.YOffset)
	}
}

func TestRequestModel_BuildContentResponseOffsetPointsPastDivider(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	m := newRequestModel("", &httpfile.File{}, req, newTestHistoryStore(t))

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
	m := newRequestModel("", &httpfile.File{}, req, newTestHistoryStore(t))
	m = m.SetSize(60, 20) // innerWidth = 56

	longURL := "https://example.com/very/long/path/that/goes/on/and/on/and/should/be/wider/than/the/viewport"
	body := `{"url": "` + longURL + `"}`
	entry := history.Entry{Method: "GET", URL: "https://example.com", StatusCode: 200, ResponseBody: body}
	m, _ = m.Update(execResultMsg{entry: entry})

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
	m := newRequestModel("", &httpfile.File{}, req, newTestHistoryStore(t))
	m = m.SetSize(60, 20) // innerWidth = 56

	longURL := "https://example.com/very/long/path/that/goes/on/and/on/and/should/be/wider/than/the/viewport"
	body := `{"url": "` + longURL + `"}`
	entry := history.Entry{Method: "GET", URL: "https://example.com", StatusCode: 200, ResponseBody: body}
	m, _ = m.Update(execResultMsg{entry: entry})

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
}

func TestRequestModel_CopyTextNotYetSent(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	m := newRequestModel("", &httpfile.File{}, req, newTestHistoryStore(t))

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
	m := newRequestModel("", &httpfile.File{}, req, newTestHistoryStore(t))

	entry := history.Entry{Method: "GET", URL: req.URL, StatusCode: 200, ResponseBody: `{"ok":true}`}
	m, _ = m.Update(execResultMsg{entry: entry})

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
	m := newRequestModel("", &httpfile.File{}, req, newTestHistoryStore(t))

	entry := history.Entry{Method: "GET", URL: req.URL, Error: "connection refused"}
	m, _ = m.Update(execResultMsg{entry: entry})

	got := m.copyText()
	if !strings.Contains(got, "Error: connection refused") {
		t.Errorf("copyText() = %q, want it to contain the error", got)
	}
}

func TestRequestModel_CopyKeyReturnsCommandWithoutInvokingIt(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	m := newRequestModel("", &httpfile.File{}, req, newTestHistoryStore(t))

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
	m := newRequestModel("", &httpfile.File{}, req, newTestHistoryStore(t))
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
	m := newRequestModel("", &httpfile.File{}, req, newTestHistoryStore(t))
	m.copyToken = 2

	m, _ = m.Update(clipboardCopyMsg{token: 1, err: nil})
	if m.copyStatus != "" {
		t.Errorf("copyStatus = %q, want empty for a stale token", m.copyStatus)
	}
}

func TestRequestModel_ClipboardCopyExpiredMsgClearsStatus(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	m := newRequestModel("", &httpfile.File{}, req, newTestHistoryStore(t))
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
	m := newRequestModel("", &httpfile.File{}, req, newTestHistoryStore(t))

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
	m := newRequestModel(filepath.Join(dir, "a.http"), &httpfile.File{}, req, newTestHistoryStore(t))
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
	m := newRequestModel("", &httpfile.File{}, req, newTestHistoryStore(t))
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
			m := newRequestModel("", &httpfile.File{}, req, newTestHistoryStore(t))

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
	m := newRequestModel("", &httpfile.File{}, req, newTestHistoryStore(t))
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
	m := newRequestModel("", &httpfile.File{}, req, newTestHistoryStore(t))

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
