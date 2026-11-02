package env

import (
	"math/rand/v2"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Ahngbeom/rest-tui/internal/httpfile"
)

func TestResolveRequest_SubstitutesAllParts(t *testing.T) {
	req := httpfile.Request{
		Method: "POST",
		URL:    "{{baseUrl}}/users",
		Headers: []httpfile.Header{
			{Name: "Authorization", Value: "Bearer {{token}}"},
		},
		Body: `{"name": "{{userName}}"}`,
	}
	vars := map[string]string{
		"baseUrl":  "https://example.com",
		"token":    "secret",
		"userName": "Bob",
	}

	resolved, missing := ResolveRequest(req, vars, ResolveOptions{})

	if len(missing) != 0 {
		t.Fatalf("missing = %v, want empty", missing)
	}
	if resolved.URL != "https://example.com/users" {
		t.Errorf("URL = %q", resolved.URL)
	}
	if resolved.Headers[0].Value != "Bearer secret" {
		t.Errorf("Authorization = %q", resolved.Headers[0].Value)
	}
	if resolved.Body != `{"name": "Bob"}` {
		t.Errorf("Body = %q", resolved.Body)
	}
	// Method/Name are passed through untouched.
	if resolved.Method != "POST" {
		t.Errorf("Method = %q", resolved.Method)
	}
}

func TestResolveRequest_CollectsMissingAcrossAllParts(t *testing.T) {
	req := httpfile.Request{
		Method: "GET",
		URL:    "{{baseUrl}}/users/{{id}}",
		Headers: []httpfile.Header{
			{Name: "X-Trace", Value: "{{traceId}}"},
		},
	}

	_, missing := ResolveRequest(req, map[string]string{"baseUrl": "https://example.com"}, ResolveOptions{})

	want := []string{"id", "traceId"}
	if !reflect.DeepEqual(missing, want) {
		t.Errorf("missing = %v, want %v", missing, want)
	}
}

func testResolveOptions(dir string) ResolveOptions {
	return ResolveOptions{BaseDir: dir, Now: testNow(), Rand: rand.New(rand.NewPCG(1, 2))}
}

func TestResolveRequest_DynamicVariablesAreDeterministic(t *testing.T) {
	req := httpfile.Request{
		Method:  "POST",
		URL:     "https://example.com/{{$uuid}}?ts={{$timestamp}}",
		Headers: []httpfile.Header{{Name: "X-Request-Id", Value: "{{ $random.uuid }}"}},
		Body:    `{"n": {{$random.integer(1, 10)}}, "at": "{{$isoTimestamp}}"}`,
	}
	a, missingA := ResolveRequest(req, nil, testResolveOptions(""))
	b, _ := ResolveRequest(req, nil, testResolveOptions(""))
	if len(missingA) != 0 {
		t.Fatalf("missing = %v, want empty", missingA)
	}
	if !reflect.DeepEqual(a, b) {
		t.Errorf("same seed produced different requests:\n%+v\n%+v", a, b)
	}
	if !strings.HasSuffix(a.URL, "?ts=1793350800") || !uuidV4RE.MatchString(a.Headers[0].Value) {
		t.Errorf("resolved = %+v", a)
	}
	if !strings.Contains(a.Body, `"at": "2026-10-30T09:00:00Z"`) {
		t.Errorf("Body = %q", a.Body)
	}
}

func TestResolveRequest_EachDynamicOccurrenceIsIndependent(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com/?a={{$uuid}}&b={{$uuid}}"}
	got, _ := ResolveRequest(req, nil, testResolveOptions(""))
	q, _ := url.ParseQuery(strings.SplitN(got.URL, "?", 2)[1])
	if q.Get("a") == q.Get("b") {
		t.Errorf("two {{$uuid}} produced the same value %q", q.Get("a"))
	}
}

func TestResolveRequest_UnknownDollarVariableIsMissing(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com/{{$foo}}/{{id}}"}
	got, missing := ResolveRequest(req, map[string]string{"$foo": "x"}, testResolveOptions(""))
	if want := []string{"$foo", "id"}; !reflect.DeepEqual(missing, want) {
		t.Errorf("missing = %v, want %v", missing, want)
	}
	if got.URL != "https://example.com/{{$foo}}/{{id}}" {
		t.Errorf("URL = %q, placeholders must stay literal", got.URL)
	}
}

func TestResolveRequest_PreservesUnsupported(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com", Unsupported: []string{"response handler script"}}
	got, _ := ResolveRequest(req, nil, testResolveOptions(""))
	if !reflect.DeepEqual(got.Unsupported, req.Unsupported) {
		t.Errorf("Unsupported = %v", got.Unsupported)
	}
}

func TestResolveRequest_IncludesBodyFileRelativeToBaseDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "body.json", "{\"a\": 1}\n")
	req := httpfile.Request{Method: "POST", URL: "https://example.com", Body: "< ./body.json"}
	got, missing := ResolveRequest(req, nil, testResolveOptions(dir))
	if got.Body != "{\"a\": 1}\n" || len(missing) != 0 {
		t.Errorf("Body = %q, missing = %v", got.Body, missing)
	}
}

func TestResolveRequest_WholeBodyIncludeKeepsTrailingCRLF(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "data.ndjson", "{\"a\":1}\r\n{\"b\":2}\r\n")
	req := httpfile.Request{Method: "POST", URL: "https://example.com", Body: "< ./data.ndjson"}
	got, _ := ResolveRequest(req, nil, testResolveOptions(dir))
	if want := "{\"a\":1}\r\n{\"b\":2}\r\n"; got.Body != want {
		t.Errorf("Body = %q, want %q", got.Body, want)
	}
}

func TestResolveRequest_IncludesFileInsideMultipartPart(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "hello\n")
	body := "--b\nContent-Disposition: form-data; name=\"f\"; filename=\"a.txt\"\n\n< ./a.txt\n--b--"
	got, _ := ResolveRequest(httpfile.Request{Method: "POST", URL: "https://example.com", Body: body}, nil, testResolveOptions(dir))
	want := "--b\nContent-Disposition: form-data; name=\"f\"; filename=\"a.txt\"\n\nhello\n--b--"
	if got.Body != want {
		t.Errorf("Body = %q, want %q", got.Body, want)
	}
}

func TestResolveRequest_IncludePathIsSubstitutedButContentIsNot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "tpl.json", `{"name": "{{name}}"}`)
	req := httpfile.Request{Method: "POST", URL: "https://example.com", Body: "< ./{{file}}"}
	got, missing := ResolveRequest(req, map[string]string{"file": "tpl.json", "name": "Bob"}, testResolveOptions(dir))
	if got.Body != `{"name": "{{name}}"}` || len(missing) != 0 {
		t.Errorf("Body = %q, missing = %v", got.Body, missing)
	}
}

func TestResolveRequest_AbsoluteIncludePath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "abs.txt", "abs")
	req := httpfile.Request{Method: "POST", URL: "https://example.com", Body: "< " + filepath.Join(dir, "abs.txt")}
	got, _ := ResolveRequest(req, nil, testResolveOptions(t.TempDir()))
	if got.Body != "abs" {
		t.Errorf("Body = %q", got.Body)
	}
}

func TestResolveRequest_MissingFileIsReportedAfterVariables(t *testing.T) {
	req := httpfile.Request{Method: "POST", URL: "{{host}}/x", Body: "< ./nope.json\n< ./nope.json"}
	got, missing := ResolveRequest(req, nil, testResolveOptions(t.TempDir()))
	if want := []string{"host", "missing file: ./nope.json"}; !reflect.DeepEqual(missing, want) {
		t.Errorf("missing = %v, want %v", missing, want)
	}
	if got.Body != "< ./nope.json\n< ./nope.json" {
		t.Errorf("Body = %q, unreadable include lines must stay as written", got.Body)
	}
}

func TestResolveRequest_XMLBodyLinesAreNotIncludes(t *testing.T) {
	body := "<?xml version=\"1.0\"?>\n<root>\n  <child/>\n</root>"
	got, missing := ResolveRequest(httpfile.Request{Method: "POST", URL: "https://example.com", Body: body}, nil, testResolveOptions(t.TempDir()))
	if got.Body != body || len(missing) != 0 {
		t.Errorf("Body = %q, missing = %v", got.Body, missing)
	}
}

func TestResolveRequest_EmptyBaseDirSkipsIncludes(t *testing.T) {
	req := httpfile.Request{Method: "POST", URL: "https://example.com", Body: "< ./body.json"}
	got, missing := ResolveRequest(req, nil, testResolveOptions(""))
	if got.Body != "< ./body.json" || len(missing) != 0 {
		t.Errorf("Body = %q, missing = %v; an empty BaseDir must not read files", got.Body, missing)
	}
}

func TestEnsureScheme(t *testing.T) {
	cases := map[string]string{
		"localhost:8080/x":    "http://localhost:8080/x",
		"example.com/api?a=1": "http://example.com/api?a=1",
		"https://example.com": "https://example.com",
		"HTTP://example.com":  "HTTP://example.com",
		"{{host}}/x":          "{{host}}/x",
		"":                    "",
	}
	for in, want := range cases {
		if got := EnsureScheme(in); got != want {
			t.Errorf("EnsureScheme(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveRequest_AddsSchemeAfterSubstitution(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "{{host}}/users"}
	got, _ := ResolveRequest(req, map[string]string{"host": "localhost:8080"}, testResolveOptions(""))
	if got.URL != "http://localhost:8080/users" {
		t.Errorf("URL = %q", got.URL)
	}
	got, _ = ResolveRequest(req, map[string]string{"host": "https://api.example.com"}, testResolveOptions(""))
	if got.URL != "https://api.example.com/users" {
		t.Errorf("URL = %q", got.URL)
	}
}
