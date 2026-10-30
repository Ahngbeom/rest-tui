package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ahngbeom/rest-tui/internal/history"
	"github.com/Ahngbeom/rest-tui/internal/httpfile"
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

	got, skipped, err := discoverHTTPFiles(dir)
	if err != nil || skipped != 0 {
		t.Fatalf("discoverHTTPFiles: skipped=%d err=%v", skipped, err)
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

func TestBrowserModel_SelectFileResetsRequestCursorToFirst(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.http", "### one\nGET https://a.example/1\n\n### two\nGET https://a.example/2\n\n### three\nGET https://a.example/3\n")
	writeTestFile(t, dir, "b.http", "GET https://b.example/only\n")
	m := newBrowserModel(dir, newTestHistoryStore(t)).SetSize(120, 35)
	key := func(m browserModel, k tea.KeyMsg) browserModel { m, _ = m.Update(k); return m }
	j := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}

	m = key(m, tea.KeyMsg{Type: tea.KeyEnter}) // open a.http
	m = key(key(m, j), j)
	if m.requests.Index() != 2 {
		t.Fatalf("precondition: requests.Index() = %d, want 2", m.requests.Index())
	}
	m = key(m, tea.KeyMsg{Type: tea.KeyEsc})
	m = key(m, j)
	m = key(m, tea.KeyMsg{Type: tea.KeyEnter}) // open b.http

	if m.requests.Index() != 0 {
		t.Errorf("requests.Index() = %d, want 0 after switching files", m.requests.Index())
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on the only request did nothing")
	}
	if msg, ok := cmd().(openRequestMsg); !ok || msg.req.URL != "https://b.example/only" {
		t.Errorf("cmd() = %#v, want openRequestMsg for b.example/only", cmd())
	}
}

func TestBrowserModel_PanesSplitFullWidth40And60(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.http", "GET https://example.com\n")
	for _, w := range []int{60, 80, 120, 200} {
		view := newBrowserModel(dir, newTestHistoryStore(t)).SetSize(w, 20).View()
		lines := strings.Split(view, "\n")
		for i, ln := range lines {
			if lipgloss.Width(ln) != w {
				t.Errorf("w=%d line %d width = %d, want %d", w, i, lipgloss.Width(ln), w)
			}
		}
		top := []rune(ansi.Strip(lines[0]))
		if filesW := w * 40 / 100; len(top) != w || top[filesW-1] != '┓' || top[w-1] != '╮' {
			t.Errorf("w=%d top border = %q, want Files corner at col %d and Requests corner at col %d", w, string(top), filesW, w)
		}
	}
}

func brokenHTTPContent() string {
	content := "### good\nGET https://example.com/good\n"
	for i := 0; i < 5; i++ {
		content += fmt.Sprintf("\n### bad%d\nthis is not a request line number %d with extra words\n", i, i)
	}
	return content
}

func TestBrowserModel_ParseWarningWrapsToAtMostThreeLines(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "broken.http", brokenHTTPContent())
	m := newBrowserModel(dir, newTestHistoryStore(t)).SetSize(80, 24)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	inner := 80 - 80*40/100 - paneStyle.GetHorizontalFrameSize()
	lines := m.parseWarningLines(inner)
	if len(lines) != 3 || !regexp.MustCompile(`^… \+\d+ more$`).MatchString(lines[2]) {
		t.Fatalf("parseWarningLines = %q, want 2 lines + \"… +N more\"", lines)
	}
	for _, ln := range lines {
		if lipgloss.Width(ln) > inner {
			t.Errorf("warning line %q wider than %d", ln, inner)
		}
	}
	view := m.View()
	assertViewFits(t, view, 80, 24)
	if !strings.Contains(ansi.Strip(view), "good") {
		t.Error("the valid request disappeared from the Requests pane")
	}
}

func TestBrowserModel_RecentUsesEntryRows(t *testing.T) {
	store := newTestHistoryStore(t)
	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com/a", StatusCode: 200, Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}
	m := newBrowserModel(t.TempDir(), store).SetSize(120, 20)
	if got := ansi.Strip(m.View()); !strings.Contains(got, "200 GET") {
		t.Errorf("view missing entry row:\n%s", got)
	}
}

func keyRunes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestBrowserModel_FilesRenderOneLineEach(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.http", "b.http", "c.http"} {
		writeTestFile(t, dir, n, "GET https://example.com/\n")
	}
	m := newBrowserModel(dir, newTestHistoryStore(t)).SetSize(120, 20)

	var rows []int
	for i, ln := range strings.Split(m.files.View(), "\n") {
		for _, n := range []string{"a.http", "b.http", "c.http"} {
			if strings.Contains(ln, n) {
				rows = append(rows, i)
			}
		}
	}
	if len(rows) != 3 || rows[1]-rows[0] != 1 || rows[2]-rows[1] != 1 {
		t.Errorf("file rows at lines %v, want three consecutive lines", rows)
	}
}

func TestBrowserModel_FilterNarrowsFiles(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.http", "users.http", "orders.http"} {
		writeTestFile(t, dir, n, "GET https://example.com/\n")
	}
	m := newBrowserModel(dir, newTestHistoryStore(t)).SetSize(120, 20)

	m.files.SetFilterText("users")

	if n := len(m.files.VisibleItems()); n != 1 {
		t.Errorf("VisibleItems = %d, want 1", n)
	}
}

func TestBrowserModel_EnterWhileFilteringAppliesFilterNotOpen(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.http", "GET https://example.com/a\n")
	writeTestFile(t, dir, "users.http", "GET https://example.com/u\n")
	m := newBrowserModel(dir, newTestHistoryStore(t)).SetSize(120, 20)

	m, _ = m.Update(keyRunes("/"))
	m, _ = m.Update(keyRunes("u"))
	if m.files.FilterState() != list.Filtering {
		t.Fatalf("FilterState = %v, want Filtering", m.files.FilterState())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.files.FilterState() != list.FilterApplied {
		t.Errorf("FilterState = %v, want FilterApplied", m.files.FilterState())
	}
	if m.parsedFile != nil || m.focus != paneFiles {
		t.Errorf("Enter opened a file while filtering: parsedFile=%v focus=%v", m.parsedFile, m.focus)
	}
}

func TestRequestItemFilterValueIncludesMethodAndURL(t *testing.T) {
	v := requestItem{req: httpfile.Request{Method: "GET", URL: "https://x/users"}}.FilterValue()
	if !strings.Contains(v, "GET") || !strings.Contains(v, "users") {
		t.Errorf("FilterValue = %q, want method and URL", v)
	}
}

func TestBrowserModel_SelectFileAfterRecentFilterShowsRequests(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.http", "### one\nGET https://a.example/1\n\n### two\nGET https://a.example/2\n")
	store := newTestHistoryStore(t)
	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com/z"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	m := newBrowserModel(dir, store).SetSize(120, 20)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m.requests.SetFilterText("x")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if n := len(m.requests.VisibleItems()); n != 2 {
		t.Errorf("VisibleItems = %d, want 2", n)
	}
}

func lockedSubdirRoot(t *testing.T) string {
	t.Helper()
	skipIfNoPermEnforcement(t)
	root := t.TempDir()
	ok := filepath.Join(root, "ok")
	if err := os.Mkdir(ok, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, ok, "a.http", "GET https://example.com\n")
	locked := filepath.Join(root, "locked")
	if err := os.Mkdir(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	return root
}

func TestDiscoverHTTPFiles_SkipsUnreadableSubdirs(t *testing.T) {
	root := lockedSubdirRoot(t)
	rels, skipped, err := discoverHTTPFiles(root)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if want := []string{filepath.Join("ok", "a.http")}; !reflect.DeepEqual(rels, want) {
		t.Errorf("rels = %v, want %v", rels, want)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
}

func TestDiscoverHTTPFiles_UnreadableOrMissingRootIsError(t *testing.T) {
	if _, _, err := discoverHTTPFiles(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("missing root: expected error")
	}
	skipIfNoPermEnforcement(t)
	root := t.TempDir()
	if err := os.Chmod(root, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	if _, _, err := discoverHTTPFiles(root); err == nil {
		t.Error("unreadable root: expected error")
	}
}

func TestNewBrowserModel_ScanNoticeCountsSkippedDirs(t *testing.T) {
	m := newBrowserModel(lockedSubdirRoot(t), newTestHistoryStore(t))
	if m.scanNotice != "skipped 1 unreadable dirs" {
		t.Errorf("scanNotice = %q", m.scanNotice)
	}
}

func TestBrowserModel_EmptyRootShowsGuidance(t *testing.T) {
	root := t.TempDir()
	m := newBrowserModel(root, newTestHistoryStore(t)).SetSize(600, 20)
	if want := "No .http files under " + root + " — d recent dirs · o open path"; !strings.Contains(ansi.Strip(m.View()), want) {
		t.Errorf("missing guidance %q in:\n%s", want, ansi.Strip(m.View()))
	}
}

func TestBrowserModel_RequestsPaneHintBeforeFileChosen(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "a.http", "GET https://example.com\n")
	m := newBrowserModel(root, newTestHistoryStore(t)).SetSize(120, 20)
	if !strings.Contains(ansi.Strip(m.View()), "Select a file and press enter") {
		t.Errorf("missing requests hint in:\n%s", ansi.Strip(m.View()))
	}
}

func TestBrowserModel_TabStaysOnFilesWhenRequestsEmpty(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "empty.http", "")
	m := newBrowserModel(root, newTestHistoryStore(t)).SetSize(120, 20)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != paneFiles {
		t.Errorf("focus = %v, want paneFiles", m.focus)
	}
}
