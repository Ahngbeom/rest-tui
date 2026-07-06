package tui

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Ahngbeom/rest-tui/internal/history"
	"github.com/Ahngbeom/rest-tui/internal/httpfile"
)

func newTestApp(t *testing.T) App {
	t.Helper()
	dir := t.TempDir()
	store := history.NewStore(filepath.Join(t.TempDir(), "history.json"))
	a := NewApp(dir, store)
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
