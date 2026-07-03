package env

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
}

func TestLoadFiles_NoFilesPresent(t *testing.T) {
	dir := t.TempDir()

	public, private, err := LoadFiles(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(public) != 0 {
		t.Errorf("public = %v, want empty", public)
	}
	if len(private) != 0 {
		t.Errorf("private = %v, want empty", private)
	}
}

func TestLoadFiles_PublicAndPrivate(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "http-client.env.json", `{
		"dev": {"baseUrl": "https://dev.example.com", "token": "public-dev-token"},
		"prod": {"baseUrl": "https://example.com"}
	}`)
	writeFile(t, dir, "http-client.private.env.json", `{
		"dev": {"token": "private-dev-token"}
	}`)

	public, private, err := LoadFiles(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if public["dev"]["baseUrl"] != "https://dev.example.com" {
		t.Errorf("public dev.baseUrl = %q", public["dev"]["baseUrl"])
	}
	if public["prod"]["baseUrl"] != "https://example.com" {
		t.Errorf("public prod.baseUrl = %q", public["prod"]["baseUrl"])
	}
	if private["dev"]["token"] != "private-dev-token" {
		t.Errorf("private dev.token = %q", private["dev"]["token"])
	}
}

func TestLoadFiles_InvalidJSONIsError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "http-client.env.json", `{ not valid json `)

	_, _, err := LoadFiles(dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestMerge_Precedence(t *testing.T) {
	public := map[string]map[string]string{
		"dev": {"baseUrl": "https://dev.example.com", "token": "public-token", "region": "us"},
	}
	private := map[string]map[string]string{
		"dev": {"token": "private-token"},
	}
	fileVars := map[string]string{
		"token": "file-token",
	}

	got := Merge(public, private, "dev", fileVars)

	want := map[string]string{
		"baseUrl": "https://dev.example.com",
		"token":   "file-token",
		"region":  "us",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Merge() = %v, want %v", got, want)
	}
}

func TestMerge_UnknownEnvNameYieldsOnlyFileVars(t *testing.T) {
	public := map[string]map[string]string{"dev": {"baseUrl": "https://dev.example.com"}}
	fileVars := map[string]string{"token": "abc"}

	got := Merge(public, nil, "staging", fileVars)

	want := map[string]string{"token": "abc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Merge() = %v, want %v", got, want)
	}
}

func TestEnvNames_UnionSorted(t *testing.T) {
	public := map[string]map[string]string{"prod": {}, "dev": {}}
	private := map[string]map[string]string{"dev": {}, "staging": {}}

	got := EnvNames(public, private)
	want := []string{"dev", "prod", "staging"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("EnvNames() = %v, want %v", got, want)
	}
}

func TestSubstitute_ReplacesKnownVariables(t *testing.T) {
	vars := map[string]string{"baseUrl": "https://example.com", "id": "42"}

	got, missing := Substitute("GET {{baseUrl}}/users/{{id}}", vars)

	if got != "GET https://example.com/users/42" {
		t.Errorf("result = %q", got)
	}
	if len(missing) != 0 {
		t.Errorf("missing = %v, want empty", missing)
	}
}

func TestSubstitute_ReportsMissingVariables(t *testing.T) {
	vars := map[string]string{"baseUrl": "https://example.com"}

	got, missing := Substitute("GET {{baseUrl}}/users/{{id}}?key={{apiKey}}", vars)

	if got != "GET https://example.com/users/{{id}}?key={{apiKey}}" {
		t.Errorf("result = %q", got)
	}
	want := []string{"apiKey", "id"}
	if !reflect.DeepEqual(missing, want) {
		t.Errorf("missing = %v, want %v", missing, want)
	}
}

func TestSubstitute_NoPlaceholdersReturnsUnchanged(t *testing.T) {
	got, missing := Substitute("plain text", map[string]string{"x": "y"})
	if got != "plain text" {
		t.Errorf("result = %q", got)
	}
	if len(missing) != 0 {
		t.Errorf("missing = %v, want empty", missing)
	}
}

func TestSubstitute_DuplicatePlaceholderCountsOnceInMissing(t *testing.T) {
	_, missing := Substitute("{{id}}-{{id}}", map[string]string{})
	want := []string{"id"}
	if !reflect.DeepEqual(missing, want) {
		t.Errorf("missing = %v, want %v", missing, want)
	}
}
