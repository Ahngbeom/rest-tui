package output

import (
	"strings"
	"testing"
	"time"

	"github.com/Ahngbeom/rest-tui/internal/executor"
	"github.com/Ahngbeom/rest-tui/internal/httpfile"
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

func TestRenderTransaction_FullExample(t *testing.T) {
	req := httpfile.Request{
		Method:  "POST",
		URL:     "https://api.example.com/users",
		Headers: []httpfile.Header{{Name: "Content-Type", Value: "application/json"}},
		Body:    `{"name":"foo"}`,
	}
	resp := &executor.Response{
		Status:   "201 Created",
		Duration: 23 * time.Millisecond,
		Headers:  []httpfile.Header{{Name: "Content-Type", Value: "application/json"}},
		Body:     []byte(`{"ok":true}`),
	}

	got := RenderTransaction(req, resp, "", Options{Color: false})

	want := "POST https://api.example.com/users\n" +
		"Content-Type: application/json\n\n" +
		"{\n  \"name\": \"foo\"\n}\n\n" +
		strings.Repeat("─", 40) + "\n\n" +
		"201 Created (23ms)\n\n" +
		"Content-Type: application/json\n\n" +
		"{\n  \"ok\": true\n}"
	if got != want {
		t.Errorf("RenderTransaction() =\n%q\nwant\n%q", got, want)
	}
	if strings.Contains(got, "\x1b[") {
		t.Errorf("RenderTransaction() with Color=false should not contain ANSI escapes, got %q", got)
	}
}

func TestRenderTransaction_EmptyBodyOmitsBodySection(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://api.example.com/users"}
	resp := &executor.Response{Status: "204 No Content", Duration: time.Millisecond}

	got := RenderTransaction(req, resp, "", Options{})

	want := "GET https://api.example.com/users\n\n" +
		strings.Repeat("─", 40) + "\n\n" +
		"204 No Content (1ms)"
	if got != want {
		t.Errorf("RenderTransaction() = %q, want %q", got, want)
	}
}

func TestRenderTransaction_NilResponseUsesNote(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://api.example.com/users"}

	got := RenderTransaction(req, nil, "(not yet sent)", Options{})

	if !strings.HasSuffix(got, "(not yet sent)") {
		t.Errorf("RenderTransaction() = %q, want it to end with the note", got)
	}
}

func TestRenderTransaction_ColorOptionAddsAnsiEscapes(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://api.example.com/users", Body: `{"a":1}`}
	resp := &executor.Response{Status: "200 OK", Duration: time.Millisecond, Body: []byte(`{"a":1}`)}

	got := RenderTransaction(req, resp, "", Options{Color: true})

	if !strings.Contains(got, "\x1b[") {
		t.Errorf("RenderTransaction() with Color=true should contain ANSI escapes, got %q", got)
	}
}

func TestRenderResponse_SanitizesHeadersAndBody(t *testing.T) {
	resp := &executor.Response{
		Status:  "200 OK",
		Headers: []httpfile.Header{{Name: "X-Evil", Value: "a\x1b]0;T\x07b"}},
		Body:    []byte("hello \x1b[2Jworld"),
	}
	got := RenderResponse(resp, Options{})
	if strings.Contains(got, "\x1b") {
		t.Errorf("RenderResponse() = %q, want no ESC bytes", got)
	}
	if !strings.Contains(got, "X-Evil: ab") || !strings.Contains(got, "hello world") {
		t.Errorf("RenderResponse() = %q", got)
	}
	resp = &executor.Response{Status: "200 OK", Body: []byte("{\"a\":\"x\u009b2Jy\"}")}
	if got := RenderResponse(resp, Options{Color: true}); strings.Contains(got, "\u009b") || !strings.Contains(got, "2Jy") {
		t.Errorf("RenderResponse(JSON with raw C1) = %q, want C1 removed", got)
	}
}

func TestRenderResponse_BinaryBodyPlaceholder(t *testing.T) {
	body := make([]byte, 5120)
	for i := range body {
		body[i] = byte(i)
	}
	resp := &executor.Response{Status: "200 OK", Headers: []httpfile.Header{{Name: "Content-Type", Value: "image/png"}}, Body: body}
	if got := RenderResponse(resp, Options{}); !strings.Contains(got, "(binary body: 5120 bytes, image/png)") {
		t.Errorf("RenderResponse() = %q", got)
	}
	resp = &executor.Response{Status: "200 OK", Body: []byte{0xff, 0xfe, 0x00}}
	if got := RenderResponse(resp, Options{}); !strings.Contains(got, "(binary body: 3 bytes, unknown)") {
		t.Errorf("RenderResponse() = %q", got)
	}
}
