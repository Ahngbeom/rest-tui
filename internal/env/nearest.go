package env

import (
	"os"
	"path/filepath"
	"strings"
)

// Loaded is the env file pair found by LoadNearest. Dir is empty when no
// folder had either file; Searched lists the folders checked, nearest first.
type Loaded struct {
	Public, Private map[string]map[string]string
	Dir             string
	Searched        []string
}

// FileName names the file the environments came from, preferring the public one.
func (l Loaded) FileName() string {
	switch {
	case l.Public != nil:
		return PublicFileName
	case l.Private != nil:
		return PrivateFileName
	}
	return ""
}

// LoadNearest looks for an env file in dir, then each parent up to and
// including root, and reads the public/private pair from the first folder
// that has either. A dir outside root (or an empty root) checks dir only.
func LoadNearest(dir, root string) (Loaded, error) {
	dir = filepath.Clean(dir)
	var out Loaded
	stopAtRoot := root != "" && within(filepath.Clean(root), dir)
	if stopAtRoot {
		root = filepath.Clean(root)
	}

	for cur := dir; ; cur = filepath.Dir(cur) {
		out.Searched = append(out.Searched, cur)
		if hasEnvFile(cur) {
			out.Dir = cur
			var err error
			out.Public, out.Private, err = LoadFiles(cur)
			return out, err
		}
		if !stopAtRoot || cur == root || filepath.Dir(cur) == cur {
			return out, nil
		}
	}
}

func hasEnvFile(dir string) bool {
	for _, name := range []string{PublicFileName, PrivateFileName} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func within(root, dir string) bool {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
