package env

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestLoadFiles_StringifiesScalarValues(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, PublicFileName, `{"local": {
		"host": "http://127.0.0.1:18091", "port": 18091, "debug": true, "off": false,
		"ratio": 1.5, "big": 12345678901234567890,
		"SSLConfiguration": {"clientCertificate": "a.pem"}, "list": [1, 2]
	}}`)

	public, _, err := LoadFiles(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{
		"host": "http://127.0.0.1:18091", "port": "18091", "debug": "true", "off": "false",
		"ratio": "1.5", "big": "12345678901234567890",
	}
	if !reflect.DeepEqual(public["local"], want) {
		t.Errorf("local = %v, want %v", public["local"], want)
	}
}

func TestLoadFiles_SkipsNullAndNonObjectEnvs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, PublicFileName, `{"dev": "oops", "qa": null, "prod": {"a": "b", "n": null}}`)

	public, _, err := LoadFiles(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]map[string]string{"prod": {"a": "b"}}
	if !reflect.DeepEqual(public, want) {
		t.Errorf("public = %v, want %v", public, want)
	}
}

func TestLoadFiles_EmptyFileYieldsNonNilMap(t *testing.T) {
	for _, content := range []string{`{}`, ``, " \n"} {
		dir := t.TempDir()
		writeFile(t, dir, PrivateFileName, content)

		public, private, err := LoadFiles(dir)
		if err != nil {
			t.Fatalf("content %q: unexpected error: %v", content, err)
		}
		if public != nil {
			t.Errorf("content %q: public = %v, want nil (file absent)", content, public)
		}
		if private == nil {
			t.Errorf("content %q: private = nil, want non-nil map (file present)", content)
		}
	}
}

func TestLoadFiles_ErrorNamesFileByBaseName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, PublicFileName, `{ not valid json `)

	_, _, err := LoadFiles(dir)
	if err == nil || !strings.HasPrefix(err.Error(), "http-client.env.json: ") {
		t.Fatalf("err = %v, want prefix %q", err, "http-client.env.json: ")
	}
}

func TestMerge_SharedLayersUnderEveryEnv(t *testing.T) {
	public := map[string]map[string]string{
		"$shared": {"k1": "public-shared", "k2": "public-shared", "k3": "public-shared", "k4": "public-shared", "k5": "public-shared"},
		"dev":     {"k1": "public-dev", "k2": "public-dev", "k3": "public-dev"},
	}
	private := map[string]map[string]string{
		"$shared": {"k1": "private-shared", "k2": "private-shared", "k3": "private-shared", "k4": "private-shared"},
		"dev":     {"k1": "private-dev", "k2": "private-dev"},
	}
	got := Merge(public, private, "dev", map[string]string{"k1": "file"})

	want := map[string]string{"k1": "file", "k2": "private-dev", "k3": "public-dev", "k4": "private-shared", "k5": "public-shared"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Merge() = %v, want %v", got, want)
	}
}

func TestMerge_SharedAppliesWithNoEnvSelected(t *testing.T) {
	public := map[string]map[string]string{"$shared": {"host": "https://shared.example.com"}}
	got := Merge(public, nil, "", nil)
	if got["host"] != "https://shared.example.com" {
		t.Errorf("host = %q", got["host"])
	}
}

func TestEnvNames_ExcludesShared(t *testing.T) {
	public := map[string]map[string]string{"$shared": {}, "prod": {}}
	private := map[string]map[string]string{"$shared": {}, "dev": {}}
	want := []string{"dev", "prod"}
	if got := EnvNames(public, private); !reflect.DeepEqual(got, want) {
		t.Errorf("EnvNames() = %v, want %v", got, want)
	}
}
