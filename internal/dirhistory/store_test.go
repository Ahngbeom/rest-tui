package dirhistory

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dirs.json")
	return NewStore(path), path
}

func TestStore_TouchAppendsNewPath_MostRecentFirst(t *testing.T) {
	s, _ := newTestStore(t)

	if _, err := s.Touch("/tmp/a"); err != nil {
		t.Fatalf("Touch 1: %v", err)
	}
	if _, err := s.Touch("/tmp/b"); err != nil {
		t.Fatalf("Touch 2: %v", err)
	}

	entries, err := s.List(0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Path != "/tmp/b" {
		t.Errorf("entries[0].Path = %q, want most-recent %q", entries[0].Path, "/tmp/b")
	}
	if entries[1].Path != "/tmp/a" {
		t.Errorf("entries[1].Path = %q, want %q", entries[1].Path, "/tmp/a")
	}
}

func TestStore_TouchDedupesAndMovesToFront(t *testing.T) {
	s, _ := newTestStore(t)

	if _, err := s.Touch("/tmp/a"); err != nil {
		t.Fatalf("Touch a: %v", err)
	}
	if _, err := s.Touch("/tmp/b"); err != nil {
		t.Fatalf("Touch b: %v", err)
	}
	if _, err := s.Touch("/tmp/a"); err != nil {
		t.Fatalf("Touch a again: %v", err)
	}

	entries, err := s.List(0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 unique entries after re-touch, got %d: %+v", len(entries), entries)
	}
	if entries[0].Path != "/tmp/a" {
		t.Errorf("entries[0].Path = %q, want re-touched %q to be most-recent", entries[0].Path, "/tmp/a")
	}
	if entries[1].Path != "/tmp/b" {
		t.Errorf("entries[1].Path = %q, want %q", entries[1].Path, "/tmp/b")
	}
}

func TestStore_TouchUpdatesTimestampOnDedup(t *testing.T) {
	s, _ := newTestStore(t)

	first, err := s.Touch("/tmp/a")
	if err != nil {
		t.Fatalf("Touch 1: %v", err)
	}
	second, err := s.Touch("/tmp/a")
	if err != nil {
		t.Fatalf("Touch 2: %v", err)
	}
	if second.Timestamp.Before(first.Timestamp) {
		t.Errorf("re-touch timestamp %v should not be before original %v", second.Timestamp, first.Timestamp)
	}

	entries, err := s.List(0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if !entries[0].Timestamp.Equal(second.Timestamp) {
		t.Errorf("stored timestamp = %v, want refreshed timestamp %v", entries[0].Timestamp, second.Timestamp)
	}
}

func TestStore_TouchCleansPath(t *testing.T) {
	s, _ := newTestStore(t)

	if _, err := s.Touch("/tmp/x/"); err != nil {
		t.Fatalf("Touch trailing slash: %v", err)
	}
	if _, err := s.Touch("/tmp/x"); err != nil {
		t.Fatalf("Touch clean: %v", err)
	}

	entries, err := s.List(0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected trailing-slash variant to dedup to 1 entry, got %d: %+v", len(entries), entries)
	}
	if entries[0].Path != "/tmp/x" {
		t.Errorf("entries[0].Path = %q, want cleaned %q", entries[0].Path, "/tmp/x")
	}
}

func TestStore_List_RespectsLimit(t *testing.T) {
	s, _ := newTestStore(t)
	for i := 0; i < 5; i++ {
		if _, err := s.Touch(filepath.Join("/tmp", string(rune('a'+i)))); err != nil {
			t.Fatalf("Touch: %v", err)
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

func TestStore_PersistsAcrossInstances(t *testing.T) {
	s, path := newTestStore(t)
	if _, err := s.Touch("/tmp/a"); err != nil {
		t.Fatalf("Touch: %v", err)
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
	path := filepath.Join(t.TempDir(), "dirs.json")
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
	if _, err := s.Touch("/tmp/a"); err != nil {
		t.Fatalf("Touch after recovery: %v", err)
	}
}

func TestStore_WarningIsSafeForConcurrentAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dirs.json")
	if err := os.WriteFile(path, []byte("{ not valid json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s := NewStore(path)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		s.List(0)
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			s.Warning()
		}
	}()
	wg.Wait()

	if s.Warning() == "" {
		t.Error("expected a non-empty recovery warning after concurrent access")
	}
}
