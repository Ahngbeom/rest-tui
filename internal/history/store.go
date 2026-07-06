// Package history persists a local record of executed requests/responses as
// a single JSON file, so past runs can be browsed and re-run from the TUI.
package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Ahngbeom/rest-tui/internal/httpfile"
)

// Entry is one recorded execution.
type Entry struct {
	ID              string
	Timestamp       time.Time
	Method          string
	URL             string
	RequestHeaders  []httpfile.Header
	RequestBody     string
	StatusCode      int
	Status          string
	ResponseHeaders []httpfile.Header
	ResponseBody    string
	Duration        time.Duration
	// Error holds the execution failure message, if the request could not
	// be completed (network error, timeout, etc).
	Error string
}

// Store reads and appends Entry records to a JSON file on disk.
type Store struct {
	path    string
	warning string
}

// NewStore returns a Store backed by the file at path. The file need not
// exist yet; it is created on first Append.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Warning returns a human-readable message describing the most recent
// recovery action (e.g. a corrupted history file being backed up and
// replaced), or "" if nothing noteworthy happened.
func (s *Store) Warning() string {
	return s.warning
}

// Append records a new entry, filling in ID/Timestamp if they are zero, and
// returns the stored entry.
func (s *Store) Append(e Entry) (Entry, error) {
	entries, err := s.load()
	if err != nil {
		return Entry{}, err
	}

	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	if e.ID == "" {
		e.ID = fmt.Sprintf("%s-%04d", e.Timestamp.UTC().Format("20060102T150405.000000000"), len(entries))
	}
	entries = append(entries, e)

	if err := s.save(entries); err != nil {
		return Entry{}, err
	}
	return e, nil
}

// List returns the most recently recorded entries first. A non-positive
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

// Get returns the entry with the given ID.
func (s *Store) Get(id string) (*Entry, error) {
	entries, err := s.load()
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.ID == id {
			return &e, nil
		}
	}
	return nil, fmt.Errorf("history: no entry with id %q", id)
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
			return nil, fmt.Errorf("history file %s is corrupted (%w) and could not be backed up: %v", s.path, err, renameErr)
		}
		s.warning = fmt.Sprintf("history file was corrupted; backed up to %s and started fresh", backupPath)
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
