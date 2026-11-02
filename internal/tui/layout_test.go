package tui

import (
	"fmt"
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

var layoutSizes = []struct{ w, h int }{{60, 20}, {80, 24}, {120, 35}, {200, 50}}

type layoutScenario struct {
	name  string
	build func(t *testing.T, w, h int) App
}

var layoutScenarios = []layoutScenario{
	{"request-intellij-warnings", func(t *testing.T, w, h int) App {
		root := t.TempDir()
		a := newLayoutApp(t, root, newTestHistoryStore(t), w, h)
		req := httpfile.Request{Method: "POST", URL: "{{host}}/very/long/path/segment/that/wraps", Body: "< ./missing-body-file.json",
			Unsupported: []string{"pre-request script", "response handler script", "response handler script: ./check.js", "output redirect: ./out.json"}}
		a = applyMsg(a, openRequestMsg{filePath: filepath.Join(root, "a.http"), file: &httpfile.File{}, req: req})
		return pressKeys(a, "enter")
	}},
	{"browser-empty", func(t *testing.T, w, h int) App {
		return newLayoutApp(t, t.TempDir(), newTestHistoryStore(t), w, h)
	}},
	{"browser-long-root", func(t *testing.T, w, h int) App {
		root := filepath.Join(t.TempDir(), strings.Repeat("long-directory-name-", 8))
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		return newLayoutApp(t, root, newTestHistoryStore(t), w, h)
	}},
	{"store-warning", func(t *testing.T, w, h int) App {
		histPath := filepath.Join(t.TempDir(), "history.json")
		if err := os.WriteFile(histPath, []byte("{ not valid json"), 0o644); err != nil {
			t.Fatal(err)
		}
		a := pressKeys(newLayoutApp(t, t.TempDir(), history.NewStore(histPath), w, h), "h")
		if a.statusMsg == "" {
			t.Fatal("precondition: expected a store warning")
		}
		return a
	}},
	{"request-long-response", func(t *testing.T, w, h int) App {
		a := newLayoutRequestApp(t, w, h)
		return applyMsg(a, execResultMsg{modelID: a.request.id, entry: history.Entry{StatusCode: 200, ResponseBody: `{"path":"` + strings.Repeat("segment/", 60) + `","text":"` + strings.Repeat("lorem ipsum ", 40) + `"}`}})
	}},
	{"request-summary-redirect", func(t *testing.T, w, h int) App {
		a := newLayoutRequestApp(t, w, h)
		long := "http://x/" + strings.Repeat("r", 191)
		return applyMsg(a, execResultMsg{modelID: a.request.id, entry: history.Entry{StatusCode: 200, Status: "200 OK",
			ResponseBody: strings.Repeat("line\n", 200), Redirects: []string{"301 → GET " + long, "302 → GET " + long}}})
	}},
	{"request-cjk-response", func(t *testing.T, w, h int) App {
		a := newLayoutRequestApp(t, w, h)
		return applyMsg(a, execResultMsg{modelID: a.request.id, entry: history.Entry{StatusCode: 200, ResponseBody: strings.Repeat("한글 응답 본문 ", 80)}})
	}},
	{"request-sending-quit-confirm", func(t *testing.T, w, h int) App {
		a := newLayoutRequestApp(t, w, h)
		a.request.executing = true
		return pressKeys(a, "q")
	}},
	{"request-copy-status", func(t *testing.T, w, h int) App {
		a := newLayoutRequestApp(t, w, h)
		return applyMsg(a, clipboardCopyMsg{token: a.request.copyToken})
	}},
	{"request-edit-error", func(t *testing.T, w, h int) App {
		a := pressKeys(newLayoutRequestApp(t, w, h), "i")
		a.request.editor.SetValue("GET https://example.com\n" + strings.Repeat("bad ", 60) + "\n")
		return pressKeys(a, "ctrl+s")
	}},
	{"browser-parse-warning", func(t *testing.T, w, h int) App {
		root := t.TempDir()
		writeTestFile(t, root, "broken.http", brokenHTTPContent())
		return pressKeys(newLayoutApp(t, root, newTestHistoryStore(t), w, h), "enter")
	}},
	{"browser-file-parse-error", func(t *testing.T, w, h int) App {
		root := t.TempDir()
		writeTestFile(t, root, "bad.http", "this is not a request "+strings.Repeat("word ", 40)+"\n")
		return pressKeys(newLayoutApp(t, root, newTestHistoryStore(t), w, h), "enter")
	}},
	{"browser-scan-error", func(t *testing.T, w, h int) App {
		root := filepath.Join(t.TempDir(), strings.Repeat("missing-directory-", 10))
		return newLayoutApp(t, root, newTestHistoryStore(t), w, h)
	}},
	{"browser-skipped-dirs", func(t *testing.T, w, h int) App {
		return newLayoutApp(t, lockedSubdirRoot(t), newTestHistoryStore(t), w, h)
	}},
	{"history-list", func(t *testing.T, w, h int) App {
		return pressKeys(newLayoutHistoryApp(t, w, h), "h")
	}},
	{"history-detail-copy", func(t *testing.T, w, h int) App {
		a := pressKeys(newLayoutHistoryApp(t, w, h), "h", "enter")
		return applyMsg(a, clipboardCopyMsg{token: a.history.copyToken})
	}},
	{"history-rerun-confirm", func(t *testing.T, w, h int) App {
		store := newTestHistoryStore(t)
		if _, err := store.Append(history.Entry{Method: "POST", URL: "https://example.com/" + strings.Repeat("p", 292), StatusCode: 200}); err != nil {
			t.Fatal(err)
		}
		a := newLayoutApp(t, t.TempDir(), store, w, h)
		a = pressKeys(a, "h", "r")
		if a.history.pendingRerun == nil {
			t.Fatal("precondition: expected a pending rerun confirmation")
		}
		return a
	}},
	{"history-rerun-confirm-newline-url", func(t *testing.T, w, h int) App {
		store := newTestHistoryStore(t)
		if _, err := store.Append(history.Entry{Method: "POST", URL: "https://example.com/a\nb\nc\td", StatusCode: 200}); err != nil {
			t.Fatal(err)
		}
		a := pressKeys(newLayoutApp(t, t.TempDir(), store, w, h), "h", "r")
		if a.history.pendingRerun == nil {
			t.Fatal("precondition: expected a pending rerun confirmation")
		}
		return a
	}},
	{"history-rows-long-url", func(t *testing.T, w, h int) App {
		store := newTestHistoryStore(t)
		long := "https://example.com/" + strings.Repeat("u", 280)
		for _, e := range []history.Entry{{Method: "GET", URL: long, StatusCode: 200}, {Method: "POST", URL: long, StatusCode: 500}, {Method: "GET", URL: long, Error: "canceled"}} {
			if _, err := store.Append(e); err != nil {
				t.Fatal(err)
			}
		}
		return pressKeys(newLayoutApp(t, t.TempDir(), store, w, h), "h")
	}},
	{"dir-history", func(t *testing.T, w, h int) App {
		return pressKeys(newLayoutHistoryApp(t, w, h), "d")
	}},
	{"dirs-open-path-error", func(t *testing.T, w, h int) App {
		a := newLayoutHistoryApp(t, w, h)
		return pressKeys(a, "o", "/no/such/"+strings.Repeat("d", 200), "enter")
	}},
	{"help", func(t *testing.T, w, h int) App {
		return pressKeys(newLayoutApp(t, t.TempDir(), newTestHistoryStore(t), w, h), "?")
	}},
}

func newLayoutHistoryApp(t *testing.T, w, h int) App {
	t.Helper()
	store := newTestHistoryStore(t)
	for _, u := range []string{"https://example.com/a", "https://example.com/b"} {
		if _, err := store.Append(history.Entry{Method: "GET", URL: u, StatusCode: 200, ResponseBody: strings.Repeat("line\n", 40)}); err != nil {
			t.Fatal(err)
		}
	}
	return newLayoutApp(t, t.TempDir(), store, w, h)
}

func newLayoutRequestApp(t *testing.T, w, h int) App {
	t.Helper()
	a := newLayoutApp(t, t.TempDir(), newTestHistoryStore(t), w, h)
	return applyMsg(a, openRequestMsg{file: &httpfile.File{}, req: httpfile.Request{Method: "GET", URL: "https://example.com/users"}})
}

func newLayoutApp(t *testing.T, root string, store *history.Store, width, height int) App {
	t.Helper()
	dirStore := dirhistory.NewStore(filepath.Join(t.TempDir(), "dirs.json"))
	return applyMsg(NewApp(root, store, dirStore, Options{}), tea.WindowSizeMsg{Width: width, Height: height})
}

func pressKeys(a App, ks ...string) App {
	named := map[string]tea.KeyType{
		"enter":  tea.KeyEnter,
		"esc":    tea.KeyEsc,
		"tab":    tea.KeyTab,
		"up":     tea.KeyUp,
		"down":   tea.KeyDown,
		"ctrl+s": tea.KeyCtrlS,
	}
	for _, k := range ks {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		if kt, ok := named[k]; ok {
			msg = tea.KeyMsg{Type: kt}
		}
		a = applyMsg(a, msg)
	}
	return a
}

func assertViewFits(t *testing.T, view string, width, height int) {
	t.Helper()
	if got := lipgloss.Height(view); got > height {
		t.Errorf("view is %d lines tall, terminal is %d", got, height)
	}
	for i, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > width {
			t.Errorf("line %d is %d cells wide, terminal is %d: %q", i+1, w, width, ansi.Strip(line))
		}
	}
}

func TestApp_ViewFitsTerminal(t *testing.T) {
	for _, sc := range layoutScenarios {
		for _, sz := range layoutSizes {
			t.Run(fmt.Sprintf("%s/%dx%d", sc.name, sz.w, sz.h), func(t *testing.T) {
				view := sc.build(t, sz.w, sz.h).View()
				assertViewFits(t, view, sz.w, sz.h)
				if first := ansi.Strip(strings.SplitN(view, "\n", 2)[0]); !strings.Contains(first, "rest-tui") {
					t.Errorf("header pushed off screen; first line = %q", first)
				}
			})
		}
	}
}

func TestApp_ViewDoesNotPanicOnTinyTerminal(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{0, 0}, {20, 5}} {
		for _, keys := range [][]string{nil, {"h"}, {"d"}, {"?"}} {
			a := pressKeys(newLayoutApp(t, t.TempDir(), newTestHistoryStore(t), sz.w, sz.h), keys...)
			_ = a.View()
			a = applyMsg(a, openRequestMsg{file: &httpfile.File{}, req: httpfile.Request{Method: "GET", URL: "https://example.com"}})
			_ = a.View()
		}
	}
}

func TestFitLines_WrapsAndKeepsLeadingLines(t *testing.T) {
	got := fitLines("abcdefgh\nij\nkl", 4, 3)
	if want := "abcd\nefgh\nij"; got != want {
		t.Errorf("fitLines = %q, want %q", got, want)
	}
}

func TestFitLines_UnsizedReturnsInput(t *testing.T) {
	s := "some error text"
	for _, sz := range []struct{ w, h int }{{0, 5}, {5, 0}, {-1, -1}} {
		if got := fitLines(s, sz.w, sz.h); got != s {
			t.Errorf("fitLines(%d, %d) = %q, want input unchanged", sz.w, sz.h, got)
		}
	}
}

func TestApp_HeaderShownBeforeFirstWindowSize(t *testing.T) {
	a := NewApp(t.TempDir(), newTestHistoryStore(t), dirhistory.NewStore(filepath.Join(t.TempDir(), "dirs.json")), Options{})
	if first := ansi.Strip(strings.SplitN(a.View(), "\n", 2)[0]); !strings.Contains(first, "rest-tui") {
		t.Errorf("header blank before first WindowSizeMsg; first line = %q", first)
	}
}
