package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ahngbeom/rest-tui/internal/httpfile"
)

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "history.json")
	return NewStore(path), path
}

func TestStore_AppendAndList_MostRecentFirst(t *testing.T) {
	s, _ := newTestStore(t)

	first, err := s.Append(Entry{Method: "GET", URL: "https://example.com/a", StatusCode: 200})
	if err != nil {
		t.Fatalf("Append 1: %v", err)
	}
	second, err := s.Append(Entry{Method: "POST", URL: "https://example.com/b", StatusCode: 201})
	if err != nil {
		t.Fatalf("Append 2: %v", err)
	}
	if first.ID == "" || second.ID == "" {
		t.Fatalf("expected non-empty IDs, got %q and %q", first.ID, second.ID)
	}
	if first.ID == second.ID {
		t.Fatalf("expected distinct IDs, both were %q", first.ID)
	}

	entries, err := s.List(0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].ID != second.ID {
		t.Errorf("entries[0].ID = %q, want most-recent %q", entries[0].ID, second.ID)
	}
	if entries[1].ID != first.ID {
		t.Errorf("entries[1].ID = %q, want %q", entries[1].ID, first.ID)
	}
}

func TestStore_List_RespectsLimit(t *testing.T) {
	s, _ := newTestStore(t)
	for i := 0; i < 5; i++ {
		if _, err := s.Append(Entry{Method: "GET", URL: "https://example.com"}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	entries, err := s.List(2)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestStore_Get_ReturnsByID(t *testing.T) {
	s, _ := newTestStore(t)
	stored, err := s.Append(Entry{Method: "GET", URL: "https://example.com/a"})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	got, err := s.Get(stored.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.URL != "https://example.com/a" {
		t.Errorf("URL = %q", got.URL)
	}
}

func TestStore_Get_UnknownIDIsError(t *testing.T) {
	s, _ := newTestStore(t)

	_, err := s.Get("does-not-exist")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestStore_PersistsAcrossInstances(t *testing.T) {
	s, path := newTestStore(t)
	if _, err := s.Append(Entry{Method: "GET", URL: "https://example.com/a"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	reopened := NewStore(path)
	entries, err := reopened.List(0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after reopen, got %d", len(entries))
	}
}

func TestStore_CorruptedFileRecoversAndBacksUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	if err := os.WriteFile(path, []byte("{ not valid json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := NewStore(path)
	entries, err := s.List(0)
	if err != nil {
		t.Fatalf("List should recover from corruption without error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after recovery, got %d", len(entries))
	}
	if s.Warning() == "" {
		t.Error("expected a non-empty recovery warning")
	}

	matches, _ := filepath.Glob(path + ".corrupted-*")
	if len(matches) != 1 {
		t.Errorf("expected 1 backup file matching %q, got %v", path+".corrupted-*", matches)
	}

	// Store should still be usable after recovery.
	if _, err := s.Append(Entry{Method: "GET", URL: "https://example.com"}); err != nil {
		t.Fatalf("Append after recovery: %v", err)
	}
}

func TestStore_RequestAndResponseFieldsRoundTrip(t *testing.T) {
	s, _ := newTestStore(t)
	entry := Entry{
		Method:          "POST",
		URL:             "https://example.com/users",
		RequestHeaders:  []httpfile.Header{{Name: "Content-Type", Value: "application/json"}},
		RequestBody:     `{"name":"Bob"}`,
		StatusCode:      201,
		ResponseHeaders: []httpfile.Header{{Name: "X-Id", Value: "1"}},
		ResponseBody:    `{"id":1}`,
		Duration:        150 * time.Millisecond,
	}

	stored, err := s.Append(entry)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	got, err := s.Get(stored.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.RequestBody != entry.RequestBody || got.ResponseBody != entry.ResponseBody {
		t.Errorf("got = %+v", got)
	}
	if got.Duration != entry.Duration {
		t.Errorf("Duration = %v, want %v", got.Duration, entry.Duration)
	}
	if len(got.RequestHeaders) != 1 || got.RequestHeaders[0].Name != "Content-Type" {
		t.Errorf("RequestHeaders = %v", got.RequestHeaders)
	}
}
