package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bahn/rest-tui/internal/history"
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
