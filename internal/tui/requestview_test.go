package tui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bahn/rest-tui/internal/history"
	"github.com/bahn/rest-tui/internal/httpfile"
)

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
