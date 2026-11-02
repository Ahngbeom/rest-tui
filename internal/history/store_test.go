package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
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

func TestStore_ConcurrentAppendDoesNotLoseEntries(t *testing.T) {
	s, _ := newTestStore(t)

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			_, err := s.Append(Entry{Method: "GET", URL: fmt.Sprintf("https://example.com/%d", i)})
			if err != nil {
				t.Errorf("Append %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	entries, err := s.List(0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != n {
		t.Fatalf("expected %d entries after concurrent Append, got %d (lost update)", n, len(entries))
	}

	seen := make(map[string]bool, n)
	for _, e := range entries {
		seen[e.URL] = true
	}
	for i := 0; i < n; i++ {
		want := fmt.Sprintf("https://example.com/%d", i)
		if !seen[want] {
			t.Errorf("missing entry with URL %q after concurrent Append", want)
		}
	}
}

func TestStore_WarningIsSafeForConcurrentAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
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

func TestStore_EnvRoundTripsAndIsOmittedWhenEmpty(t *testing.T) {
	s, path := newTestStore(t)
	if _, err := s.Append(Entry{Method: "GET", URL: "https://example.com"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), `"Env"`) {
		t.Errorf("empty Env should be omitted: %s", data)
	}
	if _, err := s.Append(Entry{Method: "GET", URL: "https://example.com", Env: "dev"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, _ := s.List(1)
	if got[0].Env != "dev" {
		t.Errorf("Env = %q, want dev", got[0].Env)
	}
}

func skipIfNoPermEnforcement(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file permission bits are not enforced here")
	}
}

func TestStore_NewFileHasMode0600(t *testing.T) {
	skipIfNoPermEnforcement(t)
	s, path := newTestStore(t)
	if _, err := s.Append(Entry{Method: "GET", URL: "u"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %v, want 0600", got)
	}
}

func TestStore_AppendTightensExistingFileTo0600(t *testing.T) {
	skipIfNoPermEnforcement(t)
	s, path := newTestStore(t)
	if err := os.WriteFile(path, []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(Entry{URL: "u"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %v, want 0600", got)
	}
}

func TestStore_AppendKeepsNewest500(t *testing.T) {
	s, path := newTestStore(t)
	seed := make([]Entry, 500)
	for i := range seed {
		seed[i] = Entry{ID: fmt.Sprintf("id%d", i), URL: fmt.Sprintf("u%d", i)}
	}
	data, err := json.Marshal(seed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(Entry{URL: "new"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, err := s.List(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 500 {
		t.Fatalf("len = %d, want 500", len(got))
	}
	if got[0].URL != "new" || got[499].URL != "u1" {
		t.Errorf("got[0]=%q got[499]=%q, want new / u1", got[0].URL, got[499].URL)
	}
}

func TestStore_AppendLeavesNoTempFiles(t *testing.T) {
	s, path := newTestStore(t)
	for i := 0; i < 2; i++ {
		if _, err := s.Append(Entry{URL: "u"}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	files, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, f.Name())
	}
	if !reflect.DeepEqual(names, []string{"history.json"}) {
		t.Errorf("dir contents = %v, want [history.json]", names)
	}
}

func TestStore_FailedWriteKeepsOriginal(t *testing.T) {
	skipIfNoPermEnforcement(t)
	s, path := newTestStore(t)
	if _, err := s.Append(Entry{URL: "first"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	if _, err := s.Append(Entry{URL: "second"}); err == nil {
		t.Fatal("expected error when directory is read-only")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("original file changed after failed write")
	}
}

func TestStore_NameAndRedirectsRoundTrip(t *testing.T) {
	s, _ := newTestStore(t)
	want := Entry{Name: "Create user", Redirects: []string{"302 \u2192 GET http://x/new"}}
	saved, err := s.Append(want)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, err := s.Get(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != want.Name || !reflect.DeepEqual(got.Redirects, want.Redirects) {
		t.Errorf("round trip: Name=%q Redirects=%v", got.Name, got.Redirects)
	}

	s2, path2 := newTestStore(t)
	if _, err := s2.Append(Entry{URL: "u"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path2)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"Redirects", `"Name"`} {
		if strings.Contains(string(raw), key) {
			t.Errorf("empty entry file should not contain %s:\n%s", key, raw)
		}
	}
}

func TestStore_BinaryRequestBodyRoundTripsExactly(t *testing.T) {
	s, path := newTestStore(t)
	body := "\x00\xff\x80\x01"
	if _, err := s.Append(Entry{Method: "POST", URL: "https://example.com", RequestBody: body}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, err := NewStore(path).List(1)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got[0].RequestBody != body {
		t.Errorf("RequestBody = % x, want % x", got[0].RequestBody, body)
	}
}

func TestStore_TextRequestBodyIsStoredAsPlainText(t *testing.T) {
	s, path := newTestStore(t)
	if _, err := s.Append(Entry{Method: "POST", URL: "https://example.com", RequestBody: `{"name":"한글"}`}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "RequestBodyBase64") {
		t.Errorf("text body should not be base64-encoded: %s", data)
	}
	got, _ := s.List(1)
	if got[0].RequestBody != `{"name":"한글"}` {
		t.Errorf("RequestBody = %q", got[0].RequestBody)
	}
}

func TestStore_LoadsEntriesWrittenWithoutBase64Field(t *testing.T) {
	s, path := newTestStore(t)
	old := `[{"ID":"old-1","Method":"POST","URL":"https://example.com","RequestBody":"plain"}]`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := s.List(0)
	if err != nil || len(got) != 1 || got[0].RequestBody != "plain" || s.Warning() != "" {
		t.Errorf("List = %+v, err = %v, warning = %q", got, err, s.Warning())
	}
}
