package env

import (
	"math/rand/v2"
	"net/url"
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
