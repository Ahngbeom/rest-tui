package tui

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Ahngbeom/rest-tui/internal/dirhistory"
)

func newTestDirHistoryStore(t *testing.T) *dirhistory.Store {
	t.Helper()
	return dirhistory.NewStore(filepath.Join(t.TempDir(), "dirs.json"))
}

func TestDirHistoryModel_RefreshPopulatesListMostRecentFirst(t *testing.T) {
	store := newTestDirHistoryStore(t)
	if _, err := store.Touch("/tmp/a"); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	if _, err := store.Touch("/tmp/b"); err != nil {
		t.Fatalf("Touch: %v", err)
	}

	m := newDirHistoryModel(store).refresh()

	items := m.list.Items()
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	first, ok := items[0].(dirEntryItem)
	if !ok || first.entry.Path != "/tmp/b" {
		t.Errorf("items[0] = %+v, want most-recent (/tmp/b) first", items[0])
	}
}

func TestDirHistoryModel_EnterEmitsSwitchDirMsgWithSelectedPath(t *testing.T) {
	store := newTestDirHistoryStore(t)
	if _, err := store.Touch("/tmp/a"); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	m := newDirHistoryModel(store).refresh()

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a switchDirMsg command")
	}
	msg, ok := cmd().(switchDirMsg)
	if !ok {
		t.Fatalf("expected switchDirMsg, got %T", cmd())
	}
	if msg.path != "/tmp/a" {
		t.Errorf("path = %q, want %q", msg.path, "/tmp/a")
	}
}

func TestDirHistoryModel_BackEmitsBackToBrowserMsg(t *testing.T) {
	store := newTestDirHistoryStore(t)
	m := newDirHistoryModel(store).refresh()

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command emitting backToBrowserMsg")
	}
	if _, ok := cmd().(backToBrowserMsg); !ok {
		t.Fatalf("expected backToBrowserMsg, got %T", cmd())
	}
}

func TestDirHistoryModel_EmptyStoreShowsEmptyList(t *testing.T) {
	store := newTestDirHistoryStore(t)
	m := newDirHistoryModel(store).refresh()

	if len(m.list.Items()) != 0 {
		t.Errorf("expected 0 items for a fresh store, got %d", len(m.list.Items()))
	}
	if m.loadErr != nil {
		t.Errorf("loadErr = %v, want nil", m.loadErr)
	}
}
