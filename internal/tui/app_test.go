package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Ahngbeom/rest-tui/internal/dirhistory"
	"github.com/Ahngbeom/rest-tui/internal/history"
	"github.com/Ahngbeom/rest-tui/internal/httpfile"
)

func newTestApp(t *testing.T) App {
	t.Helper()
	dir := t.TempDir()
	store := history.NewStore(filepath.Join(t.TempDir(), "history.json"))
	dirStore := dirhistory.NewStore(filepath.Join(t.TempDir(), "dirs.json"))
	a := NewApp(dir, store, dirStore)
	model, _ := a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return model.(App)
}

func TestApp_OpenRequestMsgSwitchesToRequestScreen(t *testing.T) {
	a := newTestApp(t)

	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	model, _ := a.Update(openRequestMsg{filePath: "a.http", file: &httpfile.File{}, req: req})
	a = model.(App)

	if a.screen != screenRequest {
		t.Fatalf("screen = %v, want screenRequest", a.screen)
	}
	if a.request.req.URL != "https://example.com" {
		t.Errorf("request.req.URL = %q", a.request.req.URL)
	}
}

func TestApp_BackToBrowserMsgSwitchesToBrowserScreen(t *testing.T) {
	a := newTestApp(t)
	model, _ := a.Update(openRequestMsg{filePath: "a.http", file: &httpfile.File{}, req: httpfile.Request{Method: "GET", URL: "https://example.com"}})
	a = model.(App)
	if a.screen != screenRequest {
		t.Fatalf("precondition failed: screen = %v", a.screen)
	}

	model, _ = a.Update(backToBrowserMsg{})
	a = model.(App)
	if a.screen != screenBrowser {
		t.Errorf("screen = %v, want screenBrowser", a.screen)
	}
}

func TestApp_ToggleLogKeySwitchesToHistoryScreen(t *testing.T) {
	a := newTestApp(t)

	model, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	a = model.(App)

	if a.screen != screenHistory {
		t.Fatalf("screen = %v, want screenHistory", a.screen)
	}
	if cmd != nil {
		t.Error("expected no follow-up command from switching to History")
	}
}

func TestApp_QuitKeyReturnsQuitCmd(t *testing.T) {
	a := newTestApp(t)

	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected a quit command")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Errorf("cmd() = %v, want tea.Quit()", msg)
	}
}

func TestApp_HelpKeyTogglesShowHelp(t *testing.T) {
	a := newTestApp(t)

	model, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	a = model.(App)
	if !a.showHelp {
		t.Fatal("expected showHelp = true after pressing ?")
	}

	model, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	a = model.(App)
	if a.showHelp {
		t.Error("expected showHelp = false after pressing ? again")
	}
}

func TestApp_SurfacesHistoryCorruptionWarningAfterDetection(t *testing.T) {
	dir := t.TempDir()
	histPath := filepath.Join(t.TempDir(), "history.json")
	if err := os.WriteFile(histPath, []byte("{ not valid json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	store := history.NewStore(histPath)
	dirStore := dirhistory.NewStore(filepath.Join(t.TempDir(), "dirs.json"))
	a := NewApp(dir, store, dirStore)
	model, _ := a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a = model.(App)

	if a.statusMsg != "" {
		t.Fatalf("statusMsg = %q, want empty before anything has read the history file", a.statusMsg)
	}

	// "h" jumps to the History screen, which calls store.List -> load(),
	// discovering the corruption for the first time.
	model, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	a = model.(App)

	if a.statusMsg == "" {
		t.Fatal("expected statusMsg to surface the corruption warning once History has loaded")
	}
}

func TestApp_DirectoriesKeySwitchesToDirHistoryScreen(t *testing.T) {
	a := newTestApp(t)

	model, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	a = model.(App)

	if a.screen != screenDirHistory {
		t.Fatalf("screen = %v, want screenDirHistory", a.screen)
	}
	if cmd != nil {
		t.Error("expected no follow-up command from switching to Directory History")
	}
}

func TestApp_EditingRequestRoutesHKeyToEditorNotHistory(t *testing.T) {
	a := newTestApp(t)
	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	model, _ := a.Update(openRequestMsg{filePath: "a.http", file: &httpfile.File{}, req: req})
	a = model.(App)

	model, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	a = model.(App)
	if !a.request.editing {
		t.Fatal("precondition failed: expected request to be in editing mode")
	}

	model, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	a = model.(App)

	if a.screen != screenRequest {
		t.Fatalf("screen = %v, want screenRequest (h should not switch screens while editing)", a.screen)
	}
	if !strings.Contains(a.request.editor.Value(), "h") {
		t.Errorf("editor.Value() = %q, want it to contain the typed \"h\"", a.request.editor.Value())
	}
}

func TestApp_EditingRequestRoutesDKeyToEditorNotDirHistory(t *testing.T) {
	a := newTestApp(t)
	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	model, _ := a.Update(openRequestMsg{filePath: "a.http", file: &httpfile.File{}, req: req})
	a = model.(App)

	model, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	a = model.(App)

	model, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	a = model.(App)

	if a.screen != screenRequest {
		t.Fatalf("screen = %v, want screenRequest (d should not switch screens while editing)", a.screen)
	}
	if !strings.Contains(a.request.editor.Value(), "d") {
		t.Errorf("editor.Value() = %q, want it to contain the typed \"d\"", a.request.editor.Value())
	}
}

func TestApp_OpenRequestFromEntryMsgLandsOnRequestViewWithoutSending(t *testing.T) {
	a := newTestApp(t)

	entry := history.Entry{Method: "GET", URL: "https://example.com/recent"}
	model, cmd := a.Update(openRequestFromEntryMsg{entry: entry})
	a = model.(App)

	if a.screen != screenRequest {
		t.Fatalf("screen = %v, want screenRequest", a.screen)
	}
	if a.request.executing {
		t.Error("expected executing = false (should land on screen without auto-sending)")
	}
	if cmd != nil {
		t.Error("expected no follow-up command (no auto-send)")
	}
	if a.request.req.URL != entry.URL {
		t.Errorf("request.req.URL = %q, want %q", a.request.req.URL, entry.URL)
	}
}

func TestApp_BackToBrowserMsgRefreshesRecentListWhenNoFileSelected(t *testing.T) {
	dir := t.TempDir()
	store := history.NewStore(filepath.Join(t.TempDir(), "history.json"))
	dirStore := dirhistory.NewStore(filepath.Join(t.TempDir(), "dirs.json"))
	a := NewApp(dir, store, dirStore)
	model, _ := a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a = model.(App)

	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com/a"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	model, _ = a.Update(backToBrowserMsg{})
	a = model.(App)

	if a.browser.requests.Title != "Recent" {
		t.Errorf("browser.requests.Title = %q, want Recent", a.browser.requests.Title)
	}
	if len(a.browser.requests.Items()) != 1 {
		t.Errorf("browser.requests.Items() = %d, want 1", len(a.browser.requests.Items()))
	}
}

func TestApp_BackToBrowserMsgDoesNotClobberSelectedFileRequestList(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.http"), []byte("GET https://example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	store := history.NewStore(filepath.Join(t.TempDir(), "history.json"))
	dirStore := dirhistory.NewStore(filepath.Join(t.TempDir(), "dirs.json"))
	a := NewApp(dir, store, dirStore)
	model, _ := a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a = model.(App)

	// Select the .http file so the Requests pane shows that file's requests.
	model, _ = a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a = model.(App)
	if a.browser.parsedFile == nil {
		t.Fatalf("precondition failed: expected a file to be selected")
	}

	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com/a"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	req := httpfile.Request{Method: "GET", URL: "https://example.com"}
	model, _ = a.Update(openRequestMsg{filePath: "a.http", file: &httpfile.File{}, req: req})
	a = model.(App)

	model, _ = a.Update(backToBrowserMsg{})
	a = model.(App)

	if a.browser.requests.Title != "Requests" {
		t.Errorf("browser.requests.Title = %q, want Requests (should not be overwritten by Recent)", a.browser.requests.Title)
	}
}

func TestApp_NewAppTouchesDirHistoryOnStartup(t *testing.T) {
	dir := t.TempDir()
	store := history.NewStore(filepath.Join(t.TempDir(), "history.json"))
	dirStore := dirhistory.NewStore(filepath.Join(t.TempDir(), "dirs.json"))
	NewApp(dir, store, dirStore)

	entries, err := dirStore.List(0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 || entries[0].Path != dir {
		t.Fatalf("entries = %+v, want a single entry for %q", entries, dir)
	}
}

func TestApp_SwitchDirMsgRescansBrowserAndUpdatesRoot(t *testing.T) {
	a := newTestApp(t)

	newDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(newDir, "b.http"), []byte("GET https://example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	model, _ := a.Update(switchDirMsg{path: newDir})
	a = model.(App)

	if a.root != newDir {
		t.Errorf("root = %q, want %q", a.root, newDir)
	}
	if a.screen != screenBrowser {
		t.Fatalf("screen = %v, want screenBrowser", a.screen)
	}
	items := a.browser.files.Items()
	if len(items) != 1 {
		t.Fatalf("expected browser to be re-scanned with 1 file, got %d", len(items))
	}
	if fi, ok := items[0].(fileItem); !ok || fi.rel != "b.http" {
		t.Errorf("items[0] = %+v, want b.http", items[0])
	}

	entries, err := a.dirStore.List(0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if entries[0].Path != newDir {
		t.Errorf("most-recent entry = %+v, want %q", entries[0], newDir)
	}
}

func TestApp_SurfacesDirHistoryCorruptionWarning(t *testing.T) {
	dir := t.TempDir()
	store := history.NewStore(filepath.Join(t.TempDir(), "history.json"))
	dirsPath := filepath.Join(t.TempDir(), "dirs.json")
	dirStore := dirhistory.NewStore(dirsPath)
	// NewApp's startup Touch would otherwise overwrite this corrupted file,
	// so corrupt it only after the app (and its startup Touch) already exist.
	a := NewApp(dir, store, dirStore)
	model, _ := a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a = model.(App)

	if err := os.WriteFile(dirsPath, []byte("{ not valid json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if a.statusMsg != "" {
		t.Fatalf("statusMsg = %q, want empty before anything has re-read the corrupted file", a.statusMsg)
	}

	// "d" jumps to the Directory History screen, which calls store.List ->
	// load(), discovering the corruption for the first time.
	model, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	a = model.(App)

	if a.statusMsg == "" {
		t.Fatal("expected statusMsg to surface the corruption warning once Directory History has loaded")
	}
}
