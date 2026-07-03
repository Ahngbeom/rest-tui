package tui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
