package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Ahngbeom/rest-tui/internal/history"
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

	m := newBrowserModel(dir, newTestHistoryStore(t))
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

func TestBrowserModel_ViewUsesThickBorderOnlyForFocusedPane(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.http", "GET https://example.com\n")

	m := newBrowserModel(dir, newTestHistoryStore(t))
	m = m.SetSize(80, 24)
	if m.focus != paneFiles {
		t.Fatalf("precondition failed: focus = %v, want paneFiles", m.focus)
	}

	view := m.View()
	if strings.Count(view, "┏") != 1 {
		t.Errorf("View() with paneFiles focused should contain exactly one thick top-left corner (┏), got %d in %q", strings.Count(view, "┏"), view)
	}
	if strings.Count(view, "╭") != 1 {
		t.Errorf("View() with paneFiles focused should contain exactly one rounded top-left corner (╭) for the unfocused pane, got %d in %q", strings.Count(view, "╭"), view)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // select the file -> parses and focuses paneRequests
	if m.focus != paneRequests {
		t.Fatalf("precondition failed: focus = %v, want paneRequests after selecting the file", m.focus)
	}

	view = m.View()
	if strings.Count(view, "┏") != 1 {
		t.Errorf("View() with paneRequests focused should still contain exactly one thick top-left corner (┏), got %d in %q", strings.Count(view, "┏"), view)
	}
	if strings.Count(view, "╭") != 1 {
		t.Errorf("View() with paneRequests focused should contain exactly one rounded top-left corner (╭) for the now-unfocused Files pane, got %d in %q", strings.Count(view, "╭"), view)
	}
}

func TestBrowserModel_TitleStylesHighlightOnlyTheFocusedPane(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.http", "GET https://example.com\n")

	m := newBrowserModel(dir, newTestHistoryStore(t))
	if m.focus != paneFiles {
		t.Fatalf("precondition failed: focus = %v, want paneFiles", m.focus)
	}

	filesStyle, requestsStyle := m.titleStyles()
	if !reflect.DeepEqual(filesStyle, paneTitleActiveStyle) {
		t.Error("with paneFiles focused, files title style should be paneTitleActiveStyle")
	}
	if !reflect.DeepEqual(requestsStyle, paneTitleInactiveStyle) {
		t.Error("with paneFiles focused, requests title style should be paneTitleInactiveStyle")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // select the file -> parses and focuses paneRequests
	if m.focus != paneRequests {
		t.Fatalf("precondition failed: focus = %v, want paneRequests after selecting the file", m.focus)
	}

	filesStyle, requestsStyle = m.titleStyles()
	if !reflect.DeepEqual(filesStyle, paneTitleInactiveStyle) {
		t.Error("with paneRequests focused, files title style should be paneTitleInactiveStyle")
	}
	if !reflect.DeepEqual(requestsStyle, paneTitleActiveStyle) {
		t.Error("with paneRequests focused, requests title style should be paneTitleActiveStyle")
	}
}

func TestBrowserModel_SelectFile_ParseErrorKeepsFocusOnFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "broken.http", "### broken\nContent-Type: application/json\n")

	m := newBrowserModel(dir, newTestHistoryStore(t))
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

	m := newBrowserModel(dir, newTestHistoryStore(t))
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

	m := newBrowserModel(dir, newTestHistoryStore(t))
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

func TestNewBrowserModel_BadRootSetsRootErrNotParseErr(t *testing.T) {
	badRoot := filepath.Join(t.TempDir(), "does-not-exist")

	m := newBrowserModel(badRoot, newTestHistoryStore(t))

	if m.rootErr == nil {
		t.Fatal("expected rootErr to be set when the root directory cannot be scanned")
	}
	if m.parseErr != nil {
		t.Errorf("parseErr = %v, want nil (a bad root should not reuse the per-file parseErr field)", m.parseErr)
	}

	view := m.View()
	if !strings.Contains(view, "cannot scan directory") {
		t.Errorf("View() = %q, want it to show the root scan error", view)
	}
	if strings.Contains(view, "parse error:") {
		t.Error("View() should not show the per-file \"parse error:\" banner for a root-scan failure")
	}
}

func TestNewBrowserModel_PopulatesRecentListWhenHistoryExists(t *testing.T) {
	dir := t.TempDir()
	store := newTestHistoryStore(t)
	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com/a", StatusCode: 200}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	m := newBrowserModel(dir, store)

	if m.requests.Title != "Recent" {
		t.Errorf("requests.Title = %q, want %q", m.requests.Title, "Recent")
	}
	items := m.requests.Items()
	if len(items) != 1 {
		t.Fatalf("requests.Items() = %d, want 1", len(items))
	}
	if _, ok := items[0].(entryItem); !ok {
		t.Errorf("items[0] = %T, want entryItem", items[0])
	}
}

func TestNewBrowserModel_LeavesRequestsPaneEmptyWhenNoHistory(t *testing.T) {
	dir := t.TempDir()

	m := newBrowserModel(dir, newTestHistoryStore(t))

	if m.requests.Title != "Requests" {
		t.Errorf("requests.Title = %q, want %q", m.requests.Title, "Requests")
	}
	if len(m.requests.Items()) != 0 {
		t.Errorf("requests.Items() = %d, want 0", len(m.requests.Items()))
	}
}

func TestBrowserModel_TabReachesRequestsPaneWithRecentListNoFileSelected(t *testing.T) {
	dir := t.TempDir()
	store := newTestHistoryStore(t)
	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com/a"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	m := newBrowserModel(dir, store)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	if m.focus != paneRequests {
		t.Fatalf("focus = %v, want paneRequests (Recent list should be reachable via Tab)", m.focus)
	}
}

func TestBrowserModel_EnterOnRecentEntryEmitsOpenRequestFromEntryMsg(t *testing.T) {
	dir := t.TempDir()
	store := newTestHistoryStore(t)
	stored, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com/a"})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	m := newBrowserModel(dir, store)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("expected a command emitting openRequestFromEntryMsg, got nil")
	}
	msg, ok := cmd().(openRequestFromEntryMsg)
	if !ok {
		t.Fatalf("expected openRequestFromEntryMsg, got %T", cmd())
	}
	if msg.entry.URL != stored.URL {
		t.Errorf("entry.URL = %q, want %q", msg.entry.URL, stored.URL)
	}
}

func TestBrowserModel_SelectFile_ReplacesRecentListEntirely(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.http", "### getUser\nGET https://example.com/users/1\n")
	store := newTestHistoryStore(t)
	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com/a"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	m := newBrowserModel(dir, store)
	if m.requests.Title != "Recent" {
		t.Fatalf("precondition failed: requests.Title = %q, want Recent", m.requests.Title)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // select the .http file

	if m.requests.Title != "Requests" {
		t.Errorf("requests.Title = %q, want Requests after selecting a file", m.requests.Title)
	}
	items := m.requests.Items()
	if len(items) != 1 {
		t.Fatalf("requests.Items() = %d, want 1", len(items))
	}
	if _, ok := items[0].(requestItem); !ok {
		t.Errorf("items[0] = %T, want requestItem", items[0])
	}
}

func TestBrowserModel_SelectFile_EmptyFileMovesFocusWithNoError(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "empty.http", "# just a comment, no requests\n")

	m := newBrowserModel(dir, newTestHistoryStore(t))
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
