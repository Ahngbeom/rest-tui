package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

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

func TestDirHistoryModel_FilterNarrowsDirs(t *testing.T) {
	store := newTestDirHistoryStore(t)
	for _, p := range []string{"/tmp/alpha", "/tmp/beta"} {
		if _, err := store.Touch(p); err != nil {
			t.Fatalf("Touch: %v", err)
		}
	}
	m := newDirHistoryModel(store).refresh().SetSize(120, 20)

	m.list.SetFilterText("alpha")

	if n := len(m.list.VisibleItems()); n != 1 {
		t.Errorf("VisibleItems = %d, want 1", n)
	}
}

func openPromptModel(t *testing.T) dirHistoryModel {
	t.Helper()
	m, _ := newDirHistoryModel(newTestDirHistoryStore(t)).refresh().SetSize(240, 20).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	if !m.isPrompting() {
		t.Fatal("o should open the prompt")
	}
	return m
}

func typeInto(m dirHistoryModel, s string) dirHistoryModel {
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return m
}

func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for in, want := range map[string]string{
		"~":      home,
		"~/api":  filepath.Join(home, "api"),
		" /abs ": "/abs",
		"~bob/x": "~bob/x",
	} {
		got, err := expandHome(in)
		if err != nil || got != want {
			t.Errorf("expandHome(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestDirHistoryModel_OpenPathSwitchesToDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.Mkdir(filepath.Join(home, "proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := typeInto(openPromptModel(t), "~/proj")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	msg, ok := cmd().(switchDirMsg)
	if !ok || msg.path != filepath.Join(home, "proj") {
		t.Errorf("got %#v", cmd())
	}
}

func TestDirHistoryModel_OpenPathRejectsMissingAndFile(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope")
	m := typeInto(openPromptModel(t), missing)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || !m.isPrompting() || !strings.Contains(ansi.Strip(m.View()), "no such directory: "+missing) {
		t.Errorf("missing: cmd=%v prompting=%v", cmd != nil, m.isPrompting())
	}

	file := writeTestFile(t, dir, "f.txt", "x")
	m = openPromptModel(t)
	m = typeInto(m, file)
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || !strings.Contains(ansi.Strip(m.View()), "not a directory: "+file) {
		t.Errorf("file: cmd=%v", cmd != nil)
	}
}

func TestDirHistoryModel_EscClosesPromptOnly(t *testing.T) {
	m, cmd := openPromptModel(t).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.isPrompting() || cmd != nil {
		t.Errorf("prompting=%v cmd=%v", m.isPrompting(), cmd != nil)
	}
}

func TestDirHistoryModel_BackspaceInPromptDeletesChar(t *testing.T) {
	m := typeInto(openPromptModel(t), "ab")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if !m.isPrompting() || m.prompt.Value() != "a" {
		t.Errorf("prompting=%v value=%q", m.isPrompting(), m.prompt.Value())
	}
}
