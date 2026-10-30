package env

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func mkdirs(t *testing.T, parts ...string) string {
	t.Helper()
	p := filepath.Join(parts...)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	return p
}

func TestLoadNearest_NearestFolderWins(t *testing.T) {
	root := t.TempDir()
	api := mkdirs(t, root, "api")
	v1 := mkdirs(t, root, "api", "v1")
	writeFile(t, root, PublicFileName, `{"dev": {"host": "root"}}`)
	writeFile(t, api, PublicFileName, `{"dev": {"host": "api"}}`)

	got, err := LoadNearest(v1, root)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.Dir != api || got.Public["dev"]["host"] != "api" {
		t.Errorf("Dir = %q, host = %q", got.Dir, got.Public["dev"]["host"])
	}
	if want := []string{v1, api}; !reflect.DeepEqual(got.Searched, want) {
		t.Errorf("Searched = %v, want %v", got.Searched, want)
	}
}

func TestLoadNearest_WalksUpToRoot(t *testing.T) {
	root := t.TempDir()
	api := mkdirs(t, root, "api")
	writeFile(t, root, PublicFileName, `{"dev": {}}`)

	got, _ := LoadNearest(api, root)
	if got.Dir != root || got.FileName() != "http-client.env.json" {
		t.Errorf("Dir = %q, FileName = %q", got.Dir, got.FileName())
	}
}

func TestLoadNearest_PublicAndPrivateComeFromSameFolder(t *testing.T) {
	root := t.TempDir()
	api := mkdirs(t, root, "api")
	writeFile(t, root, PrivateFileName, `{"dev": {"token": "root-secret"}}`)
	writeFile(t, api, PublicFileName, `{"dev": {}}`)

	got, _ := LoadNearest(api, root)
	if got.Dir != api || got.Private != nil {
		t.Errorf("Dir = %q, Private = %v (root private must not be paired)", got.Dir, got.Private)
	}
}

func TestLoadNearest_PrivateOnlyFolderCounts(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, PrivateFileName, `{"dev": {}}`)

	got, _ := LoadNearest(root, root)
	if got.Dir != root || got.FileName() != "http-client.private.env.json" {
		t.Errorf("Dir = %q, FileName = %q", got.Dir, got.FileName())
	}
}

func TestLoadNearest_NotFoundStopsAtRoot(t *testing.T) {
	parent := t.TempDir()
	writeFile(t, parent, PublicFileName, `{"dev": {}}`)
	root := mkdirs(t, parent, "proj")
	api := mkdirs(t, root, "api")

	got, err := LoadNearest(api, root)
	if err != nil || got.Dir != "" {
		t.Fatalf("Dir = %q, err = %v; env above root must be ignored", got.Dir, err)
	}
	if want := []string{api, root}; !reflect.DeepEqual(got.Searched, want) {
		t.Errorf("Searched = %v, want %v", got.Searched, want)
	}
}

func TestLoadNearest_DirOutsideRootSearchesOnlyDir(t *testing.T) {
	dir := t.TempDir()
	for _, root := range []string{"", t.TempDir()} {
		got, _ := LoadNearest(dir, root)
		if want := []string{dir}; !reflect.DeepEqual(got.Searched, want) {
			t.Errorf("root=%q: Searched = %v, want %v", root, got.Searched, want)
		}
	}
}

func TestLoadNearest_RelativeRoot(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "api")
	writeFile(t, root, PublicFileName, `{"dev": {}}`)
	t.Chdir(root)

	for _, r := range []string{".", "./"} {
		got, err := LoadNearest("api", r)
		if err != nil || got.Dir != "." {
			t.Errorf("root=%q: Dir = %q, err = %v", r, got.Dir, err)
		}
		if want := []string{"api", "."}; !reflect.DeepEqual(got.Searched, want) {
			t.Errorf("root=%q: Searched = %v, want %v", r, got.Searched, want)
		}
	}
}

func TestLoadNearest_ParseErrorStopsSearch(t *testing.T) {
	root := t.TempDir()
	api := mkdirs(t, root, "api")
	writeFile(t, root, PublicFileName, `{"dev": {}}`)
	writeFile(t, api, PublicFileName, `{ broken`)

	got, err := LoadNearest(api, root)
	if err == nil || !strings.HasPrefix(err.Error(), "http-client.env.json: ") {
		t.Fatalf("err = %v", err)
	}
	if got.Dir != api {
		t.Errorf("Dir = %q, want %q (must not fall back to root)", got.Dir, api)
	}
}
