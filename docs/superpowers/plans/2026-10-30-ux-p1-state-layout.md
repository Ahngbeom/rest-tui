# rest-tui UX P1: 화면 상태·레이아웃·출력 안전 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 다른 요청의 응답이 붙는 문제, 커서 이탈, 도움말 뒤로 새는 키 입력, 터미널을 넘는 레이아웃, 응답의 터미널 제어 시퀀스를 고쳐 spec P1-1~P1-9를 모두 충족한다.

**Architecture:** 출력 정화는 UI와 무관한 `internal/output`에 두고(`Sanitize`, `IsBinary`), `RenderResponse`가 헤더·본문을 정화한 뒤에만 앱이 색을 입힌다. TUI는 높이·폭 예산을 한곳에서 관리한다. App이 header·status·footer 행을 뺀 `contentHeight()`를 계산해 모든 sub-model에 내려 주고, 각 패널은 `lipgloss` frame 크기(`GetHorizontalFrameSize` 등)로 안쪽 폭을 구한다. footer는 `footer.go`의 `renderHints` 하나로 그린다. 레이아웃 회귀는 `layout_test.go`의 `assertViewFits`와 시나리오×크기 표 하나로 막는다.

**Tech Stack:** Go 1.26.4, Bubble Tea v1.3.10, bubbles v1.0.0(list, viewport, textarea), lipgloss v1.1.0, charmbracelet/x/ansi v0.11.6, 표준 `testing`.

**Spec:** `docs/superpowers/specs/2026-10-30-ux-improvements-design.md` (P1 표, "단계 간 계약"의 P1 행, "공통 규칙")

## Global Constraints

- 각 태스크를 마칠 때 `go test ./...`, `go vet ./...`가 통과하고 `gofmt -l .`의 출력이 없어야 한다.
- 레이아웃이 바뀌는 태스크는 `assertViewFits`로 60x20, 80x24, 120x35에서 검증한다. P1-4 표 테스트는 200x50도 포함한다.
- 단계 간 계약 이름을 정확히 이렇게 만든다: `requestModel.id uint64`, `execResultMsg.modelID uint64`, `nextRequestModelID()`, `func assertViewFits(t *testing.T, view string, width, height int)`(`internal/tui/layout_test.go`), `type hint struct{ key, desc string }`, `func renderHints(hints []hint, width int) string`(`internal/tui/footer.go`), `requestModel.statusLine() string`, `output.Sanitize(s string) string`, `output.IsBinary(body []byte, contentType string) bool`.
- 새 의존성을 추가하지 않는다. `go.mod`에 있는 `lipgloss`, `bubbles`, `x/ansi`만 쓴다.
- lipgloss v1.1.0의 `Style.Width(n)`는 padding을 포함하고 border는 뺀 폭이다. `paneStyle`은 border 1 + padding 1이므로 안쪽 폭은 `w - paneStyle.GetHorizontalFrameSize()`(=w-4), 렌더 폭 인자는 `w - paneStyle.GetHorizontalBorderSize()`(=w-2)다. 숫자 4·2를 직접 쓰지 말고 이 getter를 쓴다. 아래 태스크의 `frame`/`vframe`/`border`/`vborder`는 그 패널 스타일의 `GetHorizontalFrameSize`/`GetVerticalFrameSize`/`GetHorizontalBorderSize`/`GetVerticalBorderSize` 값이다.
- 모든 `SetSize`는 계산한 폭·높이가 음수면 0으로 고정한다(`max(0, …)`).
- 커밋 메시지는 영어 `<area>: <summary>`(`tui: …`, `output: …`). 사용자에게 보이는 문구는 영어다.
- 기존 테스트는 계약 때문에 바뀌는 곳만 고친다(Task 2의 `execResultMsg{…}` 리터럴에 `modelID` 추가). 그 밖의 기존 테스트는 그대로 통과해야 한다.
- 실제 시스템 클립보드에 쓰지 않는다. 복사 상태는 `clipboardCopyMsg`를 직접 보내서 시험하고, tmux 확인에서도 `c`/`y`를 누르지 않는다.
- 범위 밖: env 줄 문구(P2-4), 전송 차단 안내(P2-6), 스크롤 %·`g/G`(P4-6), 경로 가운데 생략(리뷰 L9), 마우스, OSC52. 이 단계의 statusLine은 복사 알림과 편집 오류만 담는다.
- 코드 주석은 "조용히 깨지는 것"을 설명하는 한 줄만 허용한다. 이번 변경으로 틀린 말이 되는 기존 주석(예: `requestview.go:177-180`의 `-1` 설명)은 지운다.

## Review Focus

spec이 말하지 않았지만 사용자가 실제로 만날 입력이다. 각 줄의 테스트는 해당 태스크에 들어 있다.

1. 탭으로 들여 쓴 응답(HTML, XML, TSV)도 한 번만 줄바꿈되고, 맨 아래로 스크롤하면 본문 마지막 줄이 보인다. `Sanitize`는 `\t`를 남기는데, `ansi.Hardwrap`은 탭 폭을 0으로 재고 lipgloss는 탭을 공백 4칸으로 바꾼다. 그래서 viewport가 줄을 다시 감고, 본문 끝이 잘려 보이지 않는다(현재 코드에서 확인함). → Task 6 `TestRequestModel_TabIndentedBodyEndStaysReachable`.
2. CRLF 줄끝(`\r\n`) 응답은 `\r`이 지워져 줄이 깨지지 않는다. → Task 1 `TestSanitize_StripsEscapeSequencesAndControls`의 CRLF 경우.
3. 한글처럼 폭이 2인 문자가 긴 응답 본문에 섞여도 폭을 넘지 않는다. → Task 6 시나리오 `request-cjk-response`.
4. History에서 `r`로 다시 보낸 결과는 그 Request view에 정상으로 표시된다(id 필터가 rerun 경로까지 막지 않는다). → Task 2 `TestNewRequestModelFromEntry_SkipsEnvUIAndSendsImmediately` 보강.
5. 아주 작은 터미널(0x0, 20x5)이나 크기 메시지 전 첫 렌더에서도 `View()`가 panic하지 않는다. → Task 5 `TestApp_ViewDoesNotPanicOnTinyTerminal`, Task 9에서 help 경우 추가.

---

### Task 1: 응답 헤더·본문 정화와 바이너리 본문 요약 (P1-9)

**Files:**
- Create: `internal/output/sanitize.go`
- Create: `internal/output/sanitize_test.go`
- Modify: `internal/output/render.go:29-56` (`RenderResponse`, `renderHeaders`)
- Test: `internal/output/render_test.go`

**Interfaces:**
- Consumes: 없음
- Produces: `func Sanitize(s string) string`, `func IsBinary(body []byte, contentType string) bool` (package `output`). `RenderResponse`는 상태 줄·헤더 이름과 값·비JSON 본문에 `Sanitize`를 적용하고, `IsBinary`가 참이면 본문 대신 `(binary body: <N> bytes, <content-type>)`을 넣는다. Content-Type 헤더가 없으면 `<content-type>` 자리에 `unknown`을 쓴다. `RenderTransaction`, History detail, 복사 텍스트는 `RenderResponse`를 거치므로 따로 고치지 않는다.

- [ ] **Step 1: 실패하는 테스트 작성** (`internal/output/sanitize_test.go`)

```go
func TestSanitize_StripsEscapeSequencesAndControls(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"csi and osc title", "hello \x1b[2J\x1b[H\x1b]0;PWNED-TITLE\x07\x1b[41mRED\x1b[0m end", "hello RED end"},
		{"osc8 with ST", "a\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\b", "alinkb"},
		{"c0 except newline and tab", "tab\there\nnl\rcr\x00nul\x08bs", "tab\there\nnlcrnulbs"},
		{"crlf", "a\r\nb", "a\nb"},
		{"c1", "c1:\u009b31mX\u0085Y", "c1:31mXY"},
		{"wide text kept", "한글 \x1b[31m빨강\x1b[0m", "한글 빨강"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		if got := Sanitize(c.in); got != c.want {
			t.Errorf("%s: Sanitize(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestIsBinary(t *testing.T) {
	cases := []struct {
		name        string
		body        []byte
		contentType string
		want        bool
	}{
		{"png bytes", []byte("\x89PNG\r\n\x1a\n\x00\x00"), "image/png", true},
		{"image type alone", []byte("GIF89a"), "image/gif", true},
		{"svg is text", []byte("<svg/>"), "image/svg+xml", false},
		{"octet-stream", []byte("abc"), "application/octet-stream", true},
		{"json", []byte(`{"a":1}`), "application/json; charset=utf-8", false},
		{"nul in text/plain", []byte("a\x00b"), "text/plain", true},
		{"latin1 text/plain", []byte("caf\xe9"), "text/plain; charset=iso-8859-1", false},
		{"no type, invalid utf8", []byte{0xff, 0xfe, 0xfd}, "", true},
		{"no type, utf8 text", []byte("plain text"), "", false},
		{"empty body", nil, "image/png", false},
	}
	for _, c := range cases {
		if got := IsBinary(c.body, c.contentType); got != c.want {
			t.Errorf("%s: IsBinary = %v, want %v", c.name, got, c.want)
		}
	}
}
```

`internal/output/render_test.go` 끝에 추가:

```go
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
		t.Errorf("RenderResponse(JSON with raw C1) = %q, want C1 removed and JSON still colored", got)
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
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/output/ -run 'TestSanitize|TestIsBinary|TestRenderResponse_SanitizesHeadersAndBody|TestRenderResponse_BinaryBodyPlaceholder'`
Expected: FAIL. `undefined: Sanitize`, `undefined: IsBinary` 컴파일 오류.

- [ ] **Step 3: `Sanitize`, `IsBinary` 구현** (`internal/output/sanitize.go`)

`Sanitize`는 `ansi.Strip`(x/ansi, ESC로 시작하는 CSI·OSC·DCS 등을 제거)을 먼저 적용한 뒤, `\n`·`\t`를 뺀 U+0000–U+001F와 U+0080–U+009F rune을 지운다.
`IsBinary` 규칙: 본문이 비면 false. `mime.ParseMediaType`로 얻은 타입이 `image/svg+xml`이면 false. `image/*`·`audio/*`·`video/*`·`font/*`·`application/octet-stream`·`application/pdf`·`application/zip`·`application/gzip`이면 true. `text/*`이거나 `json`·`xml`로 끝나면 NUL 바이트가 있을 때만 true. 그 밖(타입 없음 포함)은 NUL이 있거나 `utf8.Valid`가 false면 true.

- [ ] **Step 4: `RenderResponse`에 적용** (`internal/output/render.go`)

상태 줄의 `resp.Status`, `renderHeaders`의 이름과 값에 `Sanitize`를 적용한다. 본문 섹션을 만들기 전에 헤더에서 `Content-Type`을 대소문자 구분 없이 찾아 `IsBinary`를 부른다. 참이면 `fmt.Sprintf("(binary body: %d bytes, %s)", len(body), ct)`로 바꾼다(`ct == ""`이면 `unknown`). 거짓이면 `RenderResponse`가 `renderBody([]byte(Sanitize(string(resp.Body))), opts)`를 부른다. 정화를 색 입히기보다 먼저 해야 한다. 유효한 JSON의 문자열 안에도 C1(U+0080–U+009F) 문자가 날것으로 들어올 수 있고, 위 C1 테스트가 이 경우를 확인한다. `renderBody`와 `PrettyBody`(요청 본문 경로)는 바꾸지 않는다.

- [ ] **Step 5: 통과 확인**

Run: `go test ./internal/output/`
Expected: PASS (기존 10개 테스트 포함)

- [ ] **Step 6: 커밋**

```bash
git add internal/output/sanitize.go internal/output/sanitize_test.go internal/output/render.go internal/output/render_test.go
git commit -m "output: strip terminal control sequences and summarize binary bodies"
```

---

### Task 2: 응답을 보낸 Request view에만 결과 표시 (P1-1)

**Files:**
- Modify: `internal/tui/messages.go:41-44` (`execResultMsg`)
- Modify: `internal/tui/requestview.go:36-65` (struct), `:70-133` (생성자 세 개), `:253-284` (`sendCmd`), `:288-295` (`Update`)
- Test: `internal/tui/requestview_test.go`, `internal/tui/app_test.go`

**Interfaces:**
- Consumes: 없음
- Produces: `requestModel.id uint64`, `execResultMsg.modelID uint64`, `func nextRequestModelID() uint64`(패키지 수준 `sync/atomic.Uint64` 카운터, 첫 값 1, 0은 쓰지 않는다). 테스트 헬퍼 `func applyMsg(a App, msg tea.Msg) App`(`app_test.go`, cmd는 버린다).

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/tui/requestview_test.go`에 추가:

```go
func TestRequestModel_IgnoresExecResultFromAnotherModel(t *testing.T) {
	store := newTestHistoryStore(t)
	a := newRequestModel("", &httpfile.File{}, httpfile.Request{Method: "GET", URL: "https://a.example"}, store)
	b := newRequestModel("", &httpfile.File{}, httpfile.Request{Method: "GET", URL: "https://b.example"}, store)
	if a.id == 0 || a.id == b.id {
		t.Fatalf("ids = %d, %d, want distinct non-zero", a.id, b.id)
	}
	b, cmd := b.Update(execResultMsg{modelID: a.id, entry: history.Entry{StatusCode: 200}})
	if b.lastEntry != nil || cmd != nil {
		t.Errorf("lastEntry = %+v, cmd = %v, want the foreign result ignored", b.lastEntry, cmd)
	}
}
```

`TestNewRequestModelFromEntry_SkipsEnvUIAndSendsImmediately`(현재 `:293`) 끝에 추가:

```go
	if msg.modelID != m.id {
		t.Fatalf("msg.modelID = %d, want %d", msg.modelID, m.id)
	}
	m, _ = m.Update(msg)
	if m.lastEntry == nil || m.lastEntry.StatusCode != http.StatusOK {
		t.Errorf("rerun result not shown: lastEntry = %+v", m.lastEntry)
	}
```

`internal/tui/app_test.go`에 헬퍼와 테스트 추가(`net/http`, `net/http/httptest` import):

```go
func applyMsg(a App, msg tea.Msg) App {
	model, _ := a.Update(msg)
	return model.(App)
}

func TestApp_StaleExecResultDoesNotLandOnNewRequestView(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("slow done")) }))
	defer srv.Close()
	a := newTestApp(t)
	a = applyMsg(a, openRequestMsg{file: &httpfile.File{}, req: httpfile.Request{Method: "GET", URL: srv.URL + "/slow"}})
	model, sendCmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a = model.(App)
	a = applyMsg(a, openRequestMsg{file: &httpfile.File{}, req: httpfile.Request{Method: "GET", URL: srv.URL + "/users"}})

	a = applyMsg(a, sendCmd())

	if a.request.lastEntry != nil {
		t.Errorf("listUsers view shows a response it never sent: %+v", a.request.lastEntry)
	}
	entries, err := a.store.List(10)
	if err != nil || len(entries) != 1 || entries[0].URL != srv.URL+"/slow" {
		t.Errorf("history = %+v, %v, want the /slow send recorded", entries, err)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestRequestModel_IgnoresExecResultFromAnotherModel|TestApp_StaleExecResultDoesNotLandOnNewRequestView|TestNewRequestModelFromEntry_SkipsEnvUIAndSendsImmediately'`
Expected: FAIL. `a.id undefined`, `unknown field modelID` 컴파일 오류.

- [ ] **Step 3: 구현**

`execResultMsg`에 `modelID uint64`, `requestModel`에 `id uint64`를 추가한다. `newRequestModel`, `newRequestModelFromEntry`, `newRequestModelFromEntryNoSend`가 모두 `m.id = nextRequestModelID()`를 부른다. `newRequestModelFromEntry`는 `m.sendCmd()`를 부르기 전에 id를 정해야 한다. `sendCmd`는 클로저 밖에서 `id := m.id`를 잡아 `execResultMsg{modelID: id, …}`로 돌려준다. 히스토리 저장은 지금처럼 클로저 안에서 id와 무관하게 한다. `Update`의 `execResultMsg` 분기는 `msg.modelID != m.id`면 `m, nil`을 그대로 돌려준다.

- [ ] **Step 4: 기존 테스트의 메시지 리터럴 갱신**

`internal/tui/requestview_test.go`의 `execResultMsg{entry: entry}` 8곳(`grep -n 'execResultMsg{entry: entry}'`로 찾는다. Step 1에서 7줄을 삽입했으므로 원래 `:347`, `:366`은 `:354`, `:373`으로 밀려 있다)을 `execResultMsg{modelID: m.id, entry: entry}`로 바꾼다. 다른 줄은 건드리지 않는다.

- [ ] **Step 5: 통과 확인**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 6: 커밋**

```bash
git add internal/tui/messages.go internal/tui/requestview.go internal/tui/requestview_test.go internal/tui/app_test.go
git commit -m "tui: drop exec results that belong to a closed request view"
```

---

### Task 3: 파일을 바꾸면 Requests 선택을 첫 항목으로 초기화 (P1-2)

**Files:**
- Modify: `internal/tui/browser.go:136-148` (`refreshRecent`), `:168-207` (`selectFile`)
- Test: `internal/tui/browser_test.go`

**Interfaces:**
- Consumes: 없음
- Produces: 없음(동작 변경만)

- [ ] **Step 1: 실패하는 테스트 작성**

```go
func TestBrowserModel_SelectFileResetsRequestCursorToFirst(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.http", "### one\nGET https://a.example/1\n\n### two\nGET https://a.example/2\n\n### three\nGET https://a.example/3\n")
	writeTestFile(t, dir, "b.http", "GET https://b.example/only\n")
	m := newBrowserModel(dir, newTestHistoryStore(t)).SetSize(120, 35)
	key := func(m browserModel, k tea.KeyMsg) browserModel { m, _ = m.Update(k); return m }
	j := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}

	m = key(m, tea.KeyMsg{Type: tea.KeyEnter}) // open a.http
	m = key(key(m, j), j)
	if m.requests.Index() != 2 {
		t.Fatalf("precondition: requests.Index() = %d, want 2", m.requests.Index())
	}
	m = key(m, tea.KeyMsg{Type: tea.KeyEsc})
	m = key(m, j)
	m = key(m, tea.KeyMsg{Type: tea.KeyEnter}) // open b.http

	if m.requests.Index() != 0 {
		t.Errorf("requests.Index() = %d, want 0 after switching files", m.requests.Index())
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on the only request did nothing")
	}
	if msg, ok := cmd().(openRequestMsg); !ok || msg.req.URL != "https://b.example/only" {
		t.Errorf("cmd() = %#v, want openRequestMsg for b.example/only", cmd())
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run TestBrowserModel_SelectFileResetsRequestCursorToFirst`
Expected: FAIL. `requests.Index() = 2, want 0` (`SetItems`가 커서를 남겨 둔다)

- [ ] **Step 3: 구현**

`selectFile`과 `refreshRecent`에서 `m.requests.SetItems(…)` 바로 뒤에 `m.requests.Select(0)`을 부른다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/tui/browser.go internal/tui/browser_test.go
git commit -m "tui: reset request selection when switching files"
```

---

### Task 4: footer 힌트를 폭에 맞춰 생략 (P1-7)

**Files:**
- Create: `internal/tui/footer.go`
- Create: `internal/tui/footer_test.go`
- Modify: `internal/tui/app.go:204-224` (`footerHints`), `:276` (footer 렌더)
- Test: `internal/tui/app_test.go`

**Interfaces:**
- Consumes: Task 2의 `applyMsg`
- Produces: `type hint struct{ key, desc string }`, `func renderHints(hints []hint, width int) string`. 한 힌트는 `key + " " + desc`, 힌트 사이는 공백 두 칸이다. `width <= 0`이면 자르지 않는다. 마지막 두 힌트는 항상 남긴다. 그 앞의 힌트는 마지막 두 개에 가장 가까운 것부터 하나씩 빼서 줄 맨 앞의 화면 고유 힌트가 가장 오래 남게 한다. 두 개만 남아도 넘치면 `ansi.Truncate(s, width, "")`로 자른다. `App.footerHints()`는 `[]hint`를 돌려준다.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/tui/footer_test.go`:

```go
func TestRenderHints_DropsMiddleHintsKeepsHelpAndQuit(t *testing.T) {
	hints := []hint{{"e", "cycle env"}, {"enter", "send"}, {"c", "copy"}, {"h", "history"}, {"?", "help"}, {"q", "quit"}}
	cases := []struct {
		width int
		want  string
	}{
		{80, "e cycle env  enter send  c copy  h history  ? help  q quit"},
		{40, "e cycle env  enter send  ? help  q quit"},
		{20, "? help  q quit"},
		{0, "e cycle env  enter send  c copy  h history  ? help  q quit"},
	}
	for _, c := range cases {
		if got := renderHints(hints, c.width); got != c.want {
			t.Errorf("renderHints(width=%d) = %q, want %q", c.width, got, c.want)
		}
	}
	if got := renderHints(hints, 10); lipgloss.Width(got) > 10 {
		t.Errorf("renderHints(width=10) = %q, wider than 10", got)
	}
}
```

`internal/tui/app_test.go`(`github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/x/ansi` import 추가. `footer_test.go`는 `lipgloss` import):

```go
func TestApp_FooterKeepsHelpAndQuitAt80Columns(t *testing.T) {
	a := newTestApp(t) // 80x24
	a = applyMsg(a, openRequestMsg{file: &httpfile.File{}, req: httpfile.Request{Method: "GET", URL: "https://example.com"}})
	lines := strings.Split(a.View(), "\n")
	last := lines[len(lines)-1]
	if w := lipgloss.Width(last); w > 80 {
		t.Errorf("footer width = %d, want <= 80: %q", w, last)
	}
	if plain := ansi.Strip(last); !strings.HasSuffix(strings.TrimRight(plain, " "), "? help  q quit") {
		t.Errorf("footer = %q, want it to end with %q", plain, "? help  q quit")
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestRenderHints_DropsMiddleHintsKeepsHelpAndQuit|TestApp_FooterKeepsHelpAndQuitAt80Columns'`
Expected: FAIL. `undefined: hint`, `undefined: renderHints` 컴파일 오류

- [ ] **Step 3: `footer.go` 구현과 `footerHints` 전환**

Interfaces 블록의 규칙대로 `renderHints`를 만든다. `footerHints`의 각 문자열을 같은 순서의 `[]hint`로 옮긴다(예: Request view는 `{"e","cycle env"}, {"enter/ctrl+r","send"}, {"↑/↓","scroll"}, {"c","copy"}, {"i","edit"}, {"esc","back"}, {"h","history"}, {"d","dirs"}, {"?","help"}, {"q","quit"}`, 편집 중은 `{"ctrl+s","apply"}, {"esc","cancel"}`). `View`는 `footerStyle.Render(renderHints(a.footerHints(), a.width-footerStyle.GetHorizontalPadding()))`로 그린다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/tui/footer.go internal/tui/footer_test.go internal/tui/app.go internal/tui/app_test.go
git commit -m "tui: drop middle footer hints so help and quit stay visible"
```

---

### Task 5: 레이아웃 검사 도구와 App 프레임 높이·폭 예산 (P1-4)

**Files:**
- Create: `internal/tui/layout_test.go`
- Create: `internal/tui/layout.go`
- Modify: `internal/tui/app.go:81-87` (`contentHeight`), `:94-102` (`applyStoreWarning`), `:106-113` (`WindowSizeMsg`), `:259-282` (`View`)

**Interfaces:**
- Consumes: Task 4의 `renderHints`
- Produces:
  - `func assertViewFits(t *testing.T, view string, width, height int)` (`layout_test.go`): `lipgloss.Height(view) <= height`, 각 줄 `lipgloss.Width <= width`. 실패하면 넘친 줄 번호와 `ansi.Strip`한 내용을 보여 준다.
  - `var layoutSizes = []struct{ w, h int }{{60, 20}, {80, 24}, {120, 35}, {200, 50}}`
  - `type layoutScenario struct { name string; build func(t *testing.T, w, h int) App }`, `var layoutScenarios []layoutScenario`, 표 테스트 `TestApp_ViewFitsTerminal`. 뒤 태스크는 이 slice에 행만 추가한다.
  - `func newLayoutApp(t *testing.T, root string, store *history.Store, width, height int) App`(dirStore는 임시 경로, `WindowSizeMsg` 적용), `func pressKeys(a App, ks ...string) App`(`"enter"`, `"esc"`, `"tab"`, `"up"`, `"down"`, `"ctrl+s"`는 해당 `tea.KeyType`, 그 밖은 `KeyRunes`)
  - `func fitLines(s string, width, height int) string` (`layout.go`): `ansi.Wrap(s, width, "")`로 줄바꿈한 뒤 앞에서 `height`줄만 남긴다. `width <= 0` 또는 `height <= 0`(SetSize 전)이면 `s`를 그대로 돌려준다. 이 규칙이 없으면 SetSize 없이 `View()`를 부르는 기존 `TestNewBrowserModel_BadRootSetsRootErrNotParseErr`에서 오류 문구가 사라진다. Task 7, 8이 오류 문구에 쓴다.
  - `func (a App) resize() App`: 네 sub-model에 `SetSize(a.width, a.contentHeight())`를 적용한다. Task 9가 도움말 viewport를 여기에 더한다.
  - `contentHeight()`는 `statusMsg != ""`이면 1행을 더 뺀다.

- [ ] **Step 1: 실패하는 테스트 작성** (`internal/tui/layout_test.go`)

```go
var layoutScenarios = []layoutScenario{
	{"browser-empty", func(t *testing.T, w, h int) App {
		return newLayoutApp(t, t.TempDir(), newTestHistoryStore(t), w, h)
	}},
	{"browser-long-root", func(t *testing.T, w, h int) App {
		root := filepath.Join(t.TempDir(), strings.Repeat("long-directory-name-", 8))
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		return newLayoutApp(t, root, newTestHistoryStore(t), w, h)
	}},
	{"store-warning", func(t *testing.T, w, h int) App {
		histPath := filepath.Join(t.TempDir(), "history.json")
		if err := os.WriteFile(histPath, []byte("{ not valid json"), 0o644); err != nil {
			t.Fatal(err)
		}
		a := pressKeys(newLayoutApp(t, t.TempDir(), history.NewStore(histPath), w, h), "h")
		if a.statusMsg == "" {
			t.Fatal("precondition: expected a store warning")
		}
		return a
	}},
}

func TestApp_ViewFitsTerminal(t *testing.T) {
	for _, sc := range layoutScenarios {
		for _, sz := range layoutSizes {
			t.Run(fmt.Sprintf("%s/%dx%d", sc.name, sz.w, sz.h), func(t *testing.T) {
				view := sc.build(t, sz.w, sz.h).View()
				assertViewFits(t, view, sz.w, sz.h)
				if first := ansi.Strip(strings.SplitN(view, "\n", 2)[0]); !strings.Contains(first, "rest-tui") {
					t.Errorf("header pushed off screen; first line = %q", first)
				}
			})
		}
	}
}

func TestApp_ViewDoesNotPanicOnTinyTerminal(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{0, 0}, {20, 5}} {
		for _, keys := range [][]string{nil, {"h"}, {"d"}} {
			a := pressKeys(newLayoutApp(t, t.TempDir(), newTestHistoryStore(t), sz.w, sz.h), keys...)
			_ = a.View()
			a = applyMsg(a, openRequestMsg{file: &httpfile.File{}, req: httpfile.Request{Method: "GET", URL: "https://example.com"}})
			_ = a.View()
		}
	}
}
```

헬퍼 `assertViewFits`, `newLayoutApp`, `pressKeys`, `layoutSizes`, `layoutScenario`는 Interfaces 블록의 시그니처대로 같은 파일에 둔다.

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestApp_ViewFitsTerminal|TestApp_ViewDoesNotPanicOnTinyTerminal'`
Expected: FAIL. `store-warning/*`은 높이가 1행 넘친다(`statusMsg` 줄이 `contentHeight` 밖, 200x50에서 51행). 백업 경로가 든 경고 줄 때문에 폭도 넘친다(200x50에서 210칸). `browser-long-root/*`는 모든 크기에서 header 폭이 넘친다. `browser-empty`도 60x20·80x24·120x35에서 header 폭이 넘친다. macOS의 `t.TempDir()` 경로가 약 100자이기 때문이다(breadcrumb 127칸). `JoinVertical`이 모든 줄을 header 폭에 맞춰 채우므로 실패 출력에는 모든 줄이 넘친 것으로 나온다. 200x50의 `browser-empty`와 `TestApp_ViewDoesNotPanicOnTinyTerminal`은 지금도 PASS이고 회귀 방지용이다.

- [ ] **Step 3: 구현**

`contentHeight()`가 status 행을 빼게 한다. `resize()`를 만들어 `WindowSizeMsg` 분기가 이것을 쓰게 하고, `applyStoreWarning`은 `statusMsg`가 바뀌었을 때만 `resize()`를 부른다. `View`는 breadcrumb와 statusMsg를 각각 `ansi.Truncate(s, a.width-<style>.GetHorizontalPadding(), "…")`로 한 줄로 자른 뒤 렌더한다. `layout.go`에 `fitLines`를 둔다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/tui/layout.go internal/tui/layout_test.go internal/tui/app.go
git commit -m "tui: budget status and header rows inside the terminal frame"
```

---

### Task 6: Request view 폭을 패널 안쪽 폭과 맞추고 상태 줄 1행 고정 (P1-5, P1-4)

**Files:**
- Modify: `internal/tui/requestview.go:164-184` (`SetSize`), `:420-425` (`envLine`), `:434-452` (`View`)
- Modify: `internal/tui/styles.go:74-80` (`wrapToWidth`)
- Test: `internal/tui/requestview_test.go`, `internal/tui/layout_test.go`

**Interfaces:**
- Consumes: Task 2의 `requestModel.id`, Task 5의 `layoutScenarios`, `newLayoutApp`, `pressKeys`, `applyMsg`
- Produces: `func (m requestModel) statusLine() string`. 줄바꿈 없는 1행이고 폭은 `m.viewport.Width` 이하다. 편집 중이면 `editError`(errorTextStyle), 편집 중이 아니면 `copyStatus`(copied/error 스타일), 둘 다 없으면 `""`. 문구 안의 `\n`은 공백으로 바꾸고 `ansi.Truncate(…, "…")`로 자른다. 뒤 단계(P2-6, P4-6)는 이 함수에 경우만 더한다. `View`는 일반·편집 모드 모두 `envLine` 1행 + 본문(viewport 또는 editor) + `statusLine()` 1행을 그린다. `wrapToWidth`는 줄바꿈 전에 `\t`를 공백 4칸으로 바꾼다. 4는 lipgloss `Render`의 기본 탭 폭이다. 이 변경은 History 상세와 도움말에도 적용된다.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/tui/requestview_test.go`(`github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/x/ansi` import 추가):

```go
func TestRequestModel_ViewportWidthMatchesPaneInnerWidth(t *testing.T) {
	m := newRequestModel("", &httpfile.File{}, httpfile.Request{Method: "GET", URL: "https://example.com"}, newTestHistoryStore(t)).SetSize(60, 20)
	if want := 60 - paneFocusedStyle.GetHorizontalFrameSize(); m.viewport.Width != want {
		t.Errorf("viewport.Width = %d, want %d", m.viewport.Width, want)
	}
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: history.Entry{StatusCode: 200, ResponseBody: strings.Repeat("x", 100)}})
	view := m.View()
	assertViewFits(t, view, 60, 20)
	var runs []int
	for _, r := range regexp.MustCompile(`x{10,}`).FindAllString(ansi.Strip(view), -1) {
		runs = append(runs, len(r))
	}
	if !reflect.DeepEqual(runs, []int{56, 44}) {
		t.Errorf("x runs = %v, want [56 44] (wrapped exactly once)", runs)
	}
}

func TestRequestModel_StatusRowIsReservedForCopyAndEditError(t *testing.T) {
	m := newRequestModel("", &httpfile.File{}, httpfile.Request{Method: "GET", URL: "https://example.com"}, newTestHistoryStore(t)).SetSize(80, 24)
	if h := lipgloss.Height(m.View()); h != 24 {
		t.Fatalf("idle height = %d, want 24", h)
	}
	m, _ = m.Update(clipboardCopyMsg{token: m.copyToken})
	if h := lipgloss.Height(m.View()); h != 24 || !strings.Contains(m.statusLine(), "copied to clipboard") {
		t.Errorf("after copy: height = %d, statusLine = %q", h, m.statusLine())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m.editor.SetValue("GET https://example.com\nbad header line\n")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if h := lipgloss.Height(m.View()); h != 24 || !strings.HasPrefix(ansi.Strip(m.statusLine()), "line 2:") {
		t.Errorf("after edit error: height = %d, statusLine = %q", h, m.statusLine())
	}
}

func TestRequestModel_TabIndentedBodyEndStaysReachable(t *testing.T) {
	m := newRequestModel("", &httpfile.File{}, httpfile.Request{Method: "GET", URL: "https://example.com"}, newTestHistoryStore(t)).SetSize(60, 20)
	body := strings.Repeat("\t\t\t<p>"+strings.Repeat("word ", 9)+"</p>\n", 30) + "\t\tTHE-END"
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: history.Entry{StatusCode: 200, Status: "200 OK", ResponseBody: body}})
	m.viewport.GotoBottom()
	if v := ansi.Strip(m.View()); !strings.Contains(v, "THE-END") {
		t.Errorf("last body line unreachable after scrolling to the bottom:\n%s", v)
	}
}
```

`internal/tui/layout_test.go`의 `layoutScenarios`에 행 추가. 공통 준비: `a := newLayoutApp(t, t.TempDir(), newTestHistoryStore(t), w, h)` 후 `applyMsg(a, openRequestMsg{file: &httpfile.File{}, req: httpfile.Request{Method: "GET", URL: "https://example.com/users"}})`.
- `request-long-response`: `execResultMsg{modelID: a.request.id, entry: history.Entry{StatusCode: 200, ResponseBody: `{"path":"` + strings.Repeat("segment/", 60) + `","text":"` + strings.Repeat("lorem ipsum ", 40) + `"}`}}` 적용
- `request-cjk-response`: 같은 방식으로 `ResponseBody: strings.Repeat("한글 응답 본문 ", 80)`
- `request-copy-status`: `clipboardCopyMsg{token: a.request.copyToken}` 적용
- `request-edit-error`: `pressKeys(a, "i")`, `a.request.editor.SetValue("GET https://example.com\n" + strings.Repeat("bad ", 60) + "\n")`, `pressKeys(a, "ctrl+s")`

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestRequestModel_ViewportWidthMatchesPaneInnerWidth|TestRequestModel_StatusRowIsReservedForCopyAndEditError|TestRequestModel_TabIndentedBodyEndStaysReachable|TestApp_ViewFitsTerminal/request'`
Expected: FAIL. 먼저 `statusLine` 미정의 컴파일 오류가 난다. 정의만 넣으면 다음과 같이 실패한다.
- x 묶음이 `[54 44]`다. 56칸 줄을 lipgloss가 다시 감아 2칸 조각이 생기고, 그 조각은 정규식에 걸리지 않는다.
- 60x20 높이가 21, 복사 뒤 80x24 높이가 25다.
- `THE-END`가 보이지 않는다.
- `request-long-response`·`request-copy-status`·`request-edit-error`가 모든 크기에서 높이를 넘는다.
- `request-cjk-response`는 지금도 PASS이고 회귀 방지용이다.

- [ ] **Step 3: 구현**

`SetSize`: 안쪽 폭 `width - paneFocusedStyle.GetHorizontalFrameSize()`, viewport 높이 `height - paneFocusedStyle.GetVerticalFrameSize() - 2`(env 줄 + 상태 줄). editor는 같은 폭과 같은 높이를 쓴다(`-1` 보정과 그 주석을 지운다. 그 보정은 폭 불일치를 가리던 것이다). `View`는 `paneFocusedStyle.Width(m.width - paneFocusedStyle.GetHorizontalBorderSize())`로 렌더하고, `envLine()` 결과도 `ansi.Truncate(…, m.viewport.Width, "…")`로 자른다. Interfaces 블록대로 `statusLine()`을 만든다. `wrapToWidth`는 맨 앞에서 `s = strings.ReplaceAll(s, "\t", "    ")`를 실행한다. 바꾼 `s`를 `Hardwrap`과 `hyperlinkURLs`에 같이 넘긴다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/tui/`
Expected: PASS (기존 `// innerWidth = 56` 테스트 포함)

- [ ] **Step 5: 커밋**

```bash
git add internal/tui/requestview.go internal/tui/styles.go internal/tui/requestview_test.go internal/tui/layout_test.go
git commit -m "tui: match request viewport to pane width and reserve a status row"
```

---

### Task 7: Browser 40/60 고정 분할과 파싱 경고 3줄 제한 (P1-8, P1-6, P1-4)

**Files:**
- Modify: `internal/tui/browser.go:154-161` (`SetSize`), `:168-207` (`selectFile`), `:264-297` (`View`)
- Test: `internal/tui/browser_test.go`, `internal/tui/layout_test.go`

**Interfaces:**
- Consumes: Task 5의 `fitLines`, `layoutScenarios`, `assertViewFits`
- Produces: `func (m browserModel) parseWarningLines(width int) []string`. 파싱 경고가 없으면 nil이다. 있으면 `"parse warning: " + joinParseErrors(…)`를 `ansi.Wrap(s, width, "")`로 줄바꿈한다. 3줄 이하면 그대로 두고, 넘치면 앞 2줄과 `fmt.Sprintf("… +%d more", total-2)`를 돌려준다(N은 숨긴 줄 수). 스타일은 입히지 않는다.

- [ ] **Step 1: 실패하는 테스트 작성** (`internal/tui/browser_test.go`, `fmt`·`regexp`·`lipgloss`·`x/ansi` import 추가)

```go
func TestBrowserModel_PanesSplitFullWidth40And60(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.http", "GET https://example.com\n")
	for _, w := range []int{60, 80, 120, 200} {
		view := newBrowserModel(dir, newTestHistoryStore(t)).SetSize(w, 20).View()
		lines := strings.Split(view, "\n")
		for i, ln := range lines {
			if lipgloss.Width(ln) != w {
				t.Errorf("w=%d line %d width = %d, want %d", w, i, lipgloss.Width(ln), w)
			}
		}
		top := []rune(ansi.Strip(lines[0]))
		if filesW := w * 40 / 100; len(top) != w || top[filesW-1] != '┓' || top[w-1] != '╮' {
			t.Errorf("w=%d top border = %q, want Files corner at col %d and Requests corner at col %d", w, string(top), filesW, w)
		}
	}
}

func TestBrowserModel_ParseWarningWrapsToAtMostThreeLines(t *testing.T) {
	dir := t.TempDir()
	content := "### good\nGET https://example.com/good\n"
	for i := 0; i < 5; i++ {
		content += fmt.Sprintf("\n### bad%d\nthis is not a request line number %d with extra words\n", i, i)
	}
	writeTestFile(t, dir, "broken.http", content)
	m := newBrowserModel(dir, newTestHistoryStore(t)).SetSize(80, 24)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	inner := 80 - 80*40/100 - paneStyle.GetHorizontalFrameSize()
	lines := m.parseWarningLines(inner)
	if len(lines) != 3 || !regexp.MustCompile(`^… \+\d+ more$`).MatchString(lines[2]) {
		t.Fatalf("parseWarningLines = %q, want 2 lines + \"… +N more\"", lines)
	}
	for _, ln := range lines {
		if lipgloss.Width(ln) > inner {
			t.Errorf("warning line %q wider than %d", ln, inner)
		}
	}
	view := m.View()
	assertViewFits(t, view, 80, 24)
	if !strings.Contains(ansi.Strip(view), "good") {
		t.Error("the valid request disappeared from the Requests pane")
	}
}
```

`layoutScenarios`에 행 추가:
- `browser-parse-warning`: 위 `broken.http` 내용을 root에 쓰고 `pressKeys(a, "enter")`
- `browser-file-parse-error`: 모든 블록이 깨진 파일(`"this is not a request " + strings.Repeat("word ", 40) + "\n"`)을 쓰고 `pressKeys(a, "enter")`
- `browser-scan-error`: root를 존재하지 않는 `filepath.Join(t.TempDir(), strings.Repeat("missing-directory-", 10))`로 준다

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestBrowserModel_PanesSplitFullWidth40And60|TestBrowserModel_ParseWarningWrapsToAtMostThreeLines|TestApp_ViewFitsTerminal/browser'`
Expected: FAIL. 먼저 `parseWarningLines` 미정의 컴파일 오류가 난다. `nil`을 돌려주는 정의만 넣으면 패널 폭이 내용 길이(두 패널 합 38칸)로 줄어 줄 폭이 w보다 작다(`line 0 width = 38, want 60`). 첫 줄 border 검사의 `len(top) != w` 가드는 빼지 않는다. 이 가드가 없으면 120칸에서 `index out of range [47] with length 38` panic이 나서 패키지의 다른 테스트 결과까지 사라진다.

- [ ] **Step 3: 구현**

`SetSize`: `filesW := width * 40 / 100`, `reqW := width - filesW`. Files 목록은 `(filesW - frame, height - vframe)`, Requests 목록은 `(reqW - frame, height - vframe - warnRows)`로 맞춘다. `warnRows`는 `parseWarningLines(reqW - frame)`가 비어 있지 않으면 `len + 1`(빈 구분 줄)이다. `selectFile`은 끝에서 `m.SetSize(m.width, m.height)`를 다시 불러 경고 행을 반영한다. `View`는 두 패널을 `style.Width(paneW - border).Height(height - vborder)`로 렌더한다. rootErr·parseErr 문구는 `fitLines(…, 안쪽 폭, height - vframe)`로 감싼 뒤 `errorTextStyle`을 입힌다. 경고는 `parseWarningLines`의 각 줄에 `errorTextStyle`을 입혀 목록 위에 둔다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/tui/`
Expected: PASS (기존 `TestBrowserModel_ViewUsesThickBorderOnlyForFocusedPane` 포함)

- [ ] **Step 5: 커밋**

```bash
git add internal/tui/browser.go internal/tui/browser_test.go internal/tui/layout_test.go
git commit -m "tui: split browser panes 40/60 and cap parse warnings at three lines"
```

---

### Task 8: History 전체 폭과 상세 화면 상태 줄 예약 (P1-8, P1-4)

**Files:**
- Modify: `internal/tui/historyview.go:138-153` (`SetSize`), `:222-238` (`View`)
- Modify: `internal/tui/dirhistoryview.go:87-92` (`View`의 loadErr)
- Test: `internal/tui/historyview_test.go`, `internal/tui/layout_test.go`

**Interfaces:**
- Consumes: Task 5의 `fitLines`, `layoutScenarios`
- Produces: 없음. History 목록·상세·loadErr는 모두 `paneFocusedStyle.Width(width - border)`로 렌더한다. 상세 viewport 높이는 `height - vframe - 1`이고 마지막 1행은 복사 상태(없으면 빈 줄)에 항상 예약한다.

- [ ] **Step 1: 실패하는 테스트 작성** (`internal/tui/historyview_test.go`, `lipgloss` import 추가)

```go
func TestHistoryModel_ListUsesFullWidth(t *testing.T) {
	store := newTestHistoryStore(t)
	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com", StatusCode: 200}); err != nil {
		t.Fatal(err)
	}
	view := newHistoryModel(store).refresh().SetSize(120, 20).View()
	for i, ln := range strings.Split(view, "\n") {
		if lipgloss.Width(ln) != 120 {
			t.Errorf("line %d width = %d, want 120", i, lipgloss.Width(ln))
		}
	}
}

func TestHistoryModel_DetailCopyStatusKeepsHeight(t *testing.T) {
	store := newTestHistoryStore(t)
	if _, err := store.Append(history.Entry{Method: "GET", URL: "https://example.com", StatusCode: 200, ResponseBody: strings.Repeat("line\n", 40)}); err != nil {
		t.Fatal(err)
	}
	m := newHistoryModel(store).refresh().SetSize(80, 20)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if h := lipgloss.Height(m.View()); h != 20 {
		t.Fatalf("detail height = %d, want 20", h)
	}
	m, _ = m.Update(clipboardCopyMsg{token: m.copyToken})
	if v := m.View(); lipgloss.Height(v) != 20 || !strings.Contains(v, "copied to clipboard") {
		t.Errorf("after copy: height = %d", lipgloss.Height(v))
	}
}
```

`layoutScenarios`에 행 추가(store에 `Append`로 항목 2개를 넣은 뒤):
- `history-list`: `pressKeys(a, "h")`
- `history-detail-copy`: `pressKeys(a, "h", "enter")` 후 `applyMsg(a, clipboardCopyMsg{token: a.history.copyToken})`
- `dir-history`: `pressKeys(a, "d")`

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestHistoryModel_ListUsesFullWidth|TestHistoryModel_DetailCopyStatusKeepsHeight|TestApp_ViewFitsTerminal/(history|dir)'`
Expected: FAIL. 120칸에서 목록 폭이 내용 길이(46칸), 복사 뒤 상세 높이 21, `history-detail-copy/*` 높이 초과.

- [ ] **Step 3: 구현**

Interfaces 블록대로 `historyModel.SetSize`와 `View`를 고친다. History와 Directories의 loadErr 문구는 `fitLines(…, 안쪽 폭, height - vframe)`로 감싼다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/tui/`
Expected: PASS (기존 `// innerWidth = 56` 테스트 포함)

- [ ] **Step 5: 커밋**

```bash
git add internal/tui/historyview.go internal/tui/dirhistoryview.go internal/tui/historyview_test.go internal/tui/layout_test.go
git commit -m "tui: give history full width and a reserved status row"
```

---

### Task 9: 도움말을 스크롤 가능한 모달로 (P1-3, P1-4)

**Files:**
- Modify: `internal/tui/app.go:35-50` (App 필드), `:115-132` (KeyMsg 처리), `:204-224` (`footerHints`), `:226-257` (`helpView`), Task 5의 `resize()`
- Test: `internal/tui/app_test.go`, `internal/tui/layout_test.go`

**Interfaces:**
- Consumes: Task 4의 `hint`, Task 5의 `resize()`, `pressKeys`, `newLayoutApp`, `layoutScenarios`
- Produces: App 필드 `helpViewport viewport.Model`, `func (a App) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd)`, `func helpLines() []string`. 도움말 문구(현재 `helpView`의 `lines` slice, `:227-255`)는 내용을 바꾸지 않고 `helpLines()`로 옮긴다. 뒤 단계(P4)는 도움말 문구를 `helpLines()`에서만 고친다. `resize()`는 helpViewport를 폭 `a.width - frame`, 높이 `a.contentHeight() - vframe`으로 맞추고 `wrapToWidth(strings.Join(helpLines(), "\n"), 폭)`을 다시 넣는다. 도움말이 열려 있을 때 footer 힌트는 `{"↑/↓","scroll"}, {"esc","close"}, {"?","help"}, {"q","quit"}`이다.

- [ ] **Step 1: 실패하는 테스트 작성** (`internal/tui/app_test.go`)

```go
func helpTestApp(t *testing.T, w, h int) App {
	root := t.TempDir()
	writeTestFile(t, root, "a.http", "### one\nGET https://a.example/1\n\n### two\nGET https://a.example/2\n")
	writeTestFile(t, root, "b.http", "GET https://b.example/only\n")
	return newLayoutApp(t, root, newTestHistoryStore(t), w, h)
}

func TestApp_HelpSwallowsKeysForHiddenScreen(t *testing.T) {
	a := pressKeys(helpTestApp(t, 120, 35), "?", "j", "enter", "tab", "enter", "h", "d", "e")
	if !a.showHelp || a.screen != screenBrowser {
		t.Fatalf("showHelp = %v, screen = %v, want help still open over Browser", a.showHelp, a.screen)
	}
	if a.browser.files.Index() != 0 || a.browser.focus != paneFiles || a.browser.parsedFile != nil {
		t.Errorf("hidden Browser changed: index=%d focus=%v parsed=%v", a.browser.files.Index(), a.browser.focus, a.browser.parsedFile != nil)
	}
}

func TestApp_HelpEscClosesWithoutForwarding(t *testing.T) {
	a := pressKeys(helpTestApp(t, 120, 35), "enter", "?", "esc")
	if a.showHelp || a.browser.focus != paneRequests {
		t.Errorf("showHelp = %v, focus = %v, want help closed and esc not forwarded", a.showHelp, a.browser.focus)
	}
}

func TestApp_HelpQQuits(t *testing.T) {
	_, cmd := pressKeys(helpTestApp(t, 80, 24), "?").Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil || cmd() != tea.Quit() {
		t.Error("q inside help should quit")
	}
}

func TestApp_HelpScrollsAt80x24(t *testing.T) {
	a := pressKeys(helpTestApp(t, 80, 24), "?")
	view := ansi.Strip(a.View())
	assertViewFits(t, a.View(), 80, 24)
	first := strings.SplitN(view, "\n", 2)[0]
	if !strings.Contains(first, "rest-tui") || !strings.Contains(view, "Global") || !strings.Contains(view, "q, ctrl+c") {
		t.Fatalf("top of help is cut off:\n%s", view)
	}
	for i := 0; i < 30; i++ {
		a = pressKeys(a, "j")
	}
	if !strings.Contains(ansi.Strip(a.View()), "switch to selected directory") {
		t.Errorf("j did not scroll to the end of help")
	}
	for i := 0; i < 30; i++ {
		a = pressKeys(a, "k")
	}
	if !strings.Contains(ansi.Strip(a.View()), "Global") {
		t.Errorf("k did not scroll back to the top")
	}
}

func TestApp_HelpResizesWhileOpen(t *testing.T) {
	a := pressKeys(helpTestApp(t, 120, 35), "?")
	a = applyMsg(a, tea.WindowSizeMsg{Width: 60, Height: 20})
	assertViewFits(t, a.View(), 60, 20)
	a = pressKeys(a, "j")
	if a.helpViewport.YOffset != 1 {
		t.Errorf("helpViewport.YOffset = %d, want 1 after resize + j", a.helpViewport.YOffset)
	}
}
```

`layoutScenarios`에 `help` 행(`pressKeys(newLayoutApp(…), "?")`)을 추가하고, `TestApp_ViewDoesNotPanicOnTinyTerminal`의 키 목록에 `{"?"}`를 추가한다.

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestApp_Help|TestApp_ViewFitsTerminal/help|TestApp_ViewDoesNotPanicOnTinyTerminal'`
Expected: FAIL. 먼저 `helpViewport` 미정의 컴파일 오류가 난다. 필드만 추가하면 다음과 같이 실패한다.
- `showHelp = true, screen = 3`: `h`·`d`가 뒤 화면으로 새어 Directories로 이동한다.
- `showHelp = true, focus = 0`: `esc`가 Browser로 전달되고 도움말은 닫히지 않는다.
- 도움말 화면 높이가 31이다(80x24, 60x20).
- 크기를 바꾼 뒤 `j`를 눌러도 `YOffset = 0`이다.

`TestApp_HelpQQuits`는 지금도 PASS이고 회귀 방지용이다.

- [ ] **Step 3: 구현**

`App.Update`의 `tea.KeyMsg` 분기 맨 앞에서 `a.showHelp`이면 `updateHelp`로 넘긴다. `updateHelp`: `keys.Help` 또는 `msg.Type == tea.KeyEsc`면 닫는다. `keys.Quit`이면 `tea.Quit`. `keys.Up`·`keys.Down`이면 `helpViewport.ScrollUp(1)`/`ScrollDown(1)`. 그 밖의 키는 `a, nil`로 버린다. 도움말을 열 때 `helpViewport.GotoTop()`을 부른다. 키가 아닌 메시지(`execResultMsg`, clipboard tick 등)는 지금처럼 활성 화면에 전달한다. `helpView()`는 `paneFocusedStyle.Width(a.width - border).Render(a.helpViewport.View())`를 돌려준다. `footerHints()`는 `showHelp`일 때 Interfaces 블록의 힌트를 돌려준다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/tui/`
Expected: PASS (기존 `TestApp_HelpKeyTogglesShowHelp` 포함)

- [ ] **Step 5: 전체 검증**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: 모든 패키지 `ok`, vet 출력 없음, gofmt 출력 없음.

- [ ] **Step 6: 커밋**

```bash
git add internal/tui/app.go internal/tui/app_test.go internal/tui/layout_test.go
git commit -m "tui: make help a scrollable modal that swallows keys"
```

---

### Task 10: 실제 바이너리를 tmux로 확인하고 dev 바이너리 재빌드 (P1-1~P1-9)

**Files:**
- Modify (재생성): `rest-tui`
- 임시 픽스처: `mktemp -d`로 만든 저장소 밖 디렉터리(커밋하지 않는다)

**Interfaces:**
- Consumes: Task 1~9의 결과 전체
- Produces: 없음

- [ ] **Step 1: 바이너리 재빌드**

Run: `scripts/build.sh`
Expected: `go vet ./...` 통과 뒤 `[build.sh] rest-tui updated`

- [ ] **Step 2: 픽스처와 목 서버 준비**

```bash
REPO=$(pwd); FIX=$(mktemp -d -t rest-tui-p1); mkdir -p "$FIX/http" "$FIX/home"
cat > "$FIX/mock.py" <<'EOF'
import http.server, time
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/slow": time.sleep(6); code, ct, body = 200, "text/plain", b"slow done"
        else: code, ct, body = {
            "/users": (200, "application/json", b'[{"id":1}]'),
            "/long": (200, "application/json", ('{"path":"' + "segment/"*60 + '","text":"' + "lorem ipsum "*40 + '"}').encode()),
            "/evil": (200, "text/plain", b"hello \x1b[2J\x1b[H\x1b]0;PWNED-TITLE\x07\x1b[41mRED\x1b[0m end"),
            "/bin": (200, "image/png", bytes(range(256))*20),
        }.get(self.path, (404, "text/plain", b"not found"))
        self.send_response(code); self.send_header("Content-Type", ct); self.end_headers(); self.wfile.write(body)
http.server.ThreadingHTTPServer(("127.0.0.1", 18099), H).serve_forever()
EOF
printf '### listUsers\nGET http://127.0.0.1:18099/users\n\n### slow\nGET http://127.0.0.1:18099/slow\n\n### long\nGET http://127.0.0.1:18099/long\n\n### evil\nGET http://127.0.0.1:18099/evil\n\n### binary\nGET http://127.0.0.1:18099/bin\n' > "$FIX/http/users.http"
printf 'GET http://127.0.0.1:18099/users\n' > "$FIX/http/one.http"
{ printf '### good\nGET http://127.0.0.1:18099/users\n'; for i in 0 1 2 3 4; do printf '\n### bad%s\nthis is not a request line number %s with extra words\n' $i $i; done; } > "$FIX/http/broken.http"
python3 -I "$FIX/mock.py" & echo $! > "$FIX/mock.pid"
T="/opt/homebrew/bin/tmux -L rest-tui-p1 -f /dev/null"
$T new-session -d -s p1 -x 120 -y 35 "env HOME=$FIX/home $REPO/rest-tui -dir $FIX/http -config $FIX/history.json"
```

`HOME`을 바꿔 사용자의 실제 `~/.config/rest-tui/dirs.json`을 건드리지 않는다. 파일 순서는 `broken.http`, `one.http`, `users.http`다. 화면은 `$T capture-pane -p -t p1`로 본다.

- [ ] **Step 3: P1-8 패널 분할 (120x35)**

Run: `$T capture-pane -p -t p1 | sed -n 2p | python3 -c 'import sys; l=sys.stdin.read().rstrip("\n"); print(len(l), l.index("┓")+1)'`
Expected: `120 48`. 1행에 `rest-tui › Browser (`가 있다.

- [ ] **Step 4: P1-2 파일 전환 뒤 Enter**

`$T send-keys -t p1 j j Enter j j Escape k Enter`(users.http에서 `long`을 고른 뒤 one.http를 연다) 후 캡처.
Expected: Requests 패널의 `GET http://127.0.0.1:18099/users` 앞에 선택 막대 `│`가 있다. 이어서 `$T send-keys -t p1 Enter` 후 1행이 `rest-tui › Browser › GET http://127.0.0.1:18099/users`다.

- [ ] **Step 5: P1-1 이전 응답이 새 화면에 붙지 않음**

`$T send-keys -t p1 Escape Escape j Enter j Enter Enter`(slow 전송) → `Escape k Enter`(listUsers를 열고 보내지 않음) → 7초 대기 후 캡처.
Expected: `(not yet sent — press enter to send)`가 보이고 `slow done`은 없다. `$T send-keys -t p1 h` 후 History에 `/slow` 항목이 있다(히스토리 저장 유지). `Escape`로 Browser로 돌아온다.

- [ ] **Step 6: P1-5, P1-4, P1-7 긴 응답 (120x35 → 80x24)**

`$T send-keys -t p1 j j Enter Enter`(Browser의 listUsers에서 `long`을 열어 전송) 후 1초 뒤 캡처한다.
Expected: 1행에 `rest-tui › Browser › long`, 2행이 `╭`로 시작, 마지막 행에 footer(`q quit`)가 있다(capture는 항상 창 높이만큼 나오므로 줄 수가 아니라 첫·끝 행으로 넘침을 본다). 본문 `segment/segment/…` 줄이 패널 안쪽 폭을 꽉 채우고 `se`·`gment` 같은 짧은 조각 줄이 없다. 이어서 `$T resize-window -t p1 -x 80 -y 24` 후 같은 확인을 하고, 마지막 행이 `? help  q quit`으로 끝나는지 본다.

- [ ] **Step 7: P1-3 도움말 모달 (80x24)**

`$T send-keys -t p1 ?` 후 캡처.
Expected: 1행 `rest-tui ›`, 위쪽에 `Global`과 `q, ctrl+c     quit`. `j`를 15번 보내면 `switch to selected directory`가 보인다. `Enter Tab h`를 보낸 뒤에도 도움말이 그대로이고 1행 breadcrumb가 바뀌지 않는다. `Escape`로 닫으면 `long` 응답 화면이 그대로 있다.

- [ ] **Step 8: P1-6 파싱 경고**

`$T send-keys -t p1 Escape Escape k k Enter`(Browser로 돌아가 Files에서 `broken.http`를 연다) 후 캡처한다.
Expected: 경고가 3줄 이하이고 마지막 줄이 `… +N more` 형식이다. Requests 패널에 `good`이 보인다. 1행에 breadcrumb가 남아 있고 마지막 행에 footer(`q quit`)가 있다.

- [ ] **Step 9: P1-9 제어 시퀀스와 바이너리**

`$T send-keys -t p1 Escape j j Enter j j j Enter Enter`(users.http의 `evil`을 열어 전송) 후 1초 뒤 캡처한다.
Expected: 본문에 `hello RED end`가 보이고 breadcrumb·테두리가 남아 있다. `$T display -p -t p1 '#{pane_title}'`이 `PWNED-TITLE`이 아니다. `$T capture-pane -e -p -t p1 | grep 'hello RED end' | grep -c '41m'`이 `0`이다. 본문 줄로 범위를 좁혀, 앱 색상의 `38;5;241m` 같은 코드가 잘못 걸리지 않게 한다. 이어서 `$T send-keys -t p1 Escape j Enter Enter`(`binary` 전송) 후 `(binary body: 5120 bytes, image/png)`가 보인다.

- [ ] **Step 10: 정리**

Run: `$T kill-server; kill "$(cat "$FIX/mock.pid")"; rm -rf "$FIX"`
Expected: 오류 없이 끝난다. `git status --short`에는 `rest-tui`와 계획을 시작하기 전부터 있던 항목(예: 미추적 `docs/superpowers/` 파일)만 있다.

- [ ] **Step 11: 바이너리 커밋**

```bash
git add rest-tui
git commit -m "build: rebuild committed dev binary"
```

---

## 요구사항 → 태스크 추적표

| 요구사항 | 태스크 | 검증 테스트 / 확인 |
|---|---|---|
| P1-1 | Task 2 | `TestRequestModel_IgnoresExecResultFromAnotherModel`, `TestApp_StaleExecResultDoesNotLandOnNewRequestView`, Task 10 Step 5 |
| P1-2 | Task 3 | `TestBrowserModel_SelectFileResetsRequestCursorToFirst`, Task 10 Step 4 |
| P1-3 | Task 9 | `TestApp_HelpSwallowsKeysForHiddenScreen`, `TestApp_HelpEscClosesWithoutForwarding`, `TestApp_HelpQQuits`, `TestApp_HelpScrollsAt80x24`, `TestApp_HelpResizesWhileOpen`, Task 10 Step 7 |
| P1-4 | Task 5(도구·프레임), 6, 7, 8, 9(시나리오 추가) | `TestApp_ViewFitsTerminal`(시나리오 × 60x20/80x24/120x35/200x50), `TestApp_ViewDoesNotPanicOnTinyTerminal`, Task 10 Step 6·8 |
| P1-5 | Task 6 | `TestRequestModel_ViewportWidthMatchesPaneInnerWidth`, `TestRequestModel_TabIndentedBodyEndStaysReachable`, Task 10 Step 6 |
| P1-6 | Task 7 | `TestBrowserModel_ParseWarningWrapsToAtMostThreeLines`, Task 10 Step 8 |
| P1-7 | Task 4 | `TestRenderHints_DropsMiddleHintsKeepsHelpAndQuit`, `TestApp_FooterKeepsHelpAndQuitAt80Columns`, Task 10 Step 6 |
| P1-8 | Task 7, 8 | `TestBrowserModel_PanesSplitFullWidth40And60`, `TestHistoryModel_ListUsesFullWidth`, Task 10 Step 3 |
| P1-9 | Task 1 | `TestSanitize_StripsEscapeSequencesAndControls`, `TestIsBinary`, `TestRenderResponse_SanitizesHeadersAndBody`(JSON 안 C1 포함), `TestRenderResponse_BinaryBodyPlaceholder`, Task 10 Step 9 |
| 계약: 상태 줄 1행 | Task 6 | `TestRequestModel_StatusRowIsReservedForCopyAndEditError` |

## 실행 시 참고

- Task 1~3은 서로 독립이다. Task 4 → 5 → 6~9 순서는 지킨다(5가 4의 footer를 전제로 표 테스트를 통과시키고, 6~9는 5의 `layoutScenarios`에 행을 더한다).
- Task 6~8은 서로 독립이라 순서를 바꿔도 된다. Task 9는 Task 5의 `resize()`를 고치므로 Task 5 다음이면 된다.
- 해석으로 정한 것 두 가지: footer는 고정 두 힌트에 가장 가까운 것부터 뺀다(P1-7의 "가운데"). 파싱 경고의 `… +N more`에서 N은 숨긴 줄 수이고 표시는 합쳐서 3줄이다(P1-6). 리뷰에서 다르게 정하면 `TestRenderHints_…`와 `TestBrowserModel_ParseWarning…`의 기대값만 바꾸면 된다.
