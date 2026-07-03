package output

import (
	"strings"
	"testing"
	"time"

	"github.com/bahn/rest-tui/internal/executor"
	"github.com/bahn/rest-tui/internal/httpfile"
)

func TestRenderResponse_FullExample(t *testing.T) {
	resp := &executor.Response{
		Status:   "201 Created",
		Duration: 23 * time.Millisecond,
		Headers: []httpfile.Header{
			{Name: "X-Custom", Value: "yes"},
			{Name: "Content-Type", Value: "application/json"},
		},
		Body: []byte(`{"ok":true}`),
	}

	got := RenderResponse(resp, Options{Color: false})

	want := "201 Created (23ms)\n\n" +
		"Content-Type: application/json\n" +
		"X-Custom: yes\n\n" +
		"{\n  \"ok\": true\n}"
	if got != want {
		t.Errorf("RenderResponse() =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderResponse_NonJSONBodyPassthrough(t *testing.T) {
	resp := &executor.Response{
		Status:   "200 OK",
		Duration: time.Millisecond,
		Body:     []byte("plain text response"),
	}

	got := RenderResponse(resp, Options{})

	if !strings.Contains(got, "plain text response") {
		t.Errorf("RenderResponse() = %q, want it to contain the raw body", got)
	}
	if strings.Contains(got, "\"") {
		t.Errorf("RenderResponse() = %q, should not attempt JSON formatting", got)
	}
}

func TestRenderResponse_EmptyBodyOmitsBodySection(t *testing.T) {
	resp := &executor.Response{Status: "204 No Content", Duration: time.Millisecond}

	got := RenderResponse(resp, Options{})

	want := "204 No Content (1ms)"
	if got != want {
		t.Errorf("RenderResponse() = %q, want %q", got, want)
	}
}

func TestRenderResponse_NoHeadersOmitsHeaderSection(t *testing.T) {
	resp := &executor.Response{
		Status:   "200 OK",
		Duration: time.Millisecond,
		Body:     []byte(`{"a":1}`),
	}

	got := RenderResponse(resp, Options{})

	if strings.Count(got, "\n\n") != 1 {
		t.Errorf("RenderResponse() = %q, want exactly one section break (status, body)", got)
	}
}

func TestRenderResponse_ColorOptionAddsAnsiEscapes(t *testing.T) {
	resp := &executor.Response{
		Status:   "200 OK",
		Duration: time.Millisecond,
		Body:     []byte(`{"a":1}`),
	}

	got := RenderResponse(resp, Options{Color: true})

	if !strings.Contains(got, "\x1b[") {
		t.Errorf("RenderResponse() with Color=true should contain ANSI escapes, got %q", got)
	}
}

func TestPrettyBody_InvalidJSONReturnsTrimmedRaw(t *testing.T) {
	got := PrettyBody([]byte("  not json  "), Options{})
	if got != "not json" {
		t.Errorf("PrettyBody() = %q, want %q", got, "not json")
	}
}
