package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Ahngbeom/rest-tui/internal/history"
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
