package env

import (
	"reflect"
	"testing"

	"github.com/bahn/rest-tui/internal/httpfile"
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

	resolved, missing := ResolveRequest(req, vars)

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

	_, missing := ResolveRequest(req, map[string]string{"baseUrl": "https://example.com"})

	want := []string{"id", "traceId"}
	if !reflect.DeepEqual(missing, want) {
		t.Errorf("missing = %v, want %v", missing, want)
	}
}
