package httpfile

import (
	"testing"
)

func TestParse_SingleRequestNoDelimiter(t *testing.T) {
	src := "GET https://example.com/api\n"

	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(f.Requests))
	}
	req := f.Requests[0]
	if req.Method != "GET" {
		t.Errorf("Method = %q, want GET", req.Method)
	}
	if req.URL != "https://example.com/api" {
		t.Errorf("URL = %q, want https://example.com/api", req.URL)
	}
	if len(req.Headers) != 0 {
		t.Errorf("Headers = %v, want empty", req.Headers)
	}
	if req.Body != "" {
		t.Errorf("Body = %q, want empty", req.Body)
	}
}

func TestParse_MultipleRequestsWithHeadersAndBody(t *testing.T) {
	src := `### Get user
GET {{baseUrl}}/users/1

### Create user
POST {{baseUrl}}/users
Content-Type: application/json
Authorization: Bearer {{token}}

{
  "name": "Bob"
}
`
	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.Requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(f.Requests))
	}

	first := f.Requests[0]
	if first.Name != "Get user" {
		t.Errorf("first.Name = %q, want %q", first.Name, "Get user")
	}
	if first.Method != "GET" || first.URL != "{{baseUrl}}/users/1" {
		t.Errorf("first request = %+v", first)
	}
	if len(first.Headers) != 0 {
		t.Errorf("first.Headers = %v, want empty", first.Headers)
	}
	if first.Body != "" {
		t.Errorf("first.Body = %q, want empty", first.Body)
	}

	second := f.Requests[1]
	if second.Name != "Create user" {
		t.Errorf("second.Name = %q, want %q", second.Name, "Create user")
	}
	if second.Method != "POST" || second.URL != "{{baseUrl}}/users" {
		t.Errorf("second request = %+v", second)
	}
	wantHeaders := []Header{
		{Name: "Content-Type", Value: "application/json"},
		{Name: "Authorization", Value: "Bearer {{token}}"},
	}
	if len(second.Headers) != len(wantHeaders) {
		t.Fatalf("second.Headers = %v, want %v", second.Headers, wantHeaders)
	}
	for i, h := range wantHeaders {
		if second.Headers[i] != h {
			t.Errorf("second.Headers[%d] = %+v, want %+v", i, second.Headers[i], h)
		}
	}
	wantBody := "{\n  \"name\": \"Bob\"\n}"
	if second.Body != wantBody {
		t.Errorf("second.Body = %q, want %q", second.Body, wantBody)
	}
}

func TestParse_NameDirectiveOverridesDelimiterName(t *testing.T) {
	src := `### ignored title
# @name getUser
GET {{baseUrl}}/users/1
`
	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(f.Requests))
	}
	if f.Requests[0].Name != "getUser" {
		t.Errorf("Name = %q, want %q", f.Requests[0].Name, "getUser")
	}
}

func TestParse_NameDirectiveWithLineCommentStyle(t *testing.T) {
	src := "### \n// @name listUsers\nGET {{baseUrl}}/users\n"

	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Requests[0].Name != "listUsers" {
		t.Errorf("Name = %q, want %q", f.Requests[0].Name, "listUsers")
	}
}

func TestParse_FileScopedVariables(t *testing.T) {
	src := `@baseUrl = https://example.com
@token = secret123

###
GET {{baseUrl}}/ping
`
	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Vars["baseUrl"] != "https://example.com" {
		t.Errorf("Vars[baseUrl] = %q", f.Vars["baseUrl"])
	}
	if f.Vars["token"] != "secret123" {
		t.Errorf("Vars[token] = %q", f.Vars["token"])
	}
	if len(f.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(f.Requests))
	}
}

func TestParse_CommentLinesIgnored(t *testing.T) {
	src := `# this is a plain comment
// so is this
###
# another comment before the request line
GET https://example.com/api
`
	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(f.Requests))
	}
	if f.Requests[0].Method != "GET" {
		t.Errorf("Method = %q, want GET", f.Requests[0].Method)
	}
}

func TestParse_HttpVersionSuffixIgnored(t *testing.T) {
	src := "GET https://example.com/api HTTP/1.1\n"

	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Requests[0].URL != "https://example.com/api" {
		t.Errorf("URL = %q, want https://example.com/api", f.Requests[0].URL)
	}
}

func TestParse_MissingMethodLineIsError(t *testing.T) {
	src := "### broken\nContent-Type: application/json\n"

	_, err := Parse([]byte(src))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	perr, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected *ParseError, got %T: %v", err, err)
	}
	if perr.Line != 2 {
		t.Errorf("Line = %d, want 2", perr.Line)
	}
}

func TestParse_EmptyFileHasNoRequests(t *testing.T) {
	f, err := Parse([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.Requests) != 0 {
		t.Errorf("expected 0 requests, got %d", len(f.Requests))
	}
}

func TestParse_UnknownMethodIsError(t *testing.T) {
	src := "FROBNICATE https://example.com/api\n"

	_, err := Parse([]byte(src))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
