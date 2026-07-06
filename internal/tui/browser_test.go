package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestDiscoverHTTPFiles_FindsHttpFilesRecursively(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.http", "GET https://example.com\n")
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	writeTestFile(t, sub, "b.http", "GET https://example.com\n")
	writeTestFile(t, dir, "notes.txt", "ignore me\n")

	got, err := discoverHTTPFiles(dir)
	if err != nil {
		t.Fatalf("discoverHTTPFiles: %v", err)
	}
	want := []string{"a.http", filepath.Join("sub", "b.http")}
	if len(got) != len(want) {
		t.Fatalf("got = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBrowserModel_TabTogglesFocusOnlyAfterFileParsed(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.http", "GET https://example.com\n")

	m := newBrowserModel(dir)
	if m.focus != paneFiles {
		t.Fatalf("initial focus = %v, want paneFiles", m.focus)
	}

	// No file selected yet: Tab should stay on paneFiles.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != paneFiles {
		t.Errorf("focus after Tab with no parsed file = %v, want paneFiles", m.focus)
	}

	// Selecting (Enter on) the file parses it and moves focus to requests.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.focus != paneRequests {
		t.Fatalf("focus after selecting file = %v, want paneRequests", m.focus)
	}
	if m.parseErr != nil {
		t.Fatalf("parseErr = %v", m.parseErr)
	}

	// Tab now toggles back to files.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != paneFiles {
		t.Errorf("focus after second Tab = %v, want paneFiles", m.focus)
	}
}

func TestBrowserModel_SelectFile_ParseErrorKeepsFocusOnFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "broken.http", "### broken\nContent-Type: application/json\n")

	m := newBrowserModel(dir)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.focus != paneFiles {
		t.Errorf("focus = %v, want paneFiles after parse error", m.focus)
	}
	if m.parseErr == nil {
		t.Error("expected parseErr to be set")
	}
}

func TestBrowserModel_EnterOnRequestEmitsOpenRequestMsg(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.http", "### getUser\nGET https://example.com/users/1\n")

	m := newBrowserModel(dir)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // select file -> parses, focuses requests

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // select request
	if cmd == nil {
		t.Fatal("expected a command emitting openRequestMsg, got nil")
	}
	msg, ok := cmd().(openRequestMsg)
	if !ok {
		t.Fatalf("expected openRequestMsg, got %T", cmd())
	}
	if msg.req.Name != "getUser" {
		t.Errorf("req.Name = %q, want getUser", msg.req.Name)
	}
	if msg.req.Method != "GET" {
		t.Errorf("req.Method = %q, want GET", msg.req.Method)
	}
}

func TestBrowserModel_SelectFile_PartialParseErrorsStillShowValidRequests(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "mixed.http", "### Get user\nGET https://example.com/users/1\n\n### Broken\nContent-Type: application/json\n\n### Create user\nPOST https://example.com/users\n")

	m := newBrowserModel(dir)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.focus != paneRequests {
		t.Fatalf("focus = %v, want paneRequests despite one broken block", m.focus)
	}
	if m.parseErr != nil {
		t.Fatalf("parseErr = %v, want nil when some requests parsed successfully", m.parseErr)
	}
	if m.parsedFile == nil || len(m.parsedFile.ParseErrors) != 1 {
		t.Fatalf("parsedFile.ParseErrors = %v, want 1 entry for the broken block", m.parsedFile.ParseErrors)
	}
	if len(m.requests.Items()) != 2 {
		t.Fatalf("requests.Items() = %d, want 2 valid requests", len(m.requests.Items()))
	}
}

func TestBrowserModel_SelectFile_EmptyFileMovesFocusWithNoError(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "empty.http", "# just a comment, no requests\n")

	m := newBrowserModel(dir)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.focus != paneRequests {
		t.Fatalf("focus = %v, want paneRequests for a file with no errors and no requests", m.focus)
	}
	if m.parseErr != nil {
		t.Fatalf("parseErr = %v, want nil", m.parseErr)
	}
	if len(m.requests.Items()) != 0 {
		t.Fatalf("requests.Items() = %d, want 0", len(m.requests.Items()))
	}
}
