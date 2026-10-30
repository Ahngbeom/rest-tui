package tui

import (
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Ahngbeom/rest-tui/internal/httpfile"
)

// awaitMsg waits for every sub-command so no send goroutine writes history into an already-removed t.TempDir().
func awaitMsg[T any](t *testing.T, cmd tea.Cmd) T {
	t.Helper()
	if cmd == nil {
		t.Fatal("awaitMsg: cmd is nil")
	}

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		found bool
		first T
	)
	var run func(c tea.Cmd)
	run = func(c tea.Cmd) {
		defer wg.Done()
		if c == nil {
			return
		}
		msg := c()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, sub := range batch {
				wg.Add(1)
				go run(sub)
			}
			return
		}
		if v, ok := msg.(T); ok {
			mu.Lock()
			if !found {
				found, first = true, v
			}
			mu.Unlock()
		}
	}

	wg.Add(1)
	go run(cmd)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("awaitMsg: commands did not finish within 5s")
	}
	if !found {
		var zero T
		t.Fatalf("awaitMsg: no %T message produced", zero)
	}
	return first
}

func newTestRequestModel(t *testing.T, req httpfile.Request) requestModel {
	t.Helper()
	return newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "").SetSize(120, 35)
}

func skipIfNoPermEnforcement(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file permission bits are not enforced here")
	}
}
