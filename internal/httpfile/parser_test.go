package httpfile

import (
	"reflect"
	"strings"
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

func TestParse_OneBadBlockDoesNotHideOthers(t *testing.T) {
	src := `### Get user
GET {{baseUrl}}/users/1

### Broken
Content-Type: application/json

### Create user
POST {{baseUrl}}/users
`
	f, err := Parse([]byte(src))
	if err == nil {
		t.Fatal("expected error for the broken block, got nil")
	}
	if len(f.ParseErrors) != 1 {
		t.Fatalf("expected 1 ParseErrors, got %d: %v", len(f.ParseErrors), f.ParseErrors)
	}
	if len(f.Requests) != 2 {
		t.Fatalf("expected 2 valid requests despite the broken block, got %d", len(f.Requests))
	}
	if f.Requests[0].Name != "Get user" || f.Requests[1].Name != "Create user" {
		t.Errorf("requests = %+v", f.Requests)
	}
}

func TestParse_MethodlessRequestLineDefaultsToGET(t *testing.T) {
	cases := []struct{ src, wantURL string }{
		{"{{host}}/users\n", "{{host}}/users"},
		{"https://example.com/a HTTP/1.1\n", "https://example.com/a"},
		{"localhost:8080/x\n", "localhost:8080/x"},
		{"example.com/api\n", "example.com/api"},
	}
	for _, c := range cases {
		f, err := Parse([]byte(c.src))
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.src, err)
		}
		if got := f.Requests[0]; got.Method != "GET" || got.URL != c.wantURL {
			t.Errorf("Parse(%q) = %s %s, want GET %s", c.src, got.Method, got.URL, c.wantURL)
		}
	}
}

func TestParse_NonURLRequestLineIsStillError(t *testing.T) {
	for _, src := range []string{"hello\n", "Content-Type:application/json\n", "GET\n"} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("Parse(%q) err = nil, want a parse error", src)
		}
	}
}

func TestParse_IndentedQueryLinesAppendToURL(t *testing.T) {
	src := "GET https://example.com/api\n    ?page=1\n\t&size=20\nAccept: application/json\n"
	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := f.Requests[0]
	if req.URL != "https://example.com/api?page=1&size=20" {
		t.Errorf("URL = %q", req.URL)
	}
	if len(req.Headers) != 1 || req.Headers[0].Name != "Accept" {
		t.Errorf("Headers = %v", req.Headers)
	}
	if req.Line != 1 {
		t.Errorf("Line = %d, want 1", req.Line)
	}
}

func TestParse_UnindentedQueryLineIsStillHeaderError(t *testing.T) {
	if _, err := Parse([]byte("GET https://example.com/api\n?page=1\n")); err == nil {
		t.Fatal("expected parse error for an unindented ?-line")
	}
}

func TestParse_HeaderSectionCommentsSkipped(t *testing.T) {
	src := "GET https://example.com\nAccept: a\n# X-Debug: 1\n// X-Other: 2\nX-Real: 3\n"
	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Header{{Name: "Accept", Value: "a"}, {Name: "X-Real", Value: "3"}}
	if !reflect.DeepEqual(f.Requests[0].Headers, want) {
		t.Errorf("Headers = %v, want %v", f.Requests[0].Headers, want)
	}
}

func TestParse_ResponseHandlerScriptRemovedFromBody(t *testing.T) {
	src := "POST https://example.com\nContent-Type: application/json\n\n{\"a\": 1}\n\n> {%\n  client.global.set(\"x\", response.body.x);\n%}\n"
	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := f.Requests[0]
	if req.Body != `{"a": 1}` {
		t.Errorf("Body = %q", req.Body)
	}
	if want := []string{"response handler script"}; !reflect.DeepEqual(req.Unsupported, want) {
		t.Errorf("Unsupported = %v, want %v", req.Unsupported, want)
	}
}

func TestParse_HandlerFileAndOutputRedirectsRecorded(t *testing.T) {
	src := "GET https://example.com\n\n> {% client.test(\"ok\", function() {}); %}\n>> ./out.json\n>>! ./out2.json\n> ./check.js\n> {% more(); %}\n"
	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := f.Requests[0]
	if req.Body != "" {
		t.Errorf("Body = %q, want empty", req.Body)
	}
	want := []string{"response handler script", "output redirect: ./out.json", "output redirect: ./out2.json", "response handler script: ./check.js"}
	if !reflect.DeepEqual(req.Unsupported, want) {
		t.Errorf("Unsupported = %v, want %v", req.Unsupported, want)
	}
}

func TestParse_HandlerDirectlyAfterRequestLine(t *testing.T) {
	f, err := Parse([]byte("GET https://example.com\n> {%\n  client.log(1);\n%}\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req := f.Requests[0]; len(req.Headers) != 0 || req.Body != "" || len(req.Unsupported) != 1 {
		t.Errorf("request = %+v", req)
	}
}

func TestParse_UnclosedHandlerConsumesRestOfBlockOnly(t *testing.T) {
	src := "### a\nGET https://a.example\n\n> {%\n  client.log(1);\n\n### b\nGET https://b.example\n"
	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.Requests) != 2 || f.Requests[0].Body != "" || f.Requests[1].URL != "https://b.example" {
		t.Errorf("requests = %+v", f.Requests)
	}
}

func TestParse_IndentedGreaterThanInBodyIsKept(t *testing.T) {
	f, err := Parse([]byte("POST https://example.com\n\nline1\n  > quoted\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req := f.Requests[0]; req.Body != "line1\n  > quoted" || len(req.Unsupported) != 0 {
		t.Errorf("Body = %q, Unsupported = %v", req.Body, req.Unsupported)
	}
}

func TestParse_PreRequestScriptIsRemovedAndRecorded(t *testing.T) {
	src := "### with script\n< {%\n  request.variables.set(\"id\", \"1\");\n%}\n< ./pre.js\nGET https://example.com/{{id}}\n"
	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := f.Requests[0]
	if req.Name != "with script" || req.URL != "https://example.com/{{id}}" || req.Line != 6 {
		t.Errorf("request = %+v", req)
	}
	if want := []string{"pre-request script", "pre-request script: ./pre.js"}; !reflect.DeepEqual(req.Unsupported, want) {
		t.Errorf("Unsupported = %v, want %v", req.Unsupported, want)
	}
}

func TestParse_StripsResponseHistoryLinks(t *testing.T) {
	cases := map[string]string{
		"after body":    "POST https://example.com\n\n{\"a\":1}\n\n<> 2024-05-01T120000.200.json\n",
		"after handler": "POST https://example.com\n\n{\"a\":1}\n\n> {% client.log(1); %}\n\n<> x.json\n",
		"no body":       "GET https://example.com\n\n<> 2024-05-01T120000.200.json\n<> 2024-05-02T120000.200.json\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			f, err := Parse([]byte(src))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			req := f.Requests[0]
			wantBody := `{"a":1}`
			if name == "no body" {
				wantBody = ""
			}
			if req.Body != wantBody {
				t.Errorf("Body = %q, want %q", req.Body, wantBody)
			}
			wantUnsupported := 0
			if name == "after handler" {
				wantUnsupported = 1
			}
			if len(req.Unsupported) != wantUnsupported {
				t.Errorf("Unsupported = %v", req.Unsupported)
			}
		})
	}
}

func TestParse_IndentedHistoryLinkInBodyIsKept(t *testing.T) {
	f, _ := Parse([]byte("POST https://example.com\n\nx\n  <> y.json\n"))
	if got := f.Requests[0].Body; got != "x\n  <> y.json" {
		t.Errorf("Body = %q", got)
	}
}

func TestParse_HistoryLinkInHeaderSectionIsSkipped(t *testing.T) {
	cases := map[string]struct {
		src     string
		headers int
	}{
		"under request line": {"GET https://e\n<> 2024.json\n", 0},
		"under header":       {"GET https://e\nAccept: a\n<> x.json\n", 1},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f, err := Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(f.Requests) != 1 {
				t.Fatalf("Requests = %d, want 1", len(f.Requests))
			}
			req := f.Requests[0]
			if len(req.Headers) != tc.headers || req.Body != "" {
				t.Errorf("Headers = %+v, Body = %q", req.Headers, req.Body)
			}
		})
	}
}

func TestParse_UnclosedPreRequestScriptIsError(t *testing.T) {
	f, err := Parse([]byte("### a\n< {%\n  request.variables.set(\"id\", \"1\");\nGET https://example.com\n"))
	if err == nil || !strings.Contains(err.Error(), "unclosed pre-request script") {
		t.Fatalf("err = %v, want unclosed pre-request script", err)
	}
	if len(f.Requests) != 0 || len(f.ParseErrors) != 1 || f.ParseErrors[0].Line != 2 {
		t.Errorf("Requests = %+v, ParseErrors = %v", f.Requests, f.ParseErrors)
	}
}

func TestParseVerbatimBody_KeepsHandlerAndIncludeLinesInBody(t *testing.T) {
	src := "POST https://example.com\nX-A: 1\n\nintro\n> quoted\n<> x.json\n< ./other\n< {% s %}\ntail\n"
	f, err := ParseVerbatimBody([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := f.Requests[0]
	if want := "intro\n> quoted\n<> x.json\n< ./other\n< {% s %}\ntail"; req.Body != want {
		t.Errorf("Body = %q, want %q", req.Body, want)
	}
	if len(req.Unsupported) != 0 || len(req.Headers) != 1 {
		t.Errorf("Unsupported = %v, Headers = %+v", req.Unsupported, req.Headers)
	}
}
