// Package dirhistory persists a local record of directories passed via -dir,
// so past directories can be browsed and switched to from the TUI.
package dirhistory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is one directory that was loaded via -dir.
type Entry struct {
	Path      string
	Timestamp time.Time
}

// Store reads and appends Entry records to a JSON file on disk.
type Store struct {
	path string

	mu      sync.Mutex
	warning string
}

// NewStore returns a Store backed by the file at path. The file need not
// exist yet; it is created on first Touch.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Warning returns a human-readable message describing the most recent
// recovery action (e.g. a corrupted directory history file being backed up
// and replaced), or "" if nothing noteworthy happened. Safe to call
// concurrently with Touch/List, which may run on a different goroutine.
func (s *Store) Warning() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.warning
}

// Touch records path as just-visited: it is normalized with filepath.Clean,
// any existing entry for the same path is removed, and a fresh entry with
// the current timestamp is appended so the path sorts as most-recent in
// List. Returns the stored entry.
func (s *Store) Touch(path string) (Entry, error) {
	entries, err := s.load()
	if err != nil {
		return Entry{}, err
	}

	clean := filepath.Clean(path)
	deduped := entries[:0:0]
	for _, e := range entries {
		if e.Path != clean {
			deduped = append(deduped, e)
		}
	}

	e := Entry{Path: clean, Timestamp: time.Now()}
	deduped = append(deduped, e)

	if err := s.save(deduped); err != nil {
		return Entry{}, err
	}
	return e, nil
}

// List returns the most recently touched directories first. A non-positive
// limit returns all entries.
func (s *Store) List(limit int) ([]Entry, error) {
	entries, err := s.load()
	if err != nil {
		return nil, err
	}
	reversed := make([]Entry, len(entries))
	for i, e := range entries {
		reversed[len(entries)-1-i] = e
	}
	if limit > 0 && limit < len(reversed) {
		reversed = reversed[:limit]
	}
	return reversed, nil
}

// load reads all entries from disk. A missing file is treated as empty. A
// file that fails to parse as JSON is backed up alongside itself (so no data
// is silently discarded) and treated as empty going forward; s.warning is set
// to describe what happened.
func (s *Store) load() ([]Entry, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		backupPath := fmt.Sprintf("%s.corrupted-%d", s.path, time.Now().UnixNano())
		if renameErr := os.Rename(s.path, backupPath); renameErr != nil {
			return nil, fmt.Errorf("directory history file %s is corrupted (%w) and could not be backed up: %v", s.path, err, renameErr)
		}
		s.mu.Lock()
		s.warning = fmt.Sprintf("directory history file was corrupted; backed up to %s and started fresh", backupPath)
		s.mu.Unlock()
		return nil, nil
	}
	return entries, nil
}

func (s *Store) save(entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}
