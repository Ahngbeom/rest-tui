// Package env loads IntelliJ HTTP Client environment files
// (http-client.env.json / http-client.private.env.json) and resolves
// {{variable}} placeholders against them.
package env

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const (
	publicFileName  = "http-client.env.json"
	privateFileName = "http-client.private.env.json"
)

// LoadFiles reads the public and private environment files from dir. A
// missing file yields a nil map for that side, not an error.
func LoadFiles(dir string) (public, private map[string]map[string]string, err error) {
	public, err = loadOne(filepath.Join(dir, publicFileName))
	if err != nil {
		return nil, nil, err
	}
	private, err = loadOne(filepath.Join(dir, privateFileName))
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
	var envs map[string]map[string]string
	if err := json.Unmarshal(data, &envs); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return envs, nil
}

// Merge resolves the final variable map for envName using precedence
// (highest first): fileVars, private[envName], public[envName].
func Merge(public, private map[string]map[string]string, envName string, fileVars map[string]string) map[string]string {
	result := map[string]string{}
	for k, v := range public[envName] {
		result[k] = v
	}
	for k, v := range private[envName] {
		result[k] = v
	}
	for k, v := range fileVars {
		result[k] = v
	}
	return result
}

// EnvNames returns the sorted union of environment names declared in public
// and private.
func EnvNames(public, private map[string]map[string]string) []string {
	seen := map[string]bool{}
	for name := range public {
		seen[name] = true
	}
	for name := range private {
		seen[name] = true
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
