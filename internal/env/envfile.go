// Package env loads IntelliJ HTTP Client environment files
// (http-client.env.json / http-client.private.env.json) and resolves
// {{variable}} placeholders against them.
package env

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

const (
	PublicFileName  = "http-client.env.json"
	PrivateFileName = "http-client.private.env.json"

	// SharedEnvName is the pseudo-environment whose values sit under every real one.
	SharedEnvName = "$shared"
)

// LoadFiles reads the public and private environment files from dir. A
// missing file yields a nil map for that side, not an error.
func LoadFiles(dir string) (public, private map[string]map[string]string, err error) {
	public, err = loadOne(filepath.Join(dir, PublicFileName))
	if err != nil {
		return nil, nil, err
	}
	private, err = loadOne(filepath.Join(dir, PrivateFileName))
	if err != nil {
		return nil, nil, err
	}
	return public, private, nil
}

func loadOne(path string) (map[string]map[string]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	envs := map[string]map[string]string{}
	if len(bytes.TrimSpace(data)) == 0 {
		return envs, nil
	}
	// UseNumber keeps large integers exact; decoding into float64 would rewrite them.
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	for name, v := range raw {
		obj, ok := v.(map[string]any)
		if !ok {
			continue
		}
		vars := map[string]string{}
		for k, val := range obj {
			switch x := val.(type) {
			case string:
				vars[k] = x
			case json.Number:
				vars[k] = x.String()
			case bool:
				vars[k] = strconv.FormatBool(x)
			}
		}
		envs[name] = vars
	}
	return envs, nil
}

// Merge resolves the final variable map for envName using precedence
// (highest first): fileVars, private[envName], public[envName],
// private[$shared], public[$shared]. The $shared layers apply even when
// envName is empty.
func Merge(public, private map[string]map[string]string, envName string, fileVars map[string]string) map[string]string {
	result := map[string]string{}
	for _, layer := range []map[string]string{
		public[SharedEnvName],
		private[SharedEnvName],
		public[envName],
		private[envName],
		fileVars,
	} {
		for k, v := range layer {
			result[k] = v
		}
	}
	return result
}

// EnvNames returns the sorted union of environment names declared in public
// and private, excluding the $shared pseudo-environment.
func EnvNames(public, private map[string]map[string]string) []string {
	seen := map[string]bool{}
	for name := range public {
		seen[name] = true
	}
	for name := range private {
		seen[name] = true
	}
	delete(seen, SharedEnvName)
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
