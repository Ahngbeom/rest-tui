package tui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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

func applyMsg(a App, msg tea.Msg) App {
	model, _ := a.Update(msg)
	return model.(App)
}

func TestApp_StaleExecResultDoesNotLandOnNewRequestView(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("slow done")) }))
	defer srv.Close()
	a := newTestApp(t)
	a = applyMsg(a, openRequestMsg{file: &httpfile.File{}, req: httpfile.Request{Method: "GET", URL: srv.URL + "/slow"}})
	model, sendCmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a = model.(App)
	a = applyMsg(a, openRequestMsg{file: &httpfile.File{}, req: httpfile.Request{Method: "GET", URL: srv.URL + "/users"}})

	a = applyMsg(a, sendCmd())

	if a.request.lastEntry != nil {
		t.Errorf("listUsers view shows a response it never sent: %+v", a.request.lastEntry)
	}
	entries, err := a.store.List(10)
	if err != nil || len(entries) != 1 || entries[0].URL != srv.URL+"/slow" {
		t.Errorf("history = %+v, %v, want the /slow send recorded", entries, err)
	}
}

func TestApp_FooterKeepsHelpAndQuitAt80Columns(t *testing.T) {
	a := newTestApp(t) // 80x24
	a = applyMsg(a, openRequestMsg{file: &httpfile.File{}, req: httpfile.Request{Method: "GET", URL: "https://example.com"}})
	lines := strings.Split(a.View(), "\n")
	last := lines[len(lines)-1]
	if w := lipgloss.Width(last); w > 80 {
		t.Errorf("footer width = %d, want <= 80: %q", w, last)
	}
	if plain := ansi.Strip(last); !strings.HasSuffix(strings.TrimRight(plain, " "), "? help  q quit") {
		t.Errorf("footer = %q, want it to end with %q", plain, "? help  q quit")
	}
}

func helpTestApp(t *testing.T, w, h int) App {
	root := t.TempDir()
	writeTestFile(t, root, "a.http", "### one\nGET https://a.example/1\n\n### two\nGET https://a.example/2\n")
	writeTestFile(t, root, "b.http", "GET https://b.example/only\n")
	return newLayoutApp(t, root, newTestHistoryStore(t), w, h)
}

func TestApp_HelpSwallowsKeysForHiddenScreen(t *testing.T) {
	a := pressKeys(helpTestApp(t, 120, 35), "?", "j", "enter", "tab", "enter", "h", "d", "e")
	if !a.showHelp || a.screen != screenBrowser {
		t.Fatalf("showHelp = %v, screen = %v, want help still open over Browser", a.showHelp, a.screen)
	}
	if a.browser.files.Index() != 0 || a.browser.focus != paneFiles || a.browser.parsedFile != nil {
		t.Errorf("hidden Browser changed: index=%d focus=%v parsed=%v", a.browser.files.Index(), a.browser.focus, a.browser.parsedFile != nil)
	}
}

func TestApp_HelpEscClosesWithoutForwarding(t *testing.T) {
	a := pressKeys(helpTestApp(t, 120, 35), "enter", "?", "esc")
	if a.showHelp || a.browser.focus != paneRequests {
		t.Errorf("showHelp = %v, focus = %v, want help closed and esc not forwarded", a.showHelp, a.browser.focus)
	}
}

func TestApp_HelpQQuits(t *testing.T) {
	_, cmd := pressKeys(helpTestApp(t, 80, 24), "?").Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil || cmd() != tea.Quit() {
		t.Error("q inside help should quit")
	}
}

func TestApp_HelpScrollsAt80x24(t *testing.T) {
	a := pressKeys(helpTestApp(t, 80, 24), "?")
	view := ansi.Strip(a.View())
	assertViewFits(t, a.View(), 80, 24)
	first := strings.SplitN(view, "\n", 2)[0]
	if !strings.Contains(first, "rest-tui") || !strings.Contains(view, "Global") || !strings.Contains(view, "q, ctrl+c") {
		t.Fatalf("top of help is cut off:\n%s", view)
	}
	for i := 0; i < 30; i++ {
		a = pressKeys(a, "j")
	}
	if !strings.Contains(ansi.Strip(a.View()), "switch to selected directory") {
		t.Errorf("j did not scroll to the end of help")
	}
	for i := 0; i < 30; i++ {
		a = pressKeys(a, "k")
	}
	if !strings.Contains(ansi.Strip(a.View()), "Global") {
		t.Errorf("k did not scroll back to the top")
	}
}

func TestApp_HelpResizesWhileOpen(t *testing.T) {
	a := pressKeys(helpTestApp(t, 120, 35), "?")
	a = applyMsg(a, tea.WindowSizeMsg{Width: 60, Height: 20})
	assertViewFits(t, a.View(), 60, 20)
	a = pressKeys(a, "j")
	if a.helpViewport.YOffset != 1 {
		t.Errorf("helpViewport.YOffset = %d, want 1 after resize + j", a.helpViewport.YOffset)
	}
}

func TestApp_OpenRequestSearchesEnvUpToAppRoot(t *testing.T) {
	a := newTestApp(t)
	writeNestedFile(t, filepath.Join(a.root, "http-client.env.json"), threeEnvs)

	model, _ := a.Update(openRequestMsg{filePath: filepath.Join(a.root, "api", "users.http"), file: &httpfile.File{}, req: httpfile.Request{Method: "GET", URL: "{{host}}/x"}})
	a = model.(App)

	if a.request.resolved.URL != "https://dev.example.com/x" {
		t.Errorf("resolved.URL = %q", a.request.resolved.URL)
	}
}

func TestApp_EnvLineFitsTerminalWithDeepEnvPath(t *testing.T) {
	a := newTestApp(t)
	deep := filepath.Join(a.root, "services", "payments", "internal", "api", "v1")
	writeNestedFile(t, filepath.Join(deep, "http-client.env.json"), `{"staging-eu-west": {}, "dev": {}, "qa": {}}`)
	model, _ := a.Update(openRequestMsg{filePath: filepath.Join(deep, "a.http"), file: &httpfile.File{}, req: httpfile.Request{Method: "GET", URL: "https://example.com"}})
	a = model.(App)

	for _, sz := range [][2]int{{60, 20}, {80, 24}, {120, 35}} {
		model, _ = a.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		assertViewFits(t, model.(App).View(), sz[0], sz[1])
	}
}

func TestApp_SelectedEnvCarriesToNextRequest(t *testing.T) {
	a := newTestApp(t)
	writeNestedFile(t, filepath.Join(a.root, "http-client.env.json"), threeEnvs)
	writeNestedFile(t, filepath.Join(a.root, "qa", "http-client.env.json"), `{"dev": {}, "qa": {}}`)
	open := func(a App, path string) App {
		model, _ := a.Update(openRequestMsg{filePath: path, file: &httpfile.File{}, req: httpfile.Request{Method: "GET", URL: "https://example.com"}})
		return model.(App)
	}
	press := func(a App, s string) App {
		model, _ := a.Update(typeRunes(s))
		return model.(App)
	}

	a = press(open(a, filepath.Join(a.root, "a.http")), "e")
	if a.request.currentEnv() != "prod" || a.lastEnv != "prod" {
		t.Fatalf("currentEnv = %q, lastEnv = %q, want prod", a.request.currentEnv(), a.lastEnv)
	}
	if got := open(a, filepath.Join(a.root, "b.http")).request.currentEnv(); got != "prod" {
		t.Errorf("next request env = %q, want prod", got)
	}

	a = press(open(a, filepath.Join(t.TempDir(), "noenv.http")), "j")
	if a.lastEnv != "prod" {
		t.Errorf("lastEnv = %q after a request with no env file, want prod kept", a.lastEnv)
	}

	if got := open(a, filepath.Join(a.root, "qa", "c.http")).request.currentEnv(); got != "dev" {
		t.Errorf("env in folder without prod = %q, want dev", got)
	}
}

func footerLine(a App) string {
	lines := strings.Split(a.View(), "\n")
	return plainText(lines[len(lines)-1])
}

func TestApp_FooterHidesSendWhenBlocked(t *testing.T) {
	a := newTestApp(t)
	model, _ := a.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	model, _ = model.(App).Update(openRequestMsg{filePath: filepath.Join(a.root, "a.http"), file: &httpfile.File{}, req: httpfile.Request{Method: "GET", URL: "{{host}}/x"}})
	a = model.(App)

	if got := footerLine(a); strings.Contains(got, "send") {
		t.Errorf("footer = %q, want no send hint", got)
	}
	for _, sz := range [][2]int{{60, 20}, {80, 24}, {120, 35}} {
		model, _ = a.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		assertViewFits(t, model.(App).View(), sz[0], sz[1])
	}
}
