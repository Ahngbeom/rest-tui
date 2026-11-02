# rest-tui UX P3: IntelliJ 호환 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** IntelliJ에서 쓰던 `.http` 파일을 고치지 않고 열었을 때 블록이 사라지거나 잘못된 본문이 성공으로 전송되지 않게 한다(spec P3-1 ~ P3-8).

**Architecture:** 문법 처리는 UI와 분리된 두 패키지에서 끝낸다. `internal/httpfile`이 메서드 생략, 여러 줄 쿼리, 헤더 주석, 응답 핸들러 제거를 맡고, 제거한 항목은 `Request.Unsupported`에 남긴다. `internal/env`의 `ResolveRequest`가 `ResolveOptions{BaseDir, Now, Rand}`를 받아 동적 변수, `< path` 본문, 스킴 보정을 처리한다. `internal/tui`는 그 결과를 표시하고, 전송을 막고, 전송 뒤 동적 값을 다시 만든다.

**Tech Stack:** Go 1.26(`go.mod` `go 1.26.4`), `math/rand/v2`, Bubble Tea/Bubbles/Lipgloss, 표준 `testing`.

**Spec:** `docs/superpowers/specs/2026-10-30-ux-improvements-design.md` (P3 절, "단계 간 계약", "공통 규칙", "범위 밖")

**전제:** P1(`docs/superpowers/plans/2026-10-30-ux-p1-state-layout.md`)과 P2(`…-ux-p2-env-variables.md`)가 머지된 상태에서 시작한다. 이 계획은 앞 단계를 spec "단계 간 계약"의 이름과, 아래에 밝힌 앞 단계 계획의 이름으로 참조한다.

- P1: `requestModel.id`, `execResultMsg.modelID`, `requestModel.statusLine() string`, `assertViewFits(t, view, width, height)`(`internal/tui/layout_test.go`), `output.Sanitize(s string) string`
- P1(P1 계획이 만드는 테스트 헬퍼): `layoutScenarios`·`TestApp_ViewFitsTerminal`, `newLayoutApp(t, root, store, width, height) App`, `applyMsg(a App, msg tea.Msg) App`, `pressKeys(a App, keys ...string) App`
- P2(계약): `newRequestModel(filePath, root string, file *httpfile.File, req httpfile.Request, store *history.Store, preferredEnv string) requestModel`, `requestModel.canSend() bool`(`len(m.missingVars) == 0`)
- P2(P2 계획이 만드는 이름, 계약 밖): `requestModel.blockedMessage() string`(P2-6 문구 `cannot send: unresolved {{token}} — e switch env · i edit`), `requestModel.edited bool`(P2-7), `warnTextStyle`(`internal/tui/styles.go`, P2-5), 테스트 헬퍼 `applyTestEdit(t, m, text) requestModel`(`requestview_test.go`), `applyEdit`의 `_, m.missingVars = env.ResolveRequest(m.resolved, nil)` 호출(P2 Task 6). 실행 전에 `grep -n 'blockedMessage\|edited\|warnTextStyle\|applyTestEdit\|ResolveRequest' internal/tui/*.go`로 이름을 확인하고, 다르면 실제 이름을 쓴다.

**근거(JetBrains 문서, 2026-10-30 확인):**
- `exploring-http-syntax.html`: "For GET requests, you can omit the request method", "Indent all query string lines but the first one", "start any line with `//` or `#` to make it a comment line", 본문 파일은 "`<` symbol followed by the path", 핸들러는 `>` 뒤 `{% … %}` 또는 JS 파일 경로, "`>>` to create a new file … `>>!` to rewrite the file". pre-request script `< {% … %}`는 요청 줄 **앞**에 온다.
- `http-client-variables.html`: `$uuid`/`$random.uuid`는 UUID-v4, `$timestamp`는 UNIX timestamp, `$isoTimestamp`는 UTC ISO-8601, `$randomInt`는 "between 0 and 1000", `$random.integer(from, to)`는 "`from` (inclusive) and `to` (exclusive)", `$random.alphanumeric`는 "letters, digits, and underscores", 값은 "each time you run a request" 생성한다.
- UX 리뷰 재현: CAP17 `parse warning: line 27: expected HTTP method and URL, got "{{host}}/users"; line 31: … got "?page=1"; line 37: … got "# X-Debug: 1"`(8개 블록 중 5개만 표시). 서버 에코 `"X-Request-Id": "{{$uuid}}"`, `"body": "< ./body.json"`, JSON 뒤에 `> {% client.global.set… %}`가 붙어 201로 표시. CAP36 `unsupported protocol scheme "localhost"`. 원인 위치: `internal/env/substitute.go:8`(정규식이 `$`를 받지 않음), `internal/httpfile/parser.go:122-125`(헤더 뒤 전부를 본문으로), `internal/executor/executor.go:36`(URL 정규화 없음).

## Global Constraints

- 각 태스크는 실패하는 테스트로 시작한다. 마칠 때 `go test ./...`, `go vet ./...`, `gofmt -l .`(출력 없음)이 모두 통과해야 한다.
- 레이아웃이 바뀌는 태스크는 `assertViewFits`로 60x20, 80x24, 120x35에서 검증한다.
- 커밋 메시지는 영어이고 `<area>: <summary>` 형식이다(`httpfile: …`, `env: …`, `tui: …`, `docs: …`).
- 사용자에게 보이는 UI 문구는 영어다. 계획이 고정한 문구는 글자 그대로 쓴다.
- JS는 실행하지 않는다. 응답 핸들러·출력 리디렉션은 본문에서 빼고 경고만 한다. 전송은 허용한다.
- 동적 변수 테스트는 고정 `Now`와 시드를 고정한 `rand.New(rand.NewPCG(1, 2))`로 결정적이어야 한다. 테스트에서 `time.Now()`나 시드 없는 난수에 기대지 않는다.
- 새 외부 의존성을 추가하지 않는다(UUID도 `math/rand/v2`로 만든다).
- 범위 밖: JS 핸들러 실행, `NO_COLOR` 본문 적용, charset 변환, Windows 경로 차이.
- 코드 주석은 지시하지 않는다. 조용히 깨지는 것이 있을 때만 한 줄을 쓴다.

## Review Focus

1. XML·HTML 본문의 `<root>`, `<?xml …?>` 줄은 파일 참조가 아니다. `<` 다음에 공백이 와야 `< path`로 본다. 본문은 그대로 보내고 `missing file`을 만들지 않는다. (Task 4 `TestResolveRequest_XMLBodyLinesAreNotIncludes`)
2. 동적 변수가 있는 요청을 보낸 뒤 `y`로 복사하면, 다시 생성한 값이 아니라 실제로 보낸 요청이 복사돼야 한다. (Task 7 `TestRequestModel_CopyTextUsesSentRequestAfterRegenerate`)
3. 요청 줄 앞의 pre-request script(`< {% … %}`, `< ./pre.js`)가 있어도 블록이 파싱 오류로 사라지지 않는다. `pre-request script`로 경고하고 요청은 목록에 남는다. (Task 2 `TestParse_PreRequestScriptIsRemovedAndRecorded`)
4. `< ./file`로 읽은 파일에 터미널 제어 시퀀스나 바이너리가 있어도 Request view 미리보기가 터미널을 오염시키지 않는다. 보내는 본문은 원본 그대로다. (Task 6 `TestRequestModel_PreviewSanitizesIncludedFileContent`)
5. 메서드 생략을 받아들이더라도 오타 한 단어(`hello`), 붙여 쓴 헤더(`Content-Type:application/json`), 메서드만 있는 줄(`GET`)은 GET 요청이 되지 않고 지금처럼 파싱 오류로 남는다. (Task 1 `TestParse_NonURLRequestLineIsStillError`)

---

## 파일 구조

| 파일 | 변경 | 책임 |
|---|---|---|
| `internal/httpfile/types.go` | Modify | `Request.Unsupported []string` 추가 |
| `internal/httpfile/parser.go` | Modify | 메서드 생략·여러 줄 쿼리·헤더 주석(Task 1), 핸들러·리디렉션·pre-request script 제거(Task 2) |
| `internal/httpfile/parser_test.go` | Modify | Task 1·2 테스트 |
| `internal/env/substitute.go` | Modify | `$` 이름과 `(…)` 인자를 받는 정규식, lookup 함수 기반 치환 |
| `internal/env/dynamic.go` | Create | 동적 변수 값 생성 |
| `internal/env/dynamic_test.go` | Create | 동적 변수 표 테스트 |
| `internal/env/resolve.go` | Modify | `ResolveOptions`, 새 `ResolveRequest`, `< path` 포함, `EnsureScheme` |
| `internal/env/resolve_test.go` | Modify | 기존 호출 갱신 + Task 3·4·5 테스트 |
| `internal/tui/requestview.go` | Modify | `ResolveOptions` 전달, 경고·차단 문구, 전송 후 재생성, 복사 내용 |
| `internal/tui/requestview_test.go` | Modify | Task 3·5·6·7 테스트 |
| `README.md` | Modify | IntelliJ 지원표, env 탐색 규칙 |

---

### Task 1: 요청 줄·헤더 구간 문법 [P3-1, P3-2, P3-3]

**Files:**
- Modify: `internal/httpfile/parser.go` (`parseBlock` 68-129, `parseRequestLine` 172-184)
- Test: `internal/httpfile/parser_test.go`

**Interfaces:**
- Consumes: 없음
- Produces: `parseRequestLine(line string) (method, url string, ok bool)`(시그니처 유지, 메서드 생략 허용), `looksLikeURL(s string) bool`(패키지 내부). 메서드가 없으면 `Request.Method == "GET"`.

- [ ] **Step 1: 실패하는 테스트 작성** (`internal/httpfile/parser_test.go` 끝)

```go
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
```

`reflect`를 import에 추가한다.

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/httpfile/ -run 'TestParse_(Methodless|NonURL|IndentedQuery|UnindentedQuery|HeaderSectionComments)'`
Expected: FAIL. 메서드 생략 줄은 `expected HTTP method and URL`, `?page=1`·`# X-Debug: 1`은 `expected header "Name: value"` 오류. `TestParse_NonURLRequestLineIsStillError`, `TestParse_UnindentedQueryLineIsStillHeaderError`는 지금도 PASS다(회귀 방지용).

- [ ] **Step 3: 구현** (`internal/httpfile/parser.go`)

- `parseRequestLine`: 필드가 2개 이상이고 첫 필드가 `knownMethods`면 지금과 같다. 그렇지 않고 필드가 1개이거나, 2개이면서 둘째가 `HTTP/`로 시작(대소문자 무시)하면, 첫 필드가 `looksLikeURL`일 때 `("GET", fields[0], true)`.
- `looksLikeURL(s string) bool`: 다음 중 하나면 true. (a) `^[A-Za-z][A-Za-z0-9+.-]*://` 접두, (b) `{{`로 시작, (c) `^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*(:[0-9]+)?([/?]\S*)?$`에 맞고 `.`·`:`·`/` 중 하나를 포함. 이 규칙이 `hello`, `Content-Type:application/json`을 걸러낸다.
- 요청 줄 다음: 원본 줄이 공백/탭으로 시작하고 trim한 값이 `?`나 `&`로 시작하는 동안 `req.URL += trimmed`.
- 헤더 루프: trim한 줄에 `stripCommentPrefix`가 ok면 건너뛴다(빈 줄 판정보다 뒤, `parseHeaderLine`보다 앞).

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/httpfile/`
Expected: PASS(기존 `TestParse_UnknownMethodIsError`, `TestParse_MissingMethodLineIsError` 포함).

- [ ] **Step 5: 전체 검증 후 커밋**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: 모든 패키지 `ok`, gofmt 출력 없음.

```bash
git add internal/httpfile/parser.go internal/httpfile/parser_test.go
git commit -m "httpfile: accept method-less request lines, indented query lines, and header comments"
```

---

### Task 2: 응답 핸들러·출력 리디렉션·pre-request script 제거 [P3-4 파서]

**Files:**
- Modify: `internal/httpfile/types.go` (`Request` 13-21)
- Modify: `internal/httpfile/parser.go` (`parseBlock`)
- Test: `internal/httpfile/parser_test.go`

**Interfaces:**
- Consumes: Task 1의 `parseBlock` 구조
- Produces:
  - `httpfile.Request.Unsupported []string` (spec 계약 이름). 요청 안에서 처음 나온 순서, 중복 없음.
  - 항목 문구(고정): `response handler script`(`> {% … %}`), `response handler script: <path>`(`> <path>`), `output redirect: <path>`(`>> <path>`, `>>! <path>`), `pre-request script`(`< {% … %}`), `pre-request script: <path>`(요청 줄 앞 `< <path>`).
  - `splitResponseHandlers(lines []string) (body []string, unsupported []string)` (패키지 내부)

- [ ] **Step 1: 실패하는 테스트 작성**

```go
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
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/httpfile/ -run 'TestParse_(ResponseHandler|HandlerFile|HandlerDirectly|UnclosedHandler|IndentedGreaterThan|PreRequestScript)'`
Expected: 컴파일 실패 `req.Unsupported undefined`. 필드를 먼저 추가하면 Body에 `> {%`가 남거나 `expected header`/`expected HTTP method and URL` 오류로 FAIL. `TestParse_IndentedGreaterThanInBodyIsKept`만 회귀 방지용으로 PASS.

- [ ] **Step 3: 구현**

- `types.go`: `Request`에 `Unsupported []string` 추가.
- 요청 줄 앞 루프: trim한 줄이 `< {%`로 시작하면 trim한 줄에 `%}`가 나오는 줄까지 소비하고 `pre-request script`를 기록한다. 같은 줄에 `%}`가 있으면 그 줄로 끝난다. trim한 줄이 `<` + 공백으로 시작하면 `pre-request script: <나머지 trim>`. 기록은 지역 slice에 모았다가 요청을 만들 때 붙인다.
- 헤더 루프: 원본 줄이 `>`로 시작(0열)하면 헤더를 끝내고(빈 줄처럼 `i++`하지 않음) 본문 단계로 넘긴다. `splitResponseHandlers`와 같은 0열 기준이다. 들여 쓴 `>` 줄은 헤더 구간에서 지금처럼 `expected header` 오류다.
- 본문: `body, unsupported := splitResponseHandlers(block[i:])`, `req.Body = strings.TrimRight(strings.Join(body, "\n"), "\n \t")`.
- `splitResponseHandlers`: 원본 줄이 `>`로 시작(0열)하는 첫 줄부터 핸들러 구간이다. 그 전까지가 본문이다. 구간 안에서는 `>>!`·`>>` → `output redirect: <path>`, `> {%` → `%}`가 나오는 줄까지(닫히지 않으면 블록 끝까지) 소비하고 `response handler script`, 그 밖의 `>` → `response handler script: <path>`로 기록한다. 빈 줄과 나머지 줄은 버린다.
- 중복 기록은 건너뛴다(첫 순서 유지).

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/httpfile/`
Expected: PASS

- [ ] **Step 5: 전체 검증 후 커밋**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: 모두 통과, gofmt 출력 없음.

```bash
git add internal/httpfile/types.go internal/httpfile/parser.go internal/httpfile/parser_test.go
git commit -m "httpfile: strip response handlers and output redirects into Request.Unsupported"
```

---

### Task 3: `ResolveOptions`와 동적 변수 [P3-6 resolver]

**Files:**
- Create: `internal/env/dynamic.go`, `internal/env/dynamic_test.go`
- Modify: `internal/env/substitute.go` (`placeholderRE` 8, `Substitute` 14-35)
- Modify: `internal/env/resolve.go` (`ResolveRequest` 13-42)
- Modify: `internal/env/resolve_test.go` (기존 두 호출에 `ResolveOptions{}` 추가), `internal/env/envfile_test.go`
- Modify: `internal/tui/requestview.go` (`requestModel` 필드, 세 생성자, `recompute`와 `applyEdit`(P2 Task 6)의 `env.ResolveRequest` 호출)
- Test: `internal/tui/requestview_test.go`

**Interfaces:**
- Consumes: Task 2의 `Request.Unsupported`
- Produces:
  - `type ResolveOptions struct { BaseDir string; Now time.Time; Rand *rand.Rand }` (`rand` = `math/rand/v2`). `Now.IsZero()`면 `time.Now()`, `Rand == nil`이면 `rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))`를 쓴다.
  - `func ResolveRequest(req httpfile.Request, vars map[string]string, opts ResolveOptions) (httpfile.Request, []string)`. 결과 `Unsupported`는 `req.Unsupported`의 복사본이다(`nil`이면 `nil`).
  - `func dynamicValue(expr string, now time.Time, r *rand.Rand) (string, bool)` (패키지 내부, `expr`은 `$` 포함)
  - `requestModel.now func() time.Time`, `requestModel.rng *rand.Rand`, `func (m requestModel) resolveOptions() env.ResolveOptions`(`env.ResolveOptions{BaseDir: <filePath가 있으면 filepath.Dir(filePath), 없으면 "">, Now: m.now(), Rand: m.rng}`). `recompute`와 `applyEdit`가 이것을 넘긴다.
  - `applyEdit`(P2 Task 6이 만든 `_, m.missingVars = env.ResolveRequest(m.resolved, nil)`): 시그니처가 바뀌어 그대로 두면 컴파일되지 않는다. 결과를 버리고 `ResolveOptions{}`만 더하면 편집에 남은 `{{$uuid}}`가 해결된 것으로 집계되면서 글자 그대로 전송된다. 그래서 `m.resolved, m.missingVars = env.ResolveRequest(<편집 결과 Request>, nil, m.resolveOptions())`로 바꾼다. vars가 nil이므로 일반 `{{name}}`은 지금처럼 글자 그대로 남아 전송을 막고(P2-6), 동적 변수는 미리보기에 보이는 값으로 생성된다(P3-6). `<편집 결과 Request>`는 `parsed`의 `Method`·`URL`·`Headers`·`Body`·`Unsupported`다.

동적 변수 값 규칙(테스트가 고정):

| 이름 | 값 |
|---|---|
| `$uuid`, `$random.uuid` | 소문자 UUID v4 (`r`에서 16바이트, version 4·variant 10 비트 설정) |
| `$timestamp` | `now.Unix()` 10진수 |
| `$isoTimestamp` | `now.UTC().Format(time.RFC3339)` |
| `$randomInt` | 0 이상 1000 이하 |
| `$random.integer(min, max)` | `min` 이상 `max` 미만(JetBrains 문서). 인자는 정수, 공백 허용, `min >= max`면 해석 실패 |
| `$random.alphabetic(n)` | `[A-Za-z]` n자 |
| `$random.alphanumeric(n)` | `[A-Za-z0-9_]` n자(JetBrains 문서의 underscore 포함) |
| `$random.hexadecimal(n)` | `[0-9a-f]` n자 |
| `$random.email` | `[a-z0-9]` 8자 + `@example.com` |

`n`은 0 이상 4096 이하. 인자 개수는 이름마다 고정이다. `$uuid`·`$random.uuid`·`$timestamp`·`$isoTimestamp`·`$randomInt`·`$random.email`은 괄호가 붙으면(`$uuid(1)`, `$uuid()`) 실패, `(n)` 세 개는 정수 1개, `$random.integer`는 정수 2개이고 괄호가 없으면 실패다. 범위를 벗어나거나 형식이 틀리면 `ok=false`이고, 그 이름은 해결되지 않은 변수가 된다. 각 출현마다 새로 생성하고, 생성 순서는 URL → 헤더(순서대로) → 본문이다.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/env/dynamic_test.go` (import: `math/rand/v2`, `regexp`, `strconv`, `testing`, `time`):

```go
var uuidV4RE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func testNow() time.Time { return time.Date(2026, 10, 30, 18, 0, 0, 0, time.FixedZone("KST", 9*3600)) }

func TestDynamicValue_Formats(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	cases := []struct {
		expr string
		ok   func(string) bool
	}{
		{"$uuid", uuidV4RE.MatchString},
		{"$random.uuid", uuidV4RE.MatchString},
		{"$timestamp", func(s string) bool { return s == "1793350800" }},
		{"$isoTimestamp", func(s string) bool { return s == "2026-10-30T09:00:00Z" }},
		{"$random.alphabetic(12)", regexp.MustCompile(`^[A-Za-z]{12}$`).MatchString},
		{"$random.alphanumeric(12)", regexp.MustCompile(`^[A-Za-z0-9_]{12}$`).MatchString},
		{"$random.hexadecimal(12)", regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString},
		{"$random.alphabetic(0)", func(s string) bool { return s == "" }},
		{"$random.email", regexp.MustCompile(`^[a-z0-9]{8}@example\.com$`).MatchString},
	}
	for _, c := range cases {
		got, ok := dynamicValue(c.expr, testNow(), r)
		if !ok || !c.ok(got) {
			t.Errorf("dynamicValue(%q) = %q, %v", c.expr, got, ok)
		}
	}
}

func TestDynamicValue_IntegerBounds(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 500; i++ {
		s, ok := dynamicValue("$random.integer(5,8)", testNow(), r)
		n, _ := strconv.Atoi(s)
		if !ok || n < 5 || n >= 8 {
			t.Fatalf("$random.integer(5,8) = %q, %v; want 5 <= n < 8", s, ok)
		}
		s, ok = dynamicValue("$randomInt", testNow(), r)
		n, _ = strconv.Atoi(s)
		if !ok || n < 0 || n > 1000 {
			t.Fatalf("$randomInt = %q, %v; want 0..1000", s, ok)
		}
	}
	if s, ok := dynamicValue("$random.integer(-3, -2)", testNow(), r); !ok || s != "-3" {
		t.Errorf("$random.integer(-3, -2) = %q, %v; want -3", s, ok)
	}
}

func TestDynamicValue_RejectsUnknownAndMalformed(t *testing.T) {
	for _, expr := range []string{"$foo", "$env.HOME", "$random.float(1, 2)", "$random.integer(8, 5)", "$random.integer(a, b)", "$random.integer(1)", "$random.integer", "$random.alphabetic(-1)", "$random.alphabetic(4097)", "$random.alphabetic", "$uuid(1)", "$uuid()"} {
		if got, ok := dynamicValue(expr, testNow(), rand.New(rand.NewPCG(1, 2))); ok {
			t.Errorf("dynamicValue(%q) = %q, true; want ok=false", expr, got)
		}
	}
}
```

`internal/env/resolve_test.go` (기존 두 테스트의 호출을 `ResolveRequest(req, vars, ResolveOptions{})`로 바꾸고 아래를 추가. import에 `math/rand/v2`, `net/url`, `strings`를 더한다):

```go
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
```

`internal/env/envfile_test.go`의 `Substitute` 테스트 옆에(같은 파일 끝):

```go
func TestSubstitute_DollarNameIsReportedMissing(t *testing.T) {
	got, missing := Substitute("id={{$uuid}}", nil)
	if got != "id={{$uuid}}" || !reflect.DeepEqual(missing, []string{"$uuid"}) {
		t.Errorf("Substitute = %q, %v", got, missing)
	}
}
```

`internal/tui/requestview_test.go`:

```go
var uuidV4Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewRequestModel_ResolvesDynamicVariableInPreview(t *testing.T) {
	dir := t.TempDir()
	req := httpfile.Request{Method: "GET", URL: "https://example.com", Headers: []httpfile.Header{{Name: "X-Request-Id", Value: "{{$uuid}}"}}}
	m := newRequestModel(filepath.Join(dir, "a.http"), dir, &httpfile.File{}, req, newTestHistoryStore(t), "")

	if len(m.missingVars) != 0 || !uuidV4Pattern.MatchString(m.resolved.Headers[0].Value) {
		t.Errorf("missingVars = %v, header = %q", m.missingVars, m.resolved.Headers[0].Value)
	}
	if !m.canSend() {
		t.Error("canSend() = false, want true")
	}
}

func TestRequestModel_ApplyEditResolvesDynamicVariables(t *testing.T) {
	m := newRequestModel("", "", &httpfile.File{}, httpfile.Request{Method: "GET", URL: "https://example.com"}, newTestHistoryStore(t), "").SetSize(120, 30)
	m = applyTestEdit(t, m, "GET https://example.com\nX-Request-Id: {{$uuid}}\nAuthorization: Bearer {{token}}\n")

	if got := m.resolved.Headers[0].Value; !uuidV4Pattern.MatchString(got) {
		t.Errorf("X-Request-Id = %q, want the generated UUID that will be sent", got)
	}
	if m.resolved.Headers[1].Value != "Bearer {{token}}" || !reflect.DeepEqual(m.missingVars, []string{"token"}) {
		t.Errorf("Authorization = %q, missingVars = %v; a plain leftover placeholder must stay literal and block", m.resolved.Headers[1].Value, m.missingVars)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/env/ ./internal/tui/ -run 'TestDynamicValue|TestResolveRequest|TestSubstitute_DollarName|TestNewRequestModel_ResolvesDynamicVariableInPreview|TestRequestModel_ApplyEditResolvesDynamicVariables'`
Expected: `internal/env`는 컴파일 실패(`undefined: dynamicValue`, `undefined: ResolveOptions`, `too many arguments in call to ResolveRequest`). `internal/tui`는 컴파일되고 `TestNewRequestModel_ResolvesDynamicVariableInPreview`와 `TestRequestModel_ApplyEditResolvesDynamicVariables`가 FAIL한다(지금 정규식이 `$`를 받지 않아 header가 `{{$uuid}}` 그대로).

- [ ] **Step 3: 구현**

- `substitute.go`: `placeholderRE`를 `` `\{\{\s*(\$?[A-Za-z_][A-Za-z0-9_.-]*(?:\([^(){}]*\))?)\s*\}\}` ``로 바꾼다. `func substituteFunc(text string, lookup func(name string) (string, bool)) (string, []string)`를 만들고 `Substitute`는 `vars` 조회를 넘기는 래퍼로 둔다. 반환 규칙(정렬, 중복 제거, 미해결 자리는 원문 유지)은 그대로다.
- `dynamic.go`: `dynamicValue`를 위 표대로 구현한다. 인자 파싱은 `name(args)`를 `strings.Cut`으로 나누고 `strconv.Atoi(strings.TrimSpace(…))`로 읽는다.
- `resolve.go`: `ResolveOptions`를 정의하고 `ResolveRequest`가 `opts`의 기본값을 채운 뒤, `$`로 시작하는 이름은 `dynamicValue`로, 나머지는 `vars`로 조회하는 lookup을 `substituteFunc`에 넘긴다. `vars`에 `$foo`가 있어도 쓰지 않는다. `Unsupported`는 복사한다.
- `requestview.go`: `requestModel`에 `now func() time.Time`, `rng *rand.Rand` 필드를 추가한다. `newRequestModel`, `newRequestModelFromEntry`, `newRequestModelFromEntryNoSend` 모두 `now: time.Now`, `rng: rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))`로 채운다. `resolveOptions()`를 만들고 `recompute`의 호출을 그것으로 바꾼다. `applyEdit`는 Interfaces의 규칙대로 결과까지 `m.resolved`에 담는다. 함수 위 주석의 `missingVars` 설명도 이에 맞게 고친다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/env/ ./internal/tui/`
Expected: PASS

- [ ] **Step 5: 전체 검증 후 커밋**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: 모두 통과, gofmt 출력 없음.

```bash
git add internal/env/ internal/tui/requestview.go internal/tui/requestview_test.go
git commit -m "env: resolve IntelliJ dynamic variables through ResolveOptions"
```

---

### Task 4: `< path` 본문 파일 포함 [P3-5 resolver]

**Files:**
- Modify: `internal/env/resolve.go`
- Test: `internal/env/resolve_test.go`

**Interfaces:**
- Consumes: Task 3의 `ResolveRequest(req, vars, opts)`, `ResolveOptions.BaseDir`
- Produces:
  - `const MissingFilePrefix = "missing file: "` (`internal/env`). 반환 목록의 파일 항목은 `MissingFilePrefix + <치환 후 경로, 쓴 그대로>`이다.
  - 반환 목록 순서: 정렬한 변수 이름이 먼저, 그 뒤에 본문에 나온 순서의 `missing file: …`(중복 제거).

규칙: 변수 치환을 마친 본문에서, 왼쪽 공백을 뗀 줄이 `<` + 공백(스페이스·탭)으로 시작하면 나머지를 trim한 값을 경로로 본다. 절대 경로는 그대로, 상대 경로는 `filepath.Join(opts.BaseDir, path)`로 읽는다. 읽으면 그 줄을 파일 내용으로 바꾸고, 끝의 `\n`(또는 `\r\n`) 하나를 뗀다. 포함한 내용은 변수 치환을 하지 않는다. 읽지 못하면(없음, 디렉터리, 권한) 줄을 그대로 두고 `missing file: <path>`를 기록한다. `opts.BaseDir == ""`이면 포함 처리를 하지 않는다. 히스토리에서 연 요청(`filePath == ""`)의 본문은 이미 포함을 마친 값이라, 다시 읽으면 프로세스 cwd 기준으로 엉뚱한 파일을 읽는다.

- [ ] **Step 1: 실패하는 테스트 작성**

파일 생성은 `envfile_test.go`에 이미 있는 `writeFile(t, dir, name, content)`를 쓴다. import에 `path/filepath`를 더한다.

```go
func TestResolveRequest_IncludesBodyFileRelativeToBaseDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "body.json", "{\"a\": 1}\n")
	req := httpfile.Request{Method: "POST", URL: "https://example.com", Body: "< ./body.json"}
	got, missing := ResolveRequest(req, nil, testResolveOptions(dir))
	if got.Body != `{"a": 1}` || len(missing) != 0 {
		t.Errorf("Body = %q, missing = %v", got.Body, missing)
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
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/env/ -run 'TestResolveRequest_(Includes|IncludePath|AbsoluteInclude|MissingFile|XMLBody|EmptyBaseDir)'`
Expected: FAIL. Body가 `< ./body.json` 그대로이고 missing이 비어 있음. `TestResolveRequest_XMLBodyLinesAreNotIncludes`, `TestResolveRequest_EmptyBaseDirSkipsIncludes`는 회귀 방지용으로 PASS.

- [ ] **Step 3: 구현** — `resolve.go`에 `func includeFiles(body, baseDir string) (string, []string)`를 두고 `ResolveRequest`가 본문 치환 직후 호출한다. 반환 목록 조립은 Interfaces의 순서 규칙을 따른다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/env/`
Expected: PASS

- [ ] **Step 5: 전체 검증 후 커밋**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: 모두 통과.

```bash
git add internal/env/resolve.go internal/env/resolve_test.go
git commit -m "env: inline < path request bodies relative to the .http file"
```

---

### Task 5: 스킴 없는 URL에 `http://` [P3-7]

**Files:**
- Modify: `internal/env/resolve.go`
- Test: `internal/env/resolve_test.go`, `internal/tui/requestview_test.go`

**Interfaces:**
- Consumes: Task 3의 `ResolveRequest`
- Produces: `func EnsureScheme(rawURL string) string` (`internal/env`). `rawURL`이 비었거나, `^[A-Za-z][A-Za-z0-9+.-]*://`로 시작하거나, `{{`를 포함하면 그대로 돌려준다. 그 밖에는 `"http://" + rawURL`. `ResolveRequest`는 URL 치환 뒤에 적용한다.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
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
```

`internal/tui/requestview_test.go`:

```go
func TestRequestModel_ApplyEditAddsHTTPScheme(t *testing.T) {
	m := newRequestModel("", "", &httpfile.File{}, httpfile.Request{Method: "GET", URL: "https://example.com"}, newTestHistoryStore(t), "")
	m, _ = m.startEditing()
	m.editor.SetValue("GET localhost:8080/x\n")
	m = m.applyEdit()
	if m.editing || m.resolved.URL != "http://localhost:8080/x" {
		t.Errorf("editing = %v, URL = %q", m.editing, m.resolved.URL)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/env/ ./internal/tui/ -run 'TestEnsureScheme|TestResolveRequest_AddsScheme|TestRequestModel_ApplyEditAddsHTTPScheme'`
Expected: `internal/env`는 컴파일 실패 `undefined: EnsureScheme`. `internal/tui`는 컴파일되고 `TestRequestModel_ApplyEditAddsHTTPScheme`이 FAIL한다(URL이 `localhost:8080/x` 그대로).

- [ ] **Step 3: 구현** — `EnsureScheme`를 추가하고, `ResolveRequest`가 URL에 적용한다. `applyEdit`는 Task 3부터 `ResolveRequest`를 거치므로(`Unsupported`도 그때 옮긴다) 따로 `EnsureScheme`를 부르지 않는다. `TestRequestModel_ApplyEditAddsHTTPScheme`이 이 경로를 확인한다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/env/ ./internal/tui/`
Expected: PASS

- [ ] **Step 5: 전체 검증 후 커밋**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: 모두 통과.

```bash
git add internal/env/resolve.go internal/env/resolve_test.go internal/tui/requestview_test.go
git commit -m "env: default scheme-less request URLs to http://"
```

---

### Task 6: Request view 경고와 전송 차단 문구 [P3-4 UI, P3-5 UI]

**Files:**
- Modify: `internal/tui/requestview.go` (`buildContent`, `blockedMessage`)
- Test: `internal/tui/requestview_test.go`, `internal/tui/layout_test.go`(P1이 만든 파일에 테스트 추가)

**Interfaces:**
- Consumes: Task 2 `Request.Unsupported`, Task 4 `env.MissingFilePrefix`, P1 `statusLine()`·`layoutScenarios`·`newLayoutApp`·`applyMsg`·`pressKeys`·`output.Sanitize`, P2 `canSend()`·`blockedMessage()`·`warnTextStyle`
- Produces:
  - `func describeUnresolved(missing []string) string`: 변수 이름은 `unresolved {{a}}, {{b}}`, `MissingFilePrefix`로 시작하는 항목은 그대로, 두 묶음이 모두 있으면 `; `로 잇는다.
  - `func unsupportedLine(items []string) string`: 비었으면 `""`, 아니면 `unsupported (ignored): ` + 중복을 뺀 항목을 `, `로 이은 값.
  - `blockedMessage()`는 `"cannot send: " + describeUnresolved(m.missingVars) + " — e switch env · i edit"`가 된다. `m.missingVars`를 화면에 그리는 다른 곳이 남아 있으면(`grep -n missingVars internal/tui/requestview.go`) 그것도 `describeUnresolved`로 바꾼다.

표시 규칙: `unsupportedLine(m.resolved.Unsupported)`가 비어 있지 않으면 `buildContent` head의 첫 줄(요청 줄 위)에 `warnTextStyle`로 둔다. 요청 본문 미리보기는 `output.Sanitize(m.resolved.Body)`를 `PrettyBody`에 넘긴다. 보내는 `m.resolved.Body`는 바꾸지 않는다.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
func TestDescribeUnresolved(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"token"}, "unresolved {{token}}"},
		{[]string{"a", "b"}, "unresolved {{a}}, {{b}}"},
		{[]string{"$foo"}, "unresolved {{$foo}}"},
		{[]string{"missing file: ./body.json"}, "missing file: ./body.json"},
		{[]string{"token", "missing file: ./body.json"}, "unresolved {{token}}; missing file: ./body.json"},
	}
	for _, c := range cases {
		if got := describeUnresolved(c.in); got != c.want {
			t.Errorf("describeUnresolved(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRequestModel_ShowsUnsupportedWarningAndStillSends(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "https://example.com",
		Unsupported: []string{"response handler script", "output redirect: ./out.json", "response handler script"}}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "")
	m = m.SetSize(120, 30)

	content, _ := m.buildContent()
	plain := ansiEscapePattern.ReplaceAllString(content, "")
	if !strings.HasPrefix(plain, "unsupported (ignored): response handler script, output redirect: ./out.json\n") {
		t.Errorf("content does not start with the unsupported warning:\n%s", plain)
	}
	if !m.canSend() {
		t.Fatal("canSend() = false; unsupported items must not block sending")
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil {
		t.Error("Enter returned no command")
	}
}

func TestRequestModel_IncludesBodyFileRelativeToHTTPFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "body.json"), []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	req := httpfile.Request{Method: "POST", URL: "https://example.com", Body: "< ./body.json"}
	m := newRequestModel(filepath.Join(dir, "a.http"), dir, &httpfile.File{}, req, newTestHistoryStore(t), "")
	if m.resolved.Body != `{"a":1}` || !m.canSend() {
		t.Errorf("Body = %q, canSend = %v", m.resolved.Body, m.canSend())
	}
}

func TestRequestModel_MissingBodyFileBlocksSend(t *testing.T) {
	dir := t.TempDir()
	req := httpfile.Request{Method: "POST", URL: "https://example.com", Body: "< ./body.json"}
	m := newRequestModel(filepath.Join(dir, "a.http"), dir, &httpfile.File{}, req, newTestHistoryStore(t), "")
	m = m.SetSize(120, 30)

	if m.canSend() {
		t.Fatal("canSend() = true with a missing body file")
	}
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.executing {
		t.Error("Enter must not send when the body file is missing")
	}
	status := ansiEscapePattern.ReplaceAllString(m.statusLine(), "")
	if !strings.Contains(status, "cannot send: missing file: ./body.json — e switch env · i edit") {
		t.Errorf("statusLine() = %q", status)
	}
}

func TestRequestModel_PreviewSanitizesIncludedFileContent(t *testing.T) {
	dir := t.TempDir()
	raw := "ok\x1b]0;pwned\x07\x1b[2J\x00end"
	if err := os.WriteFile(filepath.Join(dir, "blob.bin"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	req := httpfile.Request{Method: "POST", URL: "https://example.com", Body: "< ./blob.bin"}
	m := newRequestModel(filepath.Join(dir, "a.http"), dir, &httpfile.File{}, req, newTestHistoryStore(t), "")

	if m.resolved.Body != raw {
		t.Errorf("resolved.Body = %q, the sent body must stay byte-for-byte", m.resolved.Body)
	}
	content, _ := m.buildContent()
	for _, bad := range []string{"\x1b]0;", "\x1b[2J", "\x00"} {
		if strings.Contains(content, bad) {
			t.Errorf("preview contains %q", bad)
		}
	}
}
```

`internal/tui/layout_test.go`의 `layoutScenarios`(P1)에 행을 추가한다. `TestApp_ViewFitsTerminal`이 60x20·80x24·120x35·200x50에서 `assertViewFits`로 검사한다.

```go
	{"request-intellij-warnings", func(t *testing.T, w, h int) App {
		root := t.TempDir()
		a := newLayoutApp(t, root, newTestHistoryStore(t), w, h)
		req := httpfile.Request{Method: "POST", URL: "{{host}}/very/long/path/segment/that/wraps", Body: "< ./missing-body-file.json",
			Unsupported: []string{"pre-request script", "response handler script", "response handler script: ./check.js", "output redirect: ./out.json"}}
		a = applyMsg(a, openRequestMsg{filePath: filepath.Join(root, "a.http"), file: &httpfile.File{}, req: req})
		return pressKeys(a, "enter")
	}},
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestDescribeUnresolved|TestRequestModel_(ShowsUnsupported|IncludesBodyFile|MissingBodyFile|PreviewSanitizes)|TestApp_ViewFitsTerminal/request-intellij-warnings'`
Expected: 컴파일 실패 `undefined: describeUnresolved`. 그 뒤로는 경고 줄 없음, 상태 줄 `cannot send: unresolved {{missing file: ./body.json}}`, 미리보기에 `\x1b[2J` 포함으로 FAIL. `TestRequestModel_IncludesBodyFileRelativeToHTTPFile`은 Task 4로 이미 PASS할 수 있다(연결 확인용).

- [ ] **Step 3: 구현** — Interfaces의 두 함수와 표시 규칙을 `requestview.go`에 적용한다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: 전체 검증 후 커밋**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: 모두 통과.

```bash
git add internal/tui/
git commit -m "tui: warn about ignored IntelliJ handlers and block sends on missing body files"
```

---

### Task 7: 보이는 값을 보내고, 전송 후 다시 생성 [P3-6 UI]

**Files:**
- Modify: `internal/tui/requestview.go` (`Update`의 `execResultMsg` 분기, `copyText`)
- Test: `internal/tui/requestview_test.go`

**Interfaces:**
- Consumes: Task 3 `requestModel.now`·`rng`·`recompute`, P1 `execResultMsg.modelID`·`requestModel.id`, P2 `requestModel.edited`
- Produces:
  - `execResultMsg`(자기 `modelID`)를 받으면 `lastEntry`를 저장한 뒤, `!m.edited`(P2-7)이면 `m.recompute()`로 동적 값을 다시 만든다. `SetSize`와 화면 갱신은 값을 다시 만들지 않는다.
  - `copyText()`: `!m.executing && m.lastEntry != nil`이면 요청 부분을 `lastEntry`의 `Method`·`URL`·`RequestHeaders`·`RequestBody`로 만든다(응답과 짝인 보낸 요청). 그 밖에는(전송 중 포함) `m.resolved`. 전송 중에는 `m.resolved`가 지금 보내는 요청이고 `lastEntry`는 이전 전송이다.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
func dynamicTestModel(t *testing.T, url string) requestModel {
	t.Helper()
	dir := t.TempDir()
	req := httpfile.Request{Method: "GET", URL: url, Headers: []httpfile.Header{{Name: "X-Request-Id", Value: "{{$uuid}}"}}}
	m := newRequestModel(filepath.Join(dir, "a.http"), dir, &httpfile.File{}, req, newTestHistoryStore(t), "")
	m.now = func() time.Time { return time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC) }
	m.rng = rand.New(rand.NewPCG(1, 2))
	m.recompute()
	return m.SetSize(120, 30)
}

func TestRequestModel_SendsDisplayedDynamicValue(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("X-Request-Id")
	}))
	defer srv.Close()

	m := dynamicTestModel(t, srv.URL)
	shown := m.resolved.Headers[0].Value
	content, _ := m.buildContent()
	if !strings.Contains(ansiEscapePattern.ReplaceAllString(content, ""), "X-Request-Id: "+shown) {
		t.Fatalf("preview does not show %q", shown)
	}
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	res := cmd().(execResultMsg)
	if sent := <-got; sent != shown {
		t.Errorf("sent X-Request-Id = %q, preview showed %q", sent, shown)
	}
	if res.modelID != m.id {
		t.Errorf("modelID = %d, want %d", res.modelID, m.id)
	}
}

func TestRequestModel_RegeneratesDynamicValuesAfterSend(t *testing.T) {
	m := dynamicTestModel(t, "https://example.com")
	shown := m.resolved.Headers[0].Value

	m = m.SetSize(80, 24)
	if m.resolved.Headers[0].Value != shown {
		t.Fatal("SetSize regenerated the dynamic value")
	}
	m.executing = true
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: history.Entry{Method: "GET", URL: "https://example.com", StatusCode: 200,
		RequestHeaders: []httpfile.Header{{Name: "X-Request-Id", Value: shown}}}})
	next := m.resolved.Headers[0].Value
	if next == shown || !uuidV4Pattern.MatchString(next) {
		t.Errorf("after send header = %q, want a fresh UUID different from %q", next, shown)
	}
}

func TestRequestModel_StaleResultDoesNotRegenerate(t *testing.T) {
	m := dynamicTestModel(t, "https://example.com")
	shown := m.resolved.Headers[0].Value
	m, _ = m.Update(execResultMsg{modelID: m.id + 1, entry: history.Entry{StatusCode: 200}})
	if m.resolved.Headers[0].Value != shown {
		t.Error("a result for another Request view regenerated this view's values")
	}
}

func TestRequestModel_KeepsEditedValuesAfterSend(t *testing.T) {
	m := dynamicTestModel(t, "https://example.com")
	m, _ = m.startEditing()
	m.editor.SetValue("GET https://example.com\nX-Request-Id: fixed\n")
	m = m.applyEdit()
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: history.Entry{StatusCode: 200}})
	if m.resolved.Headers[0].Value != "fixed" {
		t.Errorf("header = %q, applied edits must survive a send", m.resolved.Headers[0].Value)
	}
}

func TestRequestModel_CopyTextUsesSentRequestAfterRegenerate(t *testing.T) {
	m := dynamicTestModel(t, "https://example.com")
	shown := m.resolved.Headers[0].Value
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: history.Entry{Method: "GET", URL: "https://example.com", StatusCode: 200,
		RequestHeaders: []httpfile.Header{{Name: "X-Request-Id", Value: shown}}, ResponseBody: `{"ok":true}`}})
	if m.resolved.Headers[0].Value == shown {
		t.Fatal("precondition: the dynamic value was not regenerated after the send")
	}
	got := m.copyText()
	if !strings.Contains(got, "X-Request-Id: "+shown) || strings.Contains(got, m.resolved.Headers[0].Value) {
		t.Errorf("copyText() = %q, want the sent value %q", got, shown)
	}
	m.executing = true
	if got := m.copyText(); !strings.Contains(got, "X-Request-Id: "+m.resolved.Headers[0].Value) {
		t.Errorf("copyText() while resending = %q, want the request being sent", got)
	}
}
```

import에 `math/rand/v2`, `time`을 추가한다.

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestRequestModel_(SendsDisplayed|Regenerates|StaleResult|KeepsEdited|CopyTextUsesSent)'`
Expected: `TestRequestModel_RegeneratesDynamicValuesAfterSend`(값이 그대로)와 `TestRequestModel_CopyTextUsesSentRequestAfterRegenerate`(precondition)가 FAIL. 재생성만 넣으면 후자는 새 UUID가 복사돼 FAIL한다. `SendsDisplayed`, `StaleResult`(P1이 이미 무시), `KeepsEdited`는 회귀 방지용으로 PASS.

- [ ] **Step 3: 구현** — Interfaces의 두 동작(재생성, 복사 내용)을 `requestview.go`에 넣는다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: 전체 검증 후 커밋**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: 모두 통과.

```bash
git add internal/tui/requestview.go internal/tui/requestview_test.go
git commit -m "tui: send previewed dynamic values and regenerate them after each send"
```

---

### Task 8: README IntelliJ 지원표와 env 탐색 규칙 [P3-8]

**Files:**
- Modify: `README.md` (`## Features`의 `{{var}}` 치환 항목, `## Environment configuration` 절(P2 Task 2·4가 첫 문장과 우선순위 문장을 이미 고쳤다), 그 뒤에 `## IntelliJ HTTP Client compatibility` 절 추가)

**Interfaces:**
- Consumes: P2-1·P2-2·P2-3 규칙(spec 원문), Task 1~7의 동작
- Produces: 없음

README 본문은 기존 README처럼 한국어로 쓴다. 문법·변수 이름은 코드 서식이다.

- [ ] **Step 1: 지원표 작성** — 아래 세 열(`기능`, `상태`, `비고`)과 행을 그대로 싣는다. 상태 값은 `지원`, `무시+경고`, `미지원` 셋이다.

| 기능 | 상태 | 비고 |
|---|---|---|
| `{{var}}`, `@name = value`, `# @name` | 지원 | |
| 메서드 생략(`{{host}}/users`) | 지원 | GET으로 보낸다 |
| 들여 쓴 `?…`·`&…` 쿼리 줄 | 지원 | |
| 헤더 구간의 `#`·`//` 주석 | 지원 | |
| `< ./file` 본문(multipart part 포함) | 지원 | `.http` 파일 기준 상대 경로. 파일이 없으면 전송을 막는다 |
| `$uuid`, `$random.uuid`, `$timestamp`, `$isoTimestamp`, `$randomInt`, `$random.integer(min, max)`, `$random.alphabetic(n)`, `$random.alphanumeric(n)`, `$random.hexadecimal(n)`, `$random.email` | 지원 | 미리보기에 보인 값을 보내고, 전송 뒤 새로 만든다 |
| 스킴 없는 URL(`localhost:8080/x`) | 지원 | `http://`를 붙인다 |
| `> {% … %}`, `> handler.js` 응답 핸들러 | 무시+경고 | 본문에서 빼고 `unsupported (ignored): …`를 표시한다. JS는 실행하지 않는다 |
| `>> file`, `>>! file` 출력 리디렉션 | 무시+경고 | |
| `< {% … %}`, `< pre.js` pre-request script | 무시+경고 | |
| 그 밖의 `$` 변수(`$random.float`, `$env.*`, `$processEnv`, `$dotenv`, `$exampleServer` 등) | 미지원 | 해결되지 않은 변수로 보고 전송을 막는다 |
| `# @no-redirect`, `# @timeout` 등 요청 지시어, gRPC·WebSocket·GraphQL 요청 | 미지원 | |

- [ ] **Step 2: env 탐색 규칙 정리** — P2가 `## Environment configuration`에 쓴 첫 문장(가장 가까운 디렉터리 탐색), 우선순위 문장(`$shared` 포함), `$shared` 설명 문장을 아래 세 항목의 목록 하나로 바꾼다. 같은 규칙을 두 번 쓰지 않는다.
  - 탐색: `.http` 파일의 폴더부터 `-dir` 루트까지 위로 올라가며 두 파일 중 하나라도 있는 가장 가까운 폴더 하나를 쓴다. `http-client.env.json`과 `http-client.private.env.json`은 그 폴더에서 짝으로 읽는다. 루트 위로는 올라가지 않는다.
  - 우선순위: 파일 변수 > private[env] > public[env] > private[`$shared`] > public[`$shared`]. `$shared`는 env 목록에 나오지 않는다.
  - 값: 숫자·bool은 문자열로 쓰고, 객체·배열·`null`(`SSLConfiguration` 등)은 건너뛴다.

  `## Features`의 `{{var}}` 치환 항목에 있는 `(우선순위: 파일 변수 > private 환경 > public 환경)` 괄호는 지운다. 우선순위는 Environment configuration 한 곳에만 둔다.

- [ ] **Step 3: 확인**

Run: `grep -n 'IntelliJ HTTP Client compatibility\|무시+경고\|\$shared\|-dir. 루트' README.md`
Expected: 새 절 제목, `무시+경고` 행 3개, `$shared` 규칙이 보인다. `grep -n 'private 환경 > public 환경' README.md`는 출력이 없어야 한다(옛 Features 괄호와 옛 우선순위 문장이 모두 사라졌다는 뜻).

- [ ] **Step 4: 커밋**

```bash
git add README.md
git commit -m "docs: document IntelliJ syntax support and env file lookup rules"
```

---

### Task 9: 실제 바이너리 확인과 dev 바이너리 재빌드 [P3-1 ~ P3-7 수동 검증]

**Files:**
- Modify(재생성): `rest-tui`
- 임시 픽스처: `$SCRATCH/p3`(저장소 밖. 실행 세션의 scratchpad 디렉터리를 `$SCRATCH`로 둔다)

**Interfaces:** 없음

- [ ] **Step 1: 에코 서버와 픽스처 준비**

`$SCRATCH/echo.py`: 모든 메서드에 `{"method", "path", "headers", "body"}` JSON을 돌려주는 `http.server` 기반 서버(`127.0.0.1:18093`에 바인드). `python3 -I $SCRATCH/echo.py`를 백그라운드 작업으로 띄운다.

`$SCRATCH/p3/http-client.env.json`: `{"dev": {"host": "127.0.0.1:18093"}}`
`$SCRATCH/p3/api/body.json`: `{"from": "file"}`
`$SCRATCH/p3/api/intellij.http`:

```http
### methodless
{{host}}/users
    ?page=1
    &size=2
Accept: application/json
# X-Debug: 1

### uuid
POST http://{{host}}/echo
X-Request-Id: {{$uuid}}

{"ts": {{$timestamp}}, "n": {{$random.integer(1, 10)}}}

### withHandler
POST http://{{host}}/echo
Content-Type: application/json

{"a": 1}

> {%
  client.global.set("x", response.body.a);
%}
>> ./out.json

### fromFile
POST http://{{host}}/echo
Content-Type: application/json

< ./body.json

### missingFile
POST http://{{host}}/echo

< ./nope.json

### noScheme
GET localhost:18093/plain

### unknownDollar
GET http://{{host}}/x?v={{$foo}}

### preScript
< {%
  request.variables.set("id", "1");
%}
GET http://{{host}}/pre
```

- [ ] **Step 2: 120x35로 실행**

```bash
go build -o $SCRATCH/rest-tui-p3 .
mkdir -p $SCRATCH/home
/opt/homebrew/bin/tmux -L rest-tui-p3 new-session -d -s p3 -x 120 -y 35 "env HOME=$SCRATCH/home $SCRATCH/rest-tui-p3 -dir $SCRATCH/p3 -config $SCRATCH/history.json"
/opt/homebrew/bin/tmux -L rest-tui-p3 capture-pane -p -t p3
```

`HOME`을 바꿔 사용자의 실제 `~/.config/rest-tui/dirs.json`을 건드리지 않는다(P1·P2·P4의 tmux 확인과 같다). 키는 `/opt/homebrew/bin/tmux -L rest-tui-p3 send-keys -t p3 <key>`로 보내고, 매번 `capture-pane -p`로 확인한다.

| 확인 | 화면에 보여야 하는 문구 |
|---|---|
| 파일 열기 | Requests에 8개 블록 모두. `parse warning` 없음 |
| methodless 열기 → Enter | `GET http://127.0.0.1:18093/users?page=1&size=2`. 응답 에코 `"path": "/users?page=1&size=2"`, headers에 `X-Debug` 없음 |
| uuid 열기 | `X-Request-Id: ` 뒤 UUID. 이 값을 적어 둔다 |
| uuid Enter | 에코 `"X-Request-Id": "<적어 둔 값>"`. `up`을 눌러 위로 올리면 미리보기의 `X-Request-Id`가 다른 UUID |
| withHandler 열기 | 첫 줄 `unsupported (ignored): response handler script, output redirect: ./out.json`. Enter 뒤 에코 `"body": "{\"a\": 1}"`에 `> {%` 없음 |
| fromFile Enter | 미리보기 본문 `"from": "file"`, 에코 body도 같음 |
| missingFile 열기 → Enter | 상태 줄 `cannot send: missing file: ./nope.json — e switch env · i edit`, 전송 안 됨 |
| noScheme 열기 → Enter | `GET http://localhost:18093/plain`, 200 |
| unknownDollar 열기 | 상태 줄 `cannot send: unresolved {{$foo}} — e switch env · i edit` |
| preScript 열기 → Enter | 첫 줄 `unsupported (ignored): pre-request script`. `GET http://127.0.0.1:18093/pre`, 200 |

- [ ] **Step 3: 80x24로 다시 확인**

Run: `/opt/homebrew/bin/tmux -L rest-tui-p3 resize-window -t p3 -x 80 -y 24` 뒤 withHandler와 missingFile 화면을 다시 캡처한다.
Expected: 경고 줄이 패널 폭으로 줄바꿈되고, breadcrumb(1행)과 footer(마지막 행)가 모두 보이며 화면을 넘지 않는다.

- [ ] **Step 4: 정리**

Run: `/opt/homebrew/bin/tmux -L rest-tui-p3 kill-server; pkill -f "$SCRATCH/echo.py"`
`git status --short`에 픽스처·`history.json`·`out.json`이 없어야 한다(모두 `$SCRATCH` 아래).

- [ ] **Step 5: dev 바이너리 재빌드 후 별도 커밋**

Run: `scripts/build.sh`
Expected: `go vet ./...` 통과 후 `[build.sh] rest-tui updated`.

```bash
git status --short --untracked-files=no   # 추적 파일 중 rest-tui 만 변경
git add rest-tui
git commit -m "build: rebuild committed dev binary"
```

---

## 요구사항 → 태스크 추적표

| 요구사항 | 태스크 | 핵심 테스트/확인 |
|---|---|---|
| P3-1 메서드 생략 → GET | Task 1, Task 9 | `TestParse_MethodlessRequestLineDefaultsToGET`, `TestParse_NonURLRequestLineIsStillError` |
| P3-2 들여 쓴 `?`·`&` 줄 | Task 1, Task 9 | `TestParse_IndentedQueryLinesAppendToURL` |
| P3-3 헤더 구간 주석 | Task 1, Task 9 | `TestParse_HeaderSectionCommentsSkipped` |
| P3-4 핸들러·리디렉션 제거, `Request.Unsupported`, 경고, 전송 허용 | Task 2, Task 6, Task 9 | `TestParse_ResponseHandlerScriptRemovedFromBody`, `TestParse_HandlerFileAndOutputRedirectsRecorded`, `TestRequestModel_ShowsUnsupportedWarningAndStillSends` |
| P3-5 `< path` 포함, 없으면 전송 차단 + `missing file: …` | Task 4, Task 6, Task 9 | `TestResolveRequest_IncludesFileInsideMultipartPart`, `TestResolveRequest_MissingFileIsReportedAfterVariables`, `TestResolveRequest_EmptyBaseDirSkipsIncludes`, `TestRequestModel_MissingBodyFileBlocksSend` |
| P3-6 동적 변수, 보이는 값 전송, 전송 후 재생성, 알 수 없는 `$`는 미해결 | Task 3, Task 7, Task 9 | `TestDynamicValue_Formats`, `TestResolveRequest_DynamicVariablesAreDeterministic`, `TestResolveRequest_UnknownDollarVariableIsMissing`, `TestRequestModel_SendsDisplayedDynamicValue`, `TestRequestModel_RegeneratesDynamicValuesAfterSend`, `TestRequestModel_CopyTextUsesSentRequestAfterRegenerate` |
| P3-7 스킴 없는 URL에 `http://` | Task 5, Task 9 | `TestEnsureScheme`, `TestResolveRequest_AddsSchemeAfterSubstitution`, `TestRequestModel_ApplyEditAddsHTTPScheme` |
| P3-8 README 지원표·env 탐색 규칙 | Task 8 | grep 확인 |
| 계약 `httpfile.Request.Unsupported` | Task 2(파서), Task 3(resolver 복사, 편집 경로) | `TestResolveRequest_PreservesUnsupported` |
| 계약 `env.ResolveRequest(req, vars, opts)`, `ResolveOptions{BaseDir, Now, Rand}` | Task 3, Task 4 | 기존 tui 호출부(`recompute`, P2의 `applyEdit`) 갱신 포함, `TestRequestModel_ApplyEditResolvesDynamicVariables` |
| 공통 규칙: 레이아웃 검증 60x20·80x24·120x35 | Task 6 | `TestApp_ViewFitsTerminal/request-intellij-warnings` |
| 공통 규칙: tmux 확인 + `build: rebuild committed dev binary` | Task 9 | |
