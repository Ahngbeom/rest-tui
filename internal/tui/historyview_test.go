package tui

import (
	"bytes"
	"errors"
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

func TestHistoryModel_RefreshPopulatesListMostRecentFirst(t *testing.T) {
	store := newTestHistoryStore(t)
	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com/a"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com/b"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	m := newHistoryModel(store).refresh()

	items := m.list.Items()
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	first, ok := items[0].(entryItem)
	if !ok || first.entry.URL != "https://example.com/b" {
		t.Errorf("items[0] = %+v, want most-recent (b) first", items[0])
	}
}

func TestHistoryModel_EnterShowsDetail(t *testing.T) {
	store := newTestHistoryStore(t)
	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com/a", StatusCode: 200}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	m := newHistoryModel(store).refresh()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.mode != historyModeDetail {
		t.Fatalf("mode = %v, want historyModeDetail", m.mode)
	}
	if m.selected == nil || m.selected.URL != "https://example.com/a" {
		t.Errorf("selected = %+v", m.selected)
	}
}

func longURLResponseEntry(longURL string) history.Entry {
	return history.Entry{
		Method: "GET", URL: "https://example.com/a", StatusCode: 200,
		ResponseBody: `{"url": "` + longURL + `"}`,
	}
}

func detailViewJoined(m historyModel) string {
	view := ansiEscapePattern.ReplaceAllString(m.detail.View(), "")
	var joined strings.Builder
	for _, line := range strings.Split(view, "\n") {
		joined.WriteString(strings.TrimRight(line, " "))
	}
	return joined.String()
}

func TestHistoryModel_LongResponseLineWrapsInsteadOfTruncating(t *testing.T) {
	longURL := "https://example.com/very/long/path/that/goes/on/and/on/and/should/be/wider/than/the/viewport"
	store := newTestHistoryStore(t)
	if _, err := store.Append(longURLResponseEntry(longURL)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	m := newHistoryModel(store).refresh().SetSize(60, 20) // innerWidth = 56

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !strings.Contains(detailViewJoined(m), longURL) {
		t.Errorf("detail.View() does not contain the full URL unbroken; got:\n%s", m.detail.View())
	}
}

func TestHistoryModel_ShrinkingWhileInDetailModeRewraps(t *testing.T) {
	longURL := "https://example.com/very/long/path/that/goes/on/and/on/and/should/be/wider/than/the/viewport"
	store := newTestHistoryStore(t)
	if _, err := store.Append(longURLResponseEntry(longURL)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	m := newHistoryModel(store).refresh().SetSize(200, 20) // wide enough that the URL fits on one line

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m.SetSize(60, 20) // shrink while a detail is showing

	if !strings.Contains(detailViewJoined(m), longURL) {
		t.Errorf("detail.View() does not contain the full URL unbroken after shrinking; got:\n%s", m.detail.View())
	}
}

func TestHistoryModel_LongResponseLineIsOneClickableHyperlink(t *testing.T) {
	longURL := "https://example.com/very/long/path/that/goes/on/and/on/and/should/be/wider/than/the/viewport"
	store := newTestHistoryStore(t)
	if _, err := store.Append(longURLResponseEntry(longURL)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	m := newHistoryModel(store).refresh().SetSize(60, 20) // innerWidth = 56

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	view := m.detail.View()
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

func TestHistoryModel_RerunFromListEmitsRerunMsg(t *testing.T) {
	store := newTestHistoryStore(t)
	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com/a"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	m := newHistoryModel(store).refresh()

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Fatal("expected a rerun command")
	}
	msg, ok := cmd().(rerunMsg)
	if !ok {
		t.Fatalf("expected rerunMsg, got %T", cmd())
	}
	if msg.entry.URL != "https://example.com/a" {
		t.Errorf("entry.URL = %q", msg.entry.URL)
	}
}

func TestHistoryModel_RerunFromDetailUsesSelectedEntry(t *testing.T) {
	store := newTestHistoryStore(t)
	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com/a"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	m := newHistoryModel(store).refresh()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // -> detail mode

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Fatal("expected a rerun command")
	}
	msg, ok := cmd().(rerunMsg)
	if !ok || msg.entry.URL != "https://example.com/a" {
		t.Fatalf("cmd() = %+v", msg)
	}
}

func TestHistoryModel_BackFromDetailReturnsToList(t *testing.T) {
	store := newTestHistoryStore(t)
	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com/a"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	m := newHistoryModel(store).refresh()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Error("expected no command when backing out of detail (local transition)")
	}
	if m.mode != historyModeList {
		t.Errorf("mode = %v, want historyModeList", m.mode)
	}
}

func TestHistoryModel_BackFromListEmitsBackToBrowserMsg(t *testing.T) {
	store := newTestHistoryStore(t)
	m := newHistoryModel(store).refresh()

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command emitting backToBrowserMsg")
	}
	if _, ok := cmd().(backToBrowserMsg); !ok {
		t.Fatalf("expected backToBrowserMsg, got %T", cmd())
	}
}

func TestCopyEntryPlainText(t *testing.T) {
	e := history.Entry{
		Method: "GET", URL: "https://example.com/a",
		StatusCode: 200, ResponseBody: `{"ok":true}`,
	}

	got := copyEntryPlainText(e)
	if !strings.Contains(got, "GET https://example.com/a") {
		t.Errorf("copyEntryPlainText() = %q, want it to contain the request line", got)
	}
	if !strings.Contains(got, `"ok": true`) {
		t.Errorf("copyEntryPlainText() = %q, want it to contain the pretty response body", got)
	}
	if strings.Contains(got, "\x1b[") {
		t.Errorf("copyEntryPlainText() = %q, should not contain ANSI escapes", got)
	}
}

func TestCopyEntryPlainText_Error(t *testing.T) {
	e := history.Entry{Method: "GET", URL: "https://example.com/a", Error: "timeout"}

	got := copyEntryPlainText(e)
	if !strings.Contains(got, "Error: timeout") {
		t.Errorf("copyEntryPlainText() = %q, want it to contain the error", got)
	}
}

func TestHistoryModel_CopyKeyGatedByDetailMode(t *testing.T) {
	store := newTestHistoryStore(t)
	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com/a"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	m := newHistoryModel(store).refresh()

	// In list mode, Copy should be a no-op.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if cmd != nil {
		t.Error("expected no copy command in list mode")
	}

	// In detail mode, Copy should return a command (not invoked here — it
	// would write to the real OS clipboard).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if cmd == nil {
		t.Fatal("expected a copy command in detail mode")
	}
	if m.copyToken != 1 {
		t.Errorf("copyToken = %d, want 1", m.copyToken)
	}
}

func TestHistoryModel_ClipboardCopyMsgSetsStatus(t *testing.T) {
	store := newTestHistoryStore(t)
	m := newHistoryModel(store).refresh()
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

func TestHistoryModel_ClipboardCopyExpiredMsgClearsStatus(t *testing.T) {
	store := newTestHistoryStore(t)
	m := newHistoryModel(store).refresh()
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

func TestHistoryModel_ListUsesFullWidth(t *testing.T) {
	store := newTestHistoryStore(t)
	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com", StatusCode: 200}); err != nil {
		t.Fatal(err)
	}
	view := newHistoryModel(store).refresh().SetSize(120, 20).View()
	for i, ln := range strings.Split(view, "\n") {
		if lipgloss.Width(ln) != 120 {
			t.Errorf("line %d width = %d, want 120", i, lipgloss.Width(ln))
		}
	}
}

func TestHistoryModel_DetailCopyStatusKeepsHeight(t *testing.T) {
	store := newTestHistoryStore(t)
	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com", StatusCode: 200, ResponseBody: strings.Repeat("line\n", 40)}); err != nil {
		t.Fatal(err)
	}
	m := newHistoryModel(store).refresh().SetSize(80, 20)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if h := lipgloss.Height(m.View()); h != 20 {
		t.Fatalf("detail height = %d, want 20", h)
	}
	m, _ = m.Update(clipboardCopyMsg{token: m.copyToken})
	if v := m.View(); lipgloss.Height(v) != 20 || !strings.Contains(v, "copied to clipboard") {
		t.Errorf("after copy: height = %d", lipgloss.Height(v))
	}
}

func TestHistoryView_ShowsEnvInListAndDetail(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	e := history.Entry{Method: "GET", URL: "https://example.com", StatusCode: 200, Env: "dev", Timestamp: now}
	if got := ansi.Strip(formatEntryRow(e, now, 80)); !strings.Contains(got, "GET     dev https://example.com") {
		t.Errorf("row = %q, want env between method and URL", got)
	}
	if got := plainText(renderEntryDetail(e)); !strings.Contains(got, "env: dev") {
		t.Errorf("detail = %q, want env: dev", got)
	}
	e.Env = ""
	if got := ansi.Strip(formatEntryRow(e, now, 80)); !strings.Contains(got, "GET     https://example.com") || strings.Contains(got, "dev") {
		t.Errorf("row = %q, want no env", got)
	}
}

var rowNow = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

func TestRelativeTime(t *testing.T) {
	for _, c := range []struct {
		d    time.Duration
		want string
	}{{30 * time.Second, "just now"}, {3 * time.Minute, "3m ago"}, {5 * time.Hour, "5h ago"}, {49 * time.Hour, "2d ago"}} {
		if got := relativeTime(rowNow.Add(-c.d), rowNow); got != c.want {
			t.Errorf("relativeTime(-%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestFormatEntryRow_Layout(t *testing.T) {
	e := history.Entry{StatusCode: 201, Method: "POST", Env: "dev", Name: "Create user", Timestamp: rowNow.Add(-3 * time.Minute)}
	if got, want := ansi.Strip(formatEntryRow(e, rowNow, 80)), "201 POST    dev Create user  3m ago"; got != want {
		t.Errorf("row = %q, want %q", got, want)
	}
}

func TestFormatEntryRow_ErrorFallsBackToURL(t *testing.T) {
	e := history.Entry{Error: "canceled", Method: "GET", URL: "https://example.com/x", Timestamp: rowNow}
	if got, want := ansi.Strip(formatEntryRow(e, rowNow, 80)), "ERR GET     https://example.com/x  just now"; got != want {
		t.Errorf("row = %q, want %q", got, want)
	}
}

func TestFormatEntryRow_TruncatesNameNotStatus(t *testing.T) {
	e := history.Entry{StatusCode: 200, Method: "GET", URL: "https://example.com/" + strings.Repeat("a", 100), Timestamp: rowNow}
	row := ansi.Strip(formatEntryRow(e, rowNow, 30))
	if lipgloss.Width(row) > 30 || !strings.HasPrefix(row, "200 GET     ") || !strings.HasSuffix(row, "just now") || !strings.Contains(row, "…") {
		t.Errorf("row = %q", row)
	}
}

func TestFormatEntryRow_StaysOneLine(t *testing.T) {
	e := history.Entry{StatusCode: 200, Method: "GET", Name: "a\nb\tc\x1b[31m", Env: "d\ne", Timestamp: rowNow}
	row := formatEntryRow(e, rowNow, 80)
	if strings.ContainsAny(row, "\n\t") || strings.Contains(ansi.Strip(row), "\x1b") {
		t.Errorf("row = %q, want a single clean line", row)
	}
	if got := ansi.Strip(row); !strings.Contains(got, "d e a b c") {
		t.Errorf("row = %q, want collapsed spaces", got)
	}
}

func TestEntryRowDelegate_RendersOneLineWithCursor(t *testing.T) {
	d := entryRowDelegate{}
	if d.Height() != 1 || d.Spacing() != 0 {
		t.Fatalf("Height/Spacing = %d/%d", d.Height(), d.Spacing())
	}
	items := []list.Item{
		entryItem{entry: history.Entry{StatusCode: 200, Method: "GET", URL: "https://example.com/a", Timestamp: time.Now()}},
		entryItem{entry: history.Entry{StatusCode: 404, Method: "GET", URL: "https://example.com/b", Timestamp: time.Now()}},
	}
	l := list.New(items, d, 60, 10)
	var b0, b1 bytes.Buffer
	d.Render(&b0, l, 0, items[0])
	d.Render(&b1, l, 1, items[1])
	if got := ansi.Strip(b0.String()); !strings.HasPrefix(got, "> 200") || strings.Contains(got, "\n") {
		t.Errorf("selected = %q", got)
	}
	if got := ansi.Strip(b1.String()); !strings.HasPrefix(got, "  404") {
		t.Errorf("other = %q", got)
	}
}

func TestHistoryView_DetailSanitizesRequestHead(t *testing.T) {
	const osc = "\x1b]0;X\x07"
	e := history.Entry{
		Method:         "POST",
		URL:            "https://example.com/" + osc,
		RequestHeaders: []httpfile.Header{{Name: "X-A", Value: "v" + osc}},
		RequestBody:    "body" + osc,
		StatusCode:     200,
	}
	if got := renderEntryDetail(e); strings.Contains(got, "\x1b]") {
		t.Errorf("detail contains OSC sequence: %q", got)
	}
}

func TestIsSafeMethod(t *testing.T) {
	for _, m := range []string{"GET", "head", "OPTIONS"} {
		if !isSafeMethod(m) {
			t.Errorf("isSafeMethod(%q) = false, want true", m)
		}
	}
	for _, m := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if isSafeMethod(m) {
			t.Errorf("isSafeMethod(%q) = true, want false", m)
		}
	}
}

func newPostHistoryModel(t *testing.T) historyModel {
	t.Helper()
	store := newTestHistoryStore(t)
	if _, err := store.Append(history.Entry{Method: "POST", URL: "https://example.com/users"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	return newHistoryModel(store).refresh().SetSize(120, 20)
}

func TestHistoryModel_RerunUnsafeMethodAsksConfirmation(t *testing.T) {
	m := newPostHistoryModel(t)

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd != nil {
		t.Fatal("expected no command before confirmation")
	}
	if !strings.Contains(ansi.Strip(m.View()), "rerun POST https://example.com/users? (y/n)") {
		t.Errorf("view lacks confirmation prompt:\n%s", ansi.Strip(m.View()))
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected rerunMsg after y")
	}
	msg, ok := cmd().(rerunMsg)
	if !ok || msg.entry.URL != "https://example.com/users" {
		t.Errorf("cmd() = %#v, want rerunMsg for the POST entry", cmd())
	}
}

func TestHistoryModel_RerunConfirmationOtherKeyCancels(t *testing.T) {
	m := newPostHistoryModel(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})

	if cmd != nil || m.pendingRerun != nil {
		t.Errorf("cmd = %v, pendingRerun = %v, want both nil", cmd, m.pendingRerun)
	}
	if strings.Contains(ansi.Strip(m.View()), "(y/n)") {
		t.Error("view still shows the confirmation prompt")
	}
}

func TestHistoryModel_RerunSafeMethodSkipsConfirmation(t *testing.T) {
	store := newTestHistoryStore(t)
	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com/a"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	m := newHistoryModel(store).refresh().SetSize(120, 20)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if m.pendingRerun != nil || cmd == nil {
		t.Fatalf("pendingRerun = %v, cmd nil = %v, want immediate rerun", m.pendingRerun, cmd == nil)
	}
	if _, ok := cmd().(rerunMsg); !ok {
		t.Errorf("cmd() = %T, want rerunMsg", cmd())
	}
}

func TestHistoryModel_RefreshKeepsCursorOnSameEntry(t *testing.T) {
	store := newTestHistoryStore(t)
	a, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com/a"})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com/b"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	m := newHistoryModel(store).refresh().SetSize(120, 20)
	m.list.Select(1)
	if got := m.list.SelectedItem().(entryItem).entry.ID; got != a.ID {
		t.Fatalf("precondition: selected %q, want %q", got, a.ID)
	}

	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com/c"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	m = m.refresh()

	if got := m.list.SelectedItem().(entryItem).entry.ID; got != a.ID {
		t.Errorf("selected ID = %q, want %q", got, a.ID)
	}
}

func newFilterHistoryModel(t *testing.T) (historyModel, *history.Store) {
	t.Helper()
	store := newTestHistoryStore(t)
	for _, u := range []string{"https://example.com/users", "https://example.com/orders"} {
		if _, err := store.Append(history.Entry{Method: "GET", URL: u}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	return newHistoryModel(store).refresh().SetSize(120, 20), store
}

func TestHistoryModel_FilterThenEnterOpensFilteredEntry(t *testing.T) {
	m, _ := newFilterHistoryModel(t)
	m.list.SetFilterText("orders")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.selected == nil || !strings.HasSuffix(m.selected.URL, "/orders") {
		t.Errorf("selected = %+v, want the /orders entry", m.selected)
	}
}

func TestHistoryModel_EscWithFilterAppliedClearsFilterFirst(t *testing.T) {
	m, _ := newFilterHistoryModel(t)
	m.list.SetFilterText("orders")

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.list.FilterState() != list.Unfiltered {
		t.Errorf("FilterState = %v, want Unfiltered", m.list.FilterState())
	}
	if cmd != nil {
		if _, ok := cmd().(backToBrowserMsg); ok {
			t.Error("esc went back to the browser instead of clearing the filter")
		}
	}
}

func TestHistoryModel_RefreshWithFilterAppliedShowsAllAndKeepsCursor(t *testing.T) {
	m, store := newFilterHistoryModel(t)
	m.list.SetFilterText("orders")
	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com/new"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	m = m.refresh()

	if m.list.FilterState() != list.Unfiltered || len(m.list.VisibleItems()) != 3 {
		t.Errorf("FilterState = %v, visible = %d, want Unfiltered and 3", m.list.FilterState(), len(m.list.VisibleItems()))
	}
	if it, ok := m.list.SelectedItem().(entryItem); !ok || !strings.HasSuffix(it.entry.URL, "/orders") {
		t.Errorf("SelectedItem = %+v, want the /orders entry", m.list.SelectedItem())
	}
}
