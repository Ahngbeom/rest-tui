package tui

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
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

func TestRequestModel_BackEmitsBackFromRequestMsg(t *testing.T) {
	m := newRequestModel("", "", &httpfile.File{}, httpfile.Request{Method: "GET", URL: "https://example.com"}, newTestHistoryStore(t), "")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command emitting backFromRequestMsg")
	}
	if _, ok := cmd().(backFromRequestMsg); !ok {
		t.Fatalf("expected backFromRequestMsg, got %T", cmd())
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
		if _, ok := cmd().(backFromRequestMsg); ok {
			t.Fatal("esc while editing should not emit backFromRequestMsg")
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

func TestApp_UnresolvedRerunMsgIsNotSent(t *testing.T) {
	a := newTestApp(t)
	model, cmd := a.Update(rerunMsg{entry: history.Entry{Method: "GET", URL: "https://example.com/{{id}}"}})
	m := model.(App).request
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

func TestDescribeSendError_Timeout(t *testing.T) {
	err := fmt.Errorf("Get \"http://x\": %w", context.DeadlineExceeded)
	if got, want := describeSendError(err, 30*time.Second), "timed out after 30s (use -timeout to change)"; got != want {
		t.Errorf("describeSendError(timeout) = %q, want %q", got, want)
	}
	if got := describeSendError(errors.New("boom"), 30*time.Second); got != "boom" {
		t.Errorf("describeSendError(boom) = %q, want boom", got)
	}
}

func TestRequestModel_SendTimeoutDefaultsTo30s(t *testing.T) {
	m := newTestRequestModel(t, httpfile.Request{Method: "GET", URL: "http://x"})
	if got := m.sendTimeout(); got != 30*time.Second {
		t.Errorf("sendTimeout() = %v, want 30s", got)
	}
	if got := m.withTimeout(5 * time.Second).sendTimeout(); got != 5*time.Second {
		t.Errorf("withTimeout(5s).sendTimeout() = %v, want 5s", got)
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
	want := "cannot send: unresolved {{host}}, {{token}} — i edit"

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
	if got := plainText(m.statusLine()); !strings.Contains(got, "cannot send: unresolved {{token}} — i edit") {
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
	msg := awaitMsg[execResultMsg](t, cmd)
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

	m := newRequestModelFromEntryNoSend(history.Entry{Method: "GET", URL: srv.URL, Env: "staging"}, newTestHistoryStore(t))
	_, cmd := m.startSend()
	if got := awaitMsg[execResultMsg](t, cmd).entry.Env; got != "staging" {
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
	m = newRequestModelFromEntryNoSend(history.Entry{Method: "GET", URL: "https://example.com"}, store)
	m, _ = m.startSend()
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

func TestDescribeUnresolved(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"token"}, "unresolved {{token}}"},
		{[]string{"a", "b"}, "unresolved {{a}}, {{b}}"},
		{[]string{"$foo"}, "unresolved {{$foo}}"},
		{[]string{"missing file: ./body.json"}, "missing file: ./body.json"},
		{[]string{"token", "missing file: ./body.json"}, "unresolved {{token}}; missing file: ./body.json"},
	}
	for _, c := range cases {
		if got := describeUnresolved(c.in); got != c.want {
			t.Errorf("describeUnresolved(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRequestModel_ShowsUnsupportedWarningAndStillSends(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com",
		Unsupported: []string{"response handler script", "output redirect: ./out.json", "response handler script"}}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")
	m = m.SetSize(120, 30)

	content, _ := m.buildContent()
	plain := ansiEscapePattern.ReplaceAllString(content, "")
	if !strings.HasPrefix(plain, "unsupported (ignored): response handler script, output redirect: ./out.json\n") {
		t.Errorf("content does not start with the unsupported warning:\n%s", plain)
	}
	if !m.canSend() {
		t.Fatal("canSend() = false; unsupported items must not block sending")
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil {
		t.Error("Enter returned no command")
	}
}

func TestRequestModel_IncludesBodyFileRelativeToHTTPFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "body.json"), []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	req := httpfile.Request{Method: "POST", URL: "https://example.com", Body: "< ./body.json"}
	m := newRequestModel(filepath.Join(dir, "a.http"), dir, &httpfile.File{}, req, newTestHistoryStore(t), "")
	if m.resolved.Body != `{"a":1}` || !m.canSend() {
		t.Errorf("Body = %q, canSend = %v", m.resolved.Body, m.canSend())
	}
}

func TestRequestModel_MissingBodyFileBlocksSend(t *testing.T) {
	dir := t.TempDir()
	req := httpfile.Request{Method: "POST", URL: "https://example.com", Body: "< ./body.json"}
	m := newRequestModel(filepath.Join(dir, "a.http"), dir, &httpfile.File{}, req, newTestHistoryStore(t), "")
	m = m.SetSize(120, 30)

	if m.canSend() {
		t.Fatal("canSend() = true with a missing body file")
	}
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.executing {
		t.Error("Enter must not send when the body file is missing")
	}
	status := ansiEscapePattern.ReplaceAllString(m.statusLine(), "")
	if !strings.Contains(status, "cannot send: missing file: ./body.json — i edit") {
		t.Errorf("statusLine() = %q", status)
	}
}

func TestRequestModel_PreviewSanitizesIncludedFileContent(t *testing.T) {
	dir := t.TempDir()
	raw := "ok\x1b]0;pwned\x07\x1b[2J\x00end"
	if err := os.WriteFile(filepath.Join(dir, "blob.bin"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	req := httpfile.Request{Method: "POST", URL: "https://example.com", Body: "< ./blob.bin"}
	m := newRequestModel(filepath.Join(dir, "a.http"), dir, &httpfile.File{}, req, newTestHistoryStore(t), "")

	if m.resolved.Body != raw {
		t.Errorf("resolved.Body = %q, the sent body must stay byte-for-byte", m.resolved.Body)
	}
	content, _ := m.buildContent()
	for _, bad := range []string{"\x1b]0;", "\x1b[2J", "\x00"} {
		if strings.Contains(content, bad) {
			t.Errorf("preview contains %q", bad)
		}
	}
}

func dynamicTestModel(t *testing.T, url string) requestModel {
	t.Helper()
	dir := t.TempDir()
	req := httpfile.Request{Method: "GET", URL: url, Headers: []httpfile.Header{{Name: "X-Request-Id", Value: "{{$uuid}}"}}}
	m := newRequestModel(filepath.Join(dir, "a.http"), dir, &httpfile.File{}, req, newTestHistoryStore(t), "")
	m.now = func() time.Time { return time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC) }
	m.rng = rand.New(rand.NewPCG(1, 2))
	m.recompute()
	return m.SetSize(120, 30)
}

func TestRequestModel_SendsDisplayedDynamicValue(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("X-Request-Id")
	}))
	defer srv.Close()

	m := dynamicTestModel(t, srv.URL)
	shown := m.resolved.Headers[0].Value
	content, _ := m.buildContent()
	if !strings.Contains(ansiEscapePattern.ReplaceAllString(content, ""), "X-Request-Id: "+shown) {
		t.Fatalf("preview does not show %q", shown)
	}
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	res := awaitMsg[execResultMsg](t, cmd)
	if sent := <-got; sent != shown {
		t.Errorf("sent X-Request-Id = %q, preview showed %q", sent, shown)
	}
	if res.modelID != m.id {
		t.Errorf("modelID = %d, want %d", res.modelID, m.id)
	}
}

func TestRequestModel_RegeneratesDynamicValuesAfterSend(t *testing.T) {
	m := dynamicTestModel(t, "https://example.com")
	shown := m.resolved.Headers[0].Value

	m = m.SetSize(80, 24)
	if m.resolved.Headers[0].Value != shown {
		t.Fatal("SetSize regenerated the dynamic value")
	}
	m.executing = true
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: history.Entry{Method: "GET", URL: "https://example.com", StatusCode: 200,
		RequestHeaders: []httpfile.Header{{Name: "X-Request-Id", Value: shown}}}})
	next := m.resolved.Headers[0].Value
	if next == shown || !uuidV4Pattern.MatchString(next) {
		t.Errorf("after send header = %q, want a fresh UUID different from %q", next, shown)
	}
}

func TestRequestModel_StaleResultDoesNotRegenerate(t *testing.T) {
	m := dynamicTestModel(t, "https://example.com")
	shown := m.resolved.Headers[0].Value
	m, _ = m.Update(execResultMsg{modelID: m.id + 1, entry: history.Entry{StatusCode: 200}})
	if m.resolved.Headers[0].Value != shown {
		t.Error("a result for another Request view regenerated this view's values")
	}
}

func TestRequestModel_KeepsEditedValuesAfterSend(t *testing.T) {
	m := dynamicTestModel(t, "https://example.com")
	m, _ = m.startEditing()
	m.editor.SetValue("GET https://example.com\nX-Request-Id: fixed\n")
	m = m.applyEdit()
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: history.Entry{StatusCode: 200}})
	if m.resolved.Headers[0].Value != "fixed" {
		t.Errorf("header = %q, applied edits must survive a send", m.resolved.Headers[0].Value)
	}
}

func TestRequestModel_CopyTextUsesSentRequestAfterRegenerate(t *testing.T) {
	m := dynamicTestModel(t, "https://example.com")
	shown := m.resolved.Headers[0].Value
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: history.Entry{Method: "GET", URL: "https://example.com", StatusCode: 200,
		RequestHeaders: []httpfile.Header{{Name: "X-Request-Id", Value: shown}}, ResponseBody: `{"ok":true}`}})
	if m.resolved.Headers[0].Value == shown {
		t.Fatal("precondition: the dynamic value was not regenerated after the send")
	}
	got := m.copyText()
	if !strings.Contains(got, "X-Request-Id: "+shown) || strings.Contains(got, m.resolved.Headers[0].Value) {
		t.Errorf("copyText() = %q, want the sent value %q", got, shown)
	}
	m.executing = true
	if got := m.copyText(); !strings.Contains(got, "X-Request-Id: "+m.resolved.Headers[0].Value) {
		t.Errorf("copyText() while resending = %q, want the request being sent", got)
	}
}

func TestUnsupportedLineAndBlockedMessageStripEscapes(t *testing.T) {
	evil := "\x1b]0;PWNED\x07\x1b[2J"
	if got := unsupportedLine([]string{"output redirect: ./" + evil + "x"}); got == "" || strings.Contains(got, "\x1b") {
		t.Errorf("unsupportedLine() = %q", got)
	}
	m := requestModel{missingVars: []string{"missing file: ./" + evil + "x", "va" + evil + "r"}}
	if got := m.blockedMessage(); strings.Contains(got, "\x1b") {
		t.Errorf("blockedMessage() = %q", got)
	}
}

func applyUnchangedEdit(t *testing.T, m requestModel) requestModel {
	t.Helper()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.editing || m.editError != "" {
		t.Fatalf("apply failed: editing = %v, editError = %q", m.editing, m.editError)
	}
	return m
}

func TestRequestModel_UnchangedEditKeepsIncludedBodyAndWarnings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "inc.txt"), []byte("intro\n> quoted\ntail"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := httpfile.Request{Method: "POST", URL: "https://example.com", Body: "< ./inc.txt",
		Unsupported: []string{"response handler script"}}
	m := newRequestModel(filepath.Join(dir, "a.http"), dir, &httpfile.File{}, req, newTestHistoryStore(t), "")
	before := m.resolved

	m = applyUnchangedEdit(t, m)

	if m.resolved.Body != before.Body {
		t.Errorf("Body = %q, want %q", m.resolved.Body, before.Body)
	}
	if !reflect.DeepEqual(m.resolved.Unsupported, before.Unsupported) {
		t.Errorf("Unsupported = %v, want %v", m.resolved.Unsupported, before.Unsupported)
	}
	if !m.canSend() {
		t.Errorf("canSend() = false, missingVars = %v", m.missingVars)
	}
}

func TestRequestModel_UnchangedEditDoesNotReincludeNestedPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "inc.txt"), []byte("head\n< ./other"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other"), []byte("OTHER"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := httpfile.Request{Method: "POST", URL: "https://example.com", Body: "< ./inc.txt"}
	m := newRequestModel(filepath.Join(dir, "a.http"), dir, &httpfile.File{}, req, newTestHistoryStore(t), "")

	m = applyUnchangedEdit(t, m)

	if want := "head\n< ./other"; m.resolved.Body != want {
		t.Errorf("Body = %q, want %q", m.resolved.Body, want)
	}
}

func TestRequestModel_PreviewSanitizesRequestHead(t *testing.T) {
	const osc = "\x1b]0;X\x07"
	req := httpfile.Request{Method: "GET", URL: "https://example.com/" + osc,
		Headers: []httpfile.Header{{Name: "X-A", Value: "v" + osc}}}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")

	if content, _ := m.buildContent(); strings.Contains(content, "\x1b]") {
		t.Errorf("preview contains OSC sequence: %q", content)
	}
	if !strings.Contains(m.resolved.URL, osc) || !strings.Contains(m.resolved.Headers[0].Value, osc) {
		t.Error("resolved request must keep the bytes it sends")
	}
}

func TestRequestModel_SummaryLineAfterResponse(t *testing.T) {
	m := newTestRequestModel(t, httpfile.Request{Method: "POST", URL: "http://x/users"})
	entry := history.Entry{StatusCode: 201, Status: "201 Created", Duration: 4*time.Millisecond + 300*time.Microsecond,
		ResponseBody: strings.Repeat("x", 1229)}
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: entry})
	if got, want := ansi.Strip(m.summaryLine()), "201 Created · 4ms · 1.2 KB"; got != want {
		t.Errorf("summaryLine = %q, want %q", got, want)
	}
	if !strings.Contains(m.View(), ansi.Strip(m.summaryLine())) {
		t.Error("View does not contain the summary line")
	}
}

func TestRequestModel_SummaryLineBeforeSendAndOnError(t *testing.T) {
	m := newTestRequestModel(t, httpfile.Request{Method: "GET", URL: "http://x/"})
	if got := ansi.Strip(m.summaryLine()); got != "not sent" {
		t.Errorf("summaryLine before send = %q, want %q", got, "not sent")
	}
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: history.Entry{Error: "boom"}})
	if got := ansi.Strip(m.summaryLine()); got != "ERR · boom" {
		t.Errorf("summaryLine on error = %q, want %q", got, "ERR · boom")
	}
}

func TestRequestModel_SummaryLineErrorTruncatedToWidth(t *testing.T) {
	m := newTestRequestModel(t, httpfile.Request{Method: "GET", URL: "http://x/"})
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: history.Entry{Error: strings.Repeat("boom ", 100)}})
	if w := ansi.StringWidth(m.summaryLine()); w > m.viewport.Width {
		t.Errorf("summaryLine width = %d, want <= %d", w, m.viewport.Width)
	}
}

func TestRequestModel_ShowsRedirectsInResponse(t *testing.T) {
	m := newTestRequestModel(t, httpfile.Request{Method: "GET", URL: "http://x/old"})
	entry := history.Entry{StatusCode: 200, Status: "200 OK", Redirects: []string{"302 → GET http://x/new"}}
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: entry})
	if !strings.Contains(ansi.Strip(m.viewport.View()), "redirected: 302 → GET http://x/new") {
		t.Errorf("viewport missing redirect line:\n%s", ansi.Strip(m.viewport.View()))
	}
	if !strings.Contains(ansi.Strip(renderEntryDetail(entry)), "redirected: 302 → GET http://x/new") {
		t.Error("renderEntryDetail missing redirect line")
	}
}

func TestRequestModel_SendCmdRecordsRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/new", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	m := newTestRequestModel(t, httpfile.Request{Method: "GET", URL: srv.URL + "/old"})
	msg := m.sendCmd()().(execResultMsg)
	if len(msg.entry.Redirects) != 1 || !strings.Contains(msg.entry.Redirects[0], "/new") {
		t.Errorf("entry.Redirects = %v, want one hop to /new", msg.entry.Redirects)
	}
}

func TestRequestModel_TopBottomKeysAndScrollPercent(t *testing.T) {
	m := newTestRequestModel(t, httpfile.Request{Method: "GET", URL: "http://x/"}).SetSize(80, 20)
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: history.Entry{StatusCode: 200, Status: "200 OK", ResponseBody: strings.Repeat("line\n", 200)}})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	if !m.viewport.AtBottom() {
		t.Error("G: viewport not at bottom")
	}
	if got := ansi.Strip(m.statusLine()); !strings.HasSuffix(got, "100%") {
		t.Errorf("G: statusLine = %q, want suffix 100%%", got)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if m.viewport.YOffset != 0 {
		t.Errorf("g: YOffset = %d, want 0", m.viewport.YOffset)
	}
	if got := ansi.Strip(m.statusLine()); !strings.HasSuffix(got, "0%") {
		t.Errorf("g: statusLine = %q, want suffix 0%%", got)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if !m.viewport.AtBottom() {
		t.Error("end: viewport not at bottom")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyHome})
	if m.viewport.YOffset != 0 {
		t.Errorf("home: YOffset = %d, want 0", m.viewport.YOffset)
	}
}

func TestRequestModel_ScrollPercentKeepsMessageAndFitsWidth(t *testing.T) {
	m := newTestRequestModel(t, httpfile.Request{Method: "GET", URL: "http://x/"}).SetSize(60, 20)
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: history.Entry{StatusCode: 200, Status: "200 OK", ResponseBody: strings.Repeat("line\n", 200)}})
	m, _ = m.Update(clipboardCopyMsg{token: m.copyToken, err: errors.New(strings.Repeat("x", 200))})
	got := ansi.Strip(m.statusLine())
	if !strings.HasPrefix(got, "copy failed") || !strings.HasSuffix(got, "%") {
		t.Errorf("statusLine = %q, want copy message then percent", got)
	}
	if w := ansi.StringWidth(got); w != m.viewport.Width {
		t.Errorf("statusLine width = %d, want %d", w, m.viewport.Width)
	}
}

func TestRequestModel_StatusLineOmitsPercentWhenContentFits(t *testing.T) {
	m := newTestRequestModel(t, httpfile.Request{Method: "GET", URL: "http://x/"}).SetSize(120, 35)
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: history.Entry{StatusCode: 200, Status: "200 OK", ResponseBody: "ok"}})
	if got := ansi.Strip(m.statusLine()); strings.Contains(got, "%") {
		t.Errorf("statusLine = %q, want no percent", got)
	}
}

func TestRequestModel_MethodColoredInRequestLine(t *testing.T) {
	m := newTestRequestModel(t, httpfile.Request{Method: "DELETE", URL: "http://x/users/1"})
	if !strings.Contains(m.viewport.View(), methodStyle("DELETE").Render("DELETE")) {
		t.Error("request line method is not rendered with methodStyle")
	}
	if strings.Contains(m.copyText(), "\x1b[") {
		t.Error("copyText must stay uncolored")
	}
}

func TestRequestModel_StatusLineOmitsPercentWhileEditing(t *testing.T) {
	m := newTestRequestModel(t, httpfile.Request{Method: "GET", URL: "http://x/"}).SetSize(80, 20)
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: history.Entry{StatusCode: 200, Status: "200 OK", ResponseBody: strings.Repeat("line\n", 200)}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if got := ansi.Strip(m.statusLine()); strings.Contains(got, "%") {
		t.Errorf("statusLine while editing = %q, want no percent", got)
	}
	if got := ansi.Strip(m.View()); !strings.Contains(got, "200 OK") {
		t.Error("summary line should stay visible while editing")
	}
}

func TestDescribeSendError_Canceled(t *testing.T) {
	if got := describeSendError(fmt.Errorf("x: %w", context.Canceled), 30*time.Second); got != "canceled" {
		t.Errorf("describeSendError = %q, want canceled", got)
	}
}

func TestRequestModel_SendStoresCancelOnReturnedModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	m := newTestRequestModel(t, httpfile.Request{Method: "GET", URL: srv.URL})
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.isSending() || m.cancel == nil || m.sendStarted.IsZero() {
		t.Errorf("isSending = %v, cancel set = %v, sendStarted = %v", m.isSending(), m.cancel != nil, m.sendStarted)
	}
	_ = awaitMsg[spinner.TickMsg](t, cmd)
}

func TestRequestModel_SpinnerTickStopsWhenNotSending(t *testing.T) {
	m := newTestRequestModel(t, httpfile.Request{Method: "GET", URL: "http://x/"})
	m.executing = true
	_, cmd := m.Update(spinner.TickMsg{ID: m.spinner.ID()})
	if cmd == nil {
		t.Error("tick while sending should schedule the next tick")
	}
	m.executing = false
	_, cmd = m.Update(spinner.TickMsg{ID: m.spinner.ID()})
	if cmd != nil {
		t.Error("tick after sending should stop the chain")
	}
}

func TestRequestModel_SummaryLineWhileSendingShowsElapsed(t *testing.T) {
	m := newTestRequestModel(t, httpfile.Request{Method: "GET", URL: "http://x/"})
	m.executing = true
	m.sendStarted = time.Now().Add(-2500 * time.Millisecond)
	if got := ansi.Strip(m.summaryLine()); !regexp.MustCompile(`sending… 2\.[5-9]s · esc cancel$`).MatchString(got) {
		t.Errorf("summaryLine = %q", got)
	}
}

func TestRequestModel_EscWhileSendingCancelsAndRecordsCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()
	store := newTestHistoryStore(t)
	m := newRequestModel("", "", &httpfile.File{}, httpfile.Request{Method: "GET", URL: srv.URL}, store, "").SetSize(120, 35)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m, back := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if back != nil {
		t.Error("esc while sending should not navigate back")
	}
	if !m.isSending() {
		t.Error("esc should leave the model sending until the result arrives")
	}
	msg := awaitMsg[execResultMsg](t, cmd)
	if msg.entry.Error != "canceled" {
		t.Errorf("entry.Error = %q, want canceled", msg.entry.Error)
	}
	if got, err := store.List(1); err != nil || len(got) != 1 || got[0].Error != "canceled" {
		t.Errorf("history = %+v, %v, want one canceled entry", got, err)
	}
	m, _ = m.Update(msg)
	if m.isSending() || m.cancel != nil {
		t.Errorf("isSending = %v, cancel set = %v after result", m.isSending(), m.cancel != nil)
	}
	if got := ansi.Strip(m.summaryLine()); got != "ERR · canceled" {
		t.Errorf("summaryLine = %q, want %q", got, "ERR · canceled")
	}
}

func TestRequestModel_BlockedMessageOmitsEnvHintWhenEnvCannotCycle(t *testing.T) {
	m := newRequestModelFromEntryNoSend(history.Entry{Method: "GET", URL: "{{host}}/x"}, newTestHistoryStore(t)).SetSize(200, 30)
	if m.canCycleEnv() || m.canSend() {
		t.Fatalf("precondition: canCycleEnv = %v, canSend = %v", m.canCycleEnv(), m.canSend())
	}
	if got, want := m.blockedMessage(), "cannot send: unresolved {{host}} — i edit"; got != want {
		t.Errorf("blockedMessage() = %q, want %q", got, want)
	}
}

func TestRequestModel_BlockedMessageKeepsEnvHintWhenEnvCanCycle(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), `{"a": {}, "b": {}}`)
	m := openAt(t, root, "x.http", "")
	if !m.canCycleEnv() {
		t.Fatal("precondition: expected two envs to cycle through")
	}
	if got, want := m.blockedMessage(), "cannot send: unresolved {{host}} — e switch env · i edit"; got != want {
		t.Errorf("blockedMessage() = %q, want %q", got, want)
	}
}

func TestRequestModel_ResultLinesSanitizeErrorAndSentEnv(t *testing.T) {
	const osc = "\x1b]0;X\x07"
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), threeEnvs)
	m := openAt(t, root, "x.http", "")
	for _, e := range []history.Entry{{Error: "boom" + osc}, {StatusCode: 200, Env: "other" + osc}} {
		m.lastEntry = &e
		if content, _ := m.buildContent(); strings.Contains(content, "\x1b]0;") || strings.Contains(content, "\x07") {
			t.Errorf("content for %+v contains OSC sequence: %q", e, content)
		}
	}
}
