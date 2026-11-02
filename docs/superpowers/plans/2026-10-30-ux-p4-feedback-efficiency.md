# rest-tui UX 개선 P4 — 피드백·효율·안전 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** spec P4-1~P4-17을 모두 구현한다. 내용은 전송 중 피드백(spinner, 경과 시간, 취소, 타임아웃), 응답 요약 줄과 색, 리다이렉트 표시, 목록 필터·재스캔·경로 열기·출발 화면 복귀, 히스토리 저장소 보안, CLI·README 정리다.

**Architecture:** 실행 계층(`internal/executor`)은 리다이렉트 경로와 취소·타임아웃 원인을 돌려준다. 저장 계층(`internal/history`)은 원자적 쓰기와 0600 권한, 500건 상한을 맡는다. UI(`internal/tui`)는 MVU 값 모델을 유지한다. 전송 수명(spinner, cancel)은 `requestModel`에 두고, 종료 확인·출발 화면·상태 줄은 `App`이 맡는다. CLI 해석은 `main.go`의 순수 함수(`parseArgs`, `resolveVersion`)로 분리해 테스트한다.

**Tech Stack:** Go 1.26.4, Bubble Tea v1.3.10, Bubbles v1.0.0(`spinner`, `list`, `viewport`, `textinput`), Lip Gloss v1.1.0, `charmbracelet/x/ansi` v0.11.6, 표준 `testing`, tmux(`/opt/homebrew/bin/tmux`).

**Spec:** `docs/superpowers/specs/2026-10-30-ux-improvements-design.md` — P4 표, "단계 간 계약", "공통 규칙", "범위 밖".

**전제:** P1~P3 계획이 머지되어 있다. 이 계획은 앞 단계의 다음 계약만 쓴다.

| 만든 단계 | 이름 | 이 계획에서 쓰는 곳 |
|---|---|---|
| P1 | `requestModel.id`, `execResultMsg.modelID`, `nextRequestModelID()` | Task 3·5·6 (결과 메시지 생성·라우팅) |
| P1 | `assertViewFits(t, view, width, height)` (`internal/tui/layout_test.go`) | 레이아웃 테스트 전부 |
| P1 | `hint{key, desc}`, `renderHints(hints, width)` | footer 힌트 slice 변경 |
| P1 | Request view 고정 상태 줄, `requestModel.statusLine() string` | Task 4 (스크롤 %) |
| P2 | `newRequestModel(filePath, root, file, req, store, preferredEnv)`, `history.Entry.Env` | 테스트 생성자, Task 7 행 형식 |
| P3 | (직접 쓰지 않음) | — |

계약 밖이지만 앞 단계 계획이 만든다고 적은 이름도 쓴다. 실행 전에 `grep -n '<이름>' internal/tui/*.go`로 확인하고, 실제 이름이 다르면 그 이름을 쓴다.

- P1 계획: `layoutSizes`(60x20, 80x24, 120x35, 200x50), `layoutScenarios`, `TestApp_ViewFitsTerminal`, `newLayoutApp(t, root, store, w, h)`, `pressKeys(a, ks...)`, `applyMsg(a, msg)`, `App.resize()`, `fitLines(s, w, h)`, `updateHelp`, `helpLines()`(도움말 문구가 있는 곳. P1 이후 `helpView()`는 viewport만 렌더하므로 문구는 여기서 고친다). `statusMsg`가 비어 있지 않으면 `contentHeight()`가 1행을 더 빼므로, `statusMsg`를 바꾼 뒤에는 `resize()`를 불러야 한다.
- P2 계획: `colorWarning`(`lipgloss.Color("11")`), `warnTextStyle`, `requestModel.fromEntry`, `requestModel.entryEnv`, `newRequestModelFromEntry`를 쓰는 테스트 `TestRequestModel_RerunRecordsOriginalEnv`, `TestRequestModelFromEntry_EnvLineShowsSendTimeEnv`.

---

## Global Constraints

- 각 태스크는 실패하는 테스트로 시작한다. 태스크를 마칠 때 `go test ./...`, `go vet ./...`, `gofmt -l .`(출력 없음)이 모두 통과해야 한다.
- 레이아웃이 바뀌는 태스크는 `assertViewFits`로 60x20, 80x24, 120x35에서 검증한다. 방법은 P1의 `layoutScenarios`에 시나리오 행을 추가하는 것이다. `TestApp_ViewFitsTerminal`이 `layoutSizes` 전부에서 `assertViewFits`를 부른다.
- 커밋 메시지는 영어 `<area>: <summary>` 형식이다(`tui: …`, `history: …`, `executor: …`, `cli: …`, `docs: …`).
- 사용자에게 보이는 UI 문구는 영어이고, spec이 정한 문구는 글자 그대로 쓴다(`→`, `…`, `·`, `—` 포함).
- 의존성을 추가하지 않는다. `go.mod`는 바뀌지 않는다(`spinner`, `textinput`은 bubbles v1.0.0 안에 있다).
- footer 렌더링 경로는 P1의 `renderHints` 하나다. 이 계획은 화면별 `[]hint` slice만 바꾼다. Request view의 한 줄 상태 메시지는 `statusLine()` 하나에서 만든다.
- 히스토리 내용은 마스킹하지 않는다(spec 결정: rerun 정확성 유지).
- 범위 밖: 마우스, 응답 본문 검색, 연속/일괄 실행, curl 복사·파일 저장, OSC52, `NO_COLOR` 본문 적용, charset 변환, Windows 경로 차이, JS 핸들러 실행.
- 코드 주석은 지시하지 않는다. 꼭 필요하면 "조용히 깨지는 것"을 설명하는 한 줄만 쓴다.
- 줄 번호는 HEAD `5d02c73` 기준이고, P1~P3이 건드리지 않는 `main.go`, `internal/executor/executor.go`에만 쓴다. 나머지 파일은 함수 이름으로 찾는다.

## Review Focus

1. 전송 중 History나 Directories로 옮겨 간 사이에 결과가 도착하는 경우: Request view가 결과를 받아 전송 중 상태가 풀려야 한다. 그러지 않으면 `q`가 계속 확인을 요구하고 spinner tick이 끊긴다. → Task 5 `TestApp_ResultArrivesWhileOnOtherScreen`
2. 헤더는 받았지만 본문이 느린 응답에서 취소하거나 타임아웃이 나는 경우: 본문을 읽다가 난 오류도 `canceled` / `timed out after …`로 분류되어야 한다. → Task 1 `TestExecute_SlowBodyTimeoutIsDeadlineExceeded`, `TestExecute_CanceledDuringBodyIsCanceled`
3. 목록 필터를 입력하는 중에 `q`·`h`·`d`·`?`·`R`·`o`·`tab`·`esc`를 누르는 경우: 키가 필터 입력으로 들어가야 하고, 종료·화면 전환·포커스 이동이 일어나면 안 된다. → Task 8 `TestApp_FilterTypingSwallowsGlobalKeys`
4. History에서 rerun한 뒤 돌아오는 경우: 새 항목이 맨 위에 쌓여도 커서는 같은 항목(ID)에 남아야 한다. 그래야 다음 `r`이 엉뚱한 요청을 다시 보내지 않는다. → Task 6 `TestHistoryModel_RefreshKeepsCursorOnSameEntry`
5. 위치 인자 뒤에 플래그를 쓰는 경우(`rest-tui ./api -timeout 5s`): Go `flag`는 첫 위치 인자에서 해석을 멈춘다. 그래도 `-timeout`은 플래그로 처리되어야 한다. → Task 3 `TestParseArgs_FlagsAfterPositional`

## File Structure

| 파일 | 책임 | 태스크 |
|---|---|---|
| `main.go`, `main_test.go`(신규) | 인자 해석(`parseArgs`), 버전 결정(`resolveVersion`), `-h` 문구 | 3 |
| `internal/executor/executor.go` | 리다이렉트 경로 기록, 컨텍스트 원인 보존 | 1 |
| `internal/history/store.go` | `Entry.Name`·`Entry.Redirects`, 원자적 쓰기, 0600, 500건 | 2 |
| `internal/output/render.go` | 응답 상단 `redirected:` 줄 | 4 |
| `internal/tui/app.go` | `Options`, `NewApp`, 출발 화면, 종료 확인, 결과 라우팅, footer·help 문구, 필터 중 전역 키 차단, 스캔 알림, `R`/`o` | 3, 4, 5, 6, 8, 9, 10 |
| `internal/tui/requestview.go` | 타임아웃, `startSend`, 요약 줄, spinner, 취소, `g`/`G`, 스크롤 % | 3, 4, 5, 6, 7 |
| `internal/tui/historyview.go` | rerun 확인, 커서 유지, 행 형식, `entryRowDelegate`, 필터 | 6, 7, 8 |
| `internal/tui/browser.go` | 스캔 건너뛰기, 빈 상태, 필터, 한 줄 파일 항목, 재스캔 | 7, 8, 9, 10 |
| `internal/tui/dirhistoryview.go` | 필터, 경로 입력 프롬프트 | 8, 10 |
| `internal/tui/keys.go`, `messages.go`, `styles.go` | 새 키, `backFromRequestMsg`, 상태·메서드 색 | 4, 6, 10 |
| `internal/tui/cmd_helpers_test.go`(신규) | `awaitMsg`, `newTestRequestModel`, `skipIfNoPermEnforcement` | 3, 9 |
| `internal/tui/layout_test.go` | `layoutScenarios` 행 추가, `newLayoutApp`의 `NewApp` 호출 갱신 | 3, 4, 5, 6, 7, 9, 10 |
| `README.md`, `CLAUDE.md` | 키 표, 설치 명령, 히스토리 보안 안내, 플래그 | 11 |

---

### Task 1: executor — 리다이렉트 경로와 취소·타임아웃 원인 보존 (P4-4, P4-1·P4-3 기반)

**Files:**
- Modify: `internal/executor/executor.go:15-22` (`Response`), `:44` (`client`), `:46-56` (오류 반환)
- Test: `internal/executor/executor_test.go`

**Interfaces:**
- Consumes: 없음
- Produces:
  - `executor.Response.Redirects []string`. 따라간 순서대로, 항목마다 `"<리다이렉트 응답 상태 코드> → <다음 요청 METHOD> <다음 요청 URL>"`(예: `302 → GET http://127.0.0.1:18094/new`) 형식이다. 리다이렉트가 없으면 `nil`이다.
  - `executor.Execute`가 돌려주는 오류: 부모 ctx가 취소되었으면 `errors.Is(err, context.Canceled)`, 타임아웃이면 `errors.Is(err, context.DeadlineExceeded)`가 참이다. `client.Do` 단계와 본문을 읽는 단계 모두 해당한다.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
func TestExecute_RecordsRedirectHops(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/b", http.StatusFound) })
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/c", http.StatusTemporaryRedirect) })
	mux.HandleFunc("/c", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := Execute(context.Background(), httpfile.Request{Method: "POST", URL: srv.URL + "/a", Body: "x"}, 2*time.Second)
	// err == nil, resp.StatusCode == 200
	want := []string{"302 → GET " + srv.URL + "/b", "307 → GET " + srv.URL + "/c"}
	// reflect.DeepEqual(resp.Redirects, want)
}

func TestExecute_RedirectLimitUnchanged(t *testing.T) {
	// "/loop"가 자기 자신으로 302를 계속 돌려준다
	// err != nil && strings.Contains(err.Error(), "stopped after 10 redirects")
}

func TestExecute_SlowBodyTimeoutIsDeadlineExceeded(t *testing.T) {
	// 핸들러: WriteHeader(200), w.(http.Flusher).Flush(), 그 뒤 select { <-r.Context().Done(), <-time.After(2s) }
	_, err := Execute(context.Background(), req, 50*time.Millisecond)
	// errors.Is(err, context.DeadlineExceeded)
}

func TestExecute_CanceledDuringBodyIsCanceled(t *testing.T) {
	// 위와 같은 느린 본문 핸들러. ctx, cancel := context.WithCancel(...); 50ms 뒤 cancel()
	_, err := Execute(ctx, req, 5*time.Second)
	// errors.Is(err, context.Canceled)
}
```

`TestExecute_GetCapturesStatusHeadersAndBody`에는 `resp.Redirects != nil`이면 실패하는 단언 한 줄을, `TestExecute_TimeoutReturnsError`에는 `errors.Is(err, context.DeadlineExceeded)` 단언을 추가한다.

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/executor/ -run 'TestExecute_(RecordsRedirectHops|RedirectLimitUnchanged|SlowBodyTimeoutIsDeadlineExceeded|CanceledDuringBodyIsCanceled|TimeoutReturnsError)'`
Expected: FAIL. `resp.Redirects undefined` 컴파일 오류가 난다.

- [ ] **Step 3: 구현**

- `Response`에 `Redirects []string`을 추가한다.
- `http.Client{CheckRedirect: …}` 클로저는 `req.Response.StatusCode`, `req.Method`, `req.URL.String()`으로 항목을 만들어 로컬 slice에 쌓는다. `len(via) >= 10`이면 `errors.New("stopped after 10 redirects")`를 돌려준다. nil 정책일 때와 같은 동작이다.
- `client.Do`나 `io.ReadAll`이 실패했고 `ctx.Err() != nil`이면: `errors.Is(err, ctx.Err())`가 이미 참이면 `err`를 그대로, 아니면 `fmt.Errorf("%w: %w", ctx.Err(), err)`를 돌려준다. 여기서 `ctx`는 `context.WithTimeout`으로 만든 내부 ctx다(부모 취소면 `Canceled`, 시간 초과면 `DeadlineExceeded`).

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/executor/`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/executor/executor.go internal/executor/executor_test.go
git commit -m "executor: record redirect hops and keep cancel/timeout cause in errors"
```

---

### Task 2: history 저장소 — 0600, 최근 500건, 원자적 쓰기, `Name`·`Redirects` 필드 (P4-11, P4-4 기록)

**Files:**
- Modify: `internal/history/store.go` (`Entry`, `Append`, `save`)
- Test: `internal/history/store_test.go`

**Interfaces:**
- Consumes: 없음
- Produces:
  - `history.Entry.Name string` (`json:",omitempty"`). 요청 이름(`### name`)이고, 없으면 `""`다.
  - `history.Entry.Redirects []string` (`json:",omitempty"`). `executor.Response.Redirects`를 그대로 담는다.
  - 패키지 상수 `maxEntries = 500`. `Append`는 저장할 때 가장 오래된 항목부터 버려 최근 500건만 남긴다.
  - `save`는 같은 디렉터리에 `os.CreateTemp(dir, base+".tmp-*")`로 임시 파일을 만들고, 쓰기와 `Close`를 마친 뒤 `Chmod(0o600)`를 하고 `os.Rename`으로 교체한다. 실패하면 임시 파일을 지우고 원본은 그대로 둔다. 기존 0644 파일도 다음 저장에서 0600이 된다.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
func skipIfNoPermEnforcement(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file permission bits are not enforced here")
	}
}

func TestStore_NewFileHasMode0600(t *testing.T) {
	skipIfNoPermEnforcement(t)
	s, path := newTestStore(t)
	// s.Append(Entry{Method: "GET", URL: "u"})
	// os.Stat(path).Mode().Perm() == 0o600
}

func TestStore_AppendTightensExistingFileTo0600(t *testing.T) {
	skipIfNoPermEnforcement(t)
	// os.WriteFile(path, []byte("[]"), 0o644); Append → Mode().Perm() == 0o600
}

func TestStore_AppendKeepsNewest500(t *testing.T) {
	// json.Marshal로 URL "u0".."u499"인 500건을 파일에 미리 쓴다
	// s.Append(Entry{URL: "new"})
	got, _ := s.List(0)
	// len(got) == 500, got[0].URL == "new", got[499].URL == "u1"
}

func TestStore_AppendLeavesNoTempFiles(t *testing.T) {
	// Append 두 번 → os.ReadDir(filepath.Dir(path))의 이름 목록 == []string{"history.json"}
}

func TestStore_FailedWriteKeepsOriginal(t *testing.T) {
	skipIfNoPermEnforcement(t)
	// Append 1건 → before := os.ReadFile(path) → os.Chmod(dir, 0o500), t.Cleanup(0o755 복구)
	// _, err := s.Append(Entry{URL: "second"}); err != nil
	// os.ReadFile(path) == before
}

func TestStore_NameAndRedirectsRoundTrip(t *testing.T) {
	// Append(Entry{Name: "Create user", Redirects: []string{"302 → GET http://x/new"}}) → Get(ID)로 같은 값
	// Name·Redirects가 빈 Entry만 저장한 파일 원문에는 "Redirects"와 "\"Name\""가 없다
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/history/ -run 'TestStore_(NewFileHasMode0600|AppendTightensExistingFileTo0600|AppendKeepsNewest500|AppendLeavesNoTempFiles|FailedWriteKeepsOriginal|NameAndRedirectsRoundTrip)'`
Expected: FAIL. `Name`·`Redirects` 필드가 없어 컴파일 오류가 난다. 필드만 추가하면 0600(현재 0644)과 500건 단언에서 실패한다.

- [ ] **Step 3: 구현** — Interfaces에 적은 필드, 상수, `save` 동작을 그대로 만든다. `Append`의 `fileMu` 임계 구역은 바꾸지 않는다.

- [ ] **Step 4: 통과 확인**

Run: `go test -race ./internal/history/`
Expected: PASS (`TestStore_ConcurrentAppendDoesNotLoseEntries` 포함)

- [ ] **Step 5: 커밋**

```bash
git add internal/history/store.go internal/history/store_test.go
git commit -m "history: write atomically with 0600 perms, cap at 500 entries, store name and redirects"
```

---

### Task 3: CLI·`tui.Options`·타임아웃 (P4-13, P4-16, P4-3)

**Files:**
- Modify: `main.go:37-71` (`main` → `parseArgs`/`resolveVersion` 사용)
- Create: `main_test.go`
- Modify: `internal/tui/app.go` (`Options`, `NewApp`, `App.opts`, `openRequestMsg`·`rerunMsg`·`openRequestFromEntryMsg` 분기)
- Modify: `internal/tui/requestview.go` (`requestTimeout` → `defaultRequestTimeout`, `sendCmd`, `Update`의 Send 분기, `newRequestModelFromEntry` 삭제)
- Create: `internal/tui/cmd_helpers_test.go`
- Modify: `grep -n 'NewApp(' internal/tui/*_test.go`로 찾은 모든 호출(`newTestApp`, P1 `newLayoutApp` 포함)에 `Options{}` 추가, `internal/tui/requestview_test.go`
  - `TestNewRequestModelFromEntry_SkipsEnvUIAndSendsImmediately`는 삭제하고 아래 App 테스트로 대체한다.
  - P2의 `TestRequestModel_RerunRecordsOriginalEnv`, `TestRequestModelFromEntry_EnvLineShowsSendTimeEnv`는 `newRequestModelFromEntry(e, s)` 호출을 `newRequestModelFromEntryNoSend(e, s)` 다음 `startSend()`를 부르는 두 줄로 바꾼다(`m`이 이미 선언된 자리에서는 `:=` 대신 `=`, cmd를 쓰지 않는 자리에서는 `_`).

**Interfaces:**
- Consumes: P1 `execResultMsg.modelID`, P2 `newRequestModel(...)`, `newRequestModelFromEntryNoSend`(P2 이후 시그니처 그대로)
- Produces:
  - `type Options struct { Timeout time.Duration }`, `func NewApp(root string, store *history.Store, dirStore *dirhistory.Store, opts Options) App`, 필드 `App.opts Options`
  - `const defaultRequestTimeout = 30 * time.Second`
  - `requestModel.timeout time.Duration`, `func (m requestModel) withTimeout(d time.Duration) requestModel`, `func (m requestModel) sendTimeout() time.Duration`(`timeout <= 0`이면 `defaultRequestTimeout`)
  - `func (m requestModel) startSend() (requestModel, tea.Cmd)`. `executing = true`, `refreshContent`, `scrollToResponse`를 한 뒤 send cmd를 돌려준다. Send 키와 App의 rerun이 모두 이것을 쓴다.
  - `func describeSendError(err error, timeout time.Duration) string`. `errors.Is(err, context.DeadlineExceeded)`이면 `fmt.Sprintf("timed out after %s (use -timeout to change)", timeout)`, 그 밖에는 `err.Error()`다. `sendCmd`가 `entry.Error`에 넣는다.
  - `func (a App) openRequestView(m requestModel) App`. `withTimeout(a.opts.Timeout)`, `SetSize(a.width, a.contentHeight())`, `a.screen = screenRequest`를 한 번에 한다(Task 6에서 `origin` 인자가 붙는다).
  - `newRequestModelFromEntry`를 삭제한다. `rerunMsg` 분기는 `newRequestModelFromEntryNoSend` → `openRequestView` → `a.request.startSend()` 순서다. 삭제하기 전에 두 생성자의 본문을 비교한다. `newRequestModelFromEntry`만 채우는 필드(P1 `id`, P2 `fromEntry`·`entryEnv`, P3 `now`·`rng` 등)가 있으면 `NoSend`도 같게 채우는지 확인한다. 이름 앞의 `(rerun) `은 없앤다. rerun 표시는 Task 6의 breadcrumb가 맡는다.
  - `main.go`: `type cliConfig struct { root, historyPath string; timeout time.Duration; showVersion bool }`, `func parseArgs(args []string, stderr io.Writer, defaultHistory string) (cliConfig, error)`, `func resolveVersion(linked string, info *debug.BuildInfo, ok bool) string`
  - 테스트 헬퍼(`cmd_helpers_test.go`): `func awaitMsg[T any](t *testing.T, cmd tea.Cmd) T`. cmd를 실행하고 `tea.BatchMsg`를 재귀적으로 펼쳐 하위 cmd를 goroutine으로 돌린다. 하위 cmd가 모두 끝날 때까지(전체 5초 제한) 기다린 뒤 처음 나온 `T`를 돌려주고, 없거나 시간을 넘기면 `t.Fatal`이다. 모두 기다리는 이유: 남은 send goroutine이 테스트 종료 뒤 `t.TempDir()`에 히스토리를 쓰면 TempDir 정리가 실패한다. `func newTestRequestModel(t *testing.T, req httpfile.Request) requestModel`(= `newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "").SetSize(120, 35)`. 크기가 0이면 요약·상태 줄이 폭 0으로 잘리고 viewport가 빈 문자열을 그린다). App 크기 지정은 P1 `newLayoutApp`/`applyMsg(a, tea.WindowSizeMsg{…})`를 쓴다.

- [ ] **Step 1: 실패하는 테스트 작성 (`main_test.go`)**

```go
func TestParseArgs_PositionalDir(t *testing.T) {
	dir := t.TempDir()
	cfg, err := parseArgs([]string{dir}, io.Discard, "h.json")
	// err == nil, cfg.root == dir, cfg.timeout == 30*time.Second, cfg.historyPath == "h.json"
}

func TestParseArgs_FlagsAfterPositional(t *testing.T) {
	cfg, err := parseArgs([]string{dir, "-timeout", "5s"}, io.Discard, "h.json")
	// err == nil, cfg.root == dir, cfg.timeout == 5*time.Second
}

func TestParseArgs_DirFlagAndDifferentPositionalIsError(t *testing.T) {
	_, err := parseArgs([]string{"-dir", a, b}, io.Discard, "h.json")
	// err != nil && strings.Contains(err.Error(), "point to different directories")
}

func TestParseArgs_DirFlagAndSamePositionalIsOK(t *testing.T) {
	// parseArgs([]string{"-dir", dir, dir + "/"}) → err == nil, cfg.root == dir
}

func TestParseArgs_TooManyPositionalIsError(t *testing.T) {
	// parseArgs([]string{a, b}) → err.Error() == "expected at most one directory argument, got 2"
}

func TestParseArgs_NonPositiveTimeoutIsError(t *testing.T) {
	// "-timeout", "0" → strings.Contains(err.Error(), "-timeout must be positive")
}

func TestParseArgs_HelpShowsUsage(t *testing.T) {
	var buf bytes.Buffer
	_, err := parseArgs([]string{"-h"}, &buf, "h.json")
	// errors.Is(err, flag.ErrHelp)
	// buf에 다음이 모두 들어 있다: "browse and run IntelliJ HTTP Client (.http) files", "rest-tui [flags] [dir]",
	//   "rest-tui ./api", "Press ? inside the app", "history file path", "-timeout"
}

func TestResolveVersion(t *testing.T) {
	// resolveVersion("v1.2.3", nil, false) == "v1.2.3"
	// resolveVersion("dev", &debug.BuildInfo{Main: debug.Module{Version: "v0.2.2"}}, true) == "v0.2.2"
	// resolveVersion("dev", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, true) == "dev"
	// resolveVersion("dev", nil, false) == "dev"
}
```

- [ ] **Step 2: 실패하는 테스트 작성 (tui)**

```go
func TestDescribeSendError_Timeout(t *testing.T) {
	err := fmt.Errorf("Get \"http://x\": %w", context.DeadlineExceeded)
	// describeSendError(err, 30*time.Second) == "timed out after 30s (use -timeout to change)"
	// describeSendError(errors.New("boom"), 30*time.Second) == "boom"
}

func TestRequestModel_SendTimeoutDefaultsTo30s(t *testing.T) {
	m := newTestRequestModel(t, httpfile.Request{Method: "GET", URL: "http://x"})
	// m.sendTimeout() == 30*time.Second; m.withTimeout(5*time.Second).sendTimeout() == 5*time.Second
}

func TestApp_RerunMsgSendsWithAppTimeout(t *testing.T) {
	// srv 핸들러는 <-r.Context().Done() 또는 2s까지 막힌다
	a := applyMsg(NewApp(t.TempDir(), store, dirStore, Options{Timeout: 50 * time.Millisecond}), tea.WindowSizeMsg{Width: 80, Height: 24})
	model, cmd := a.Update(rerunMsg{entry: history.Entry{Method: "GET", URL: srv.URL}})
	// model.(App).screen == screenRequest, model.(App).request.executing
	msg := awaitMsg[execResultMsg](t, cmd)
	// msg.entry.Error == "timed out after 50ms (use -timeout to change)"
}

func TestApp_RerunMsgSendsImmediately(t *testing.T) {
	// srv: 200 + `{"replayed":true}` → awaitMsg[execResultMsg] → StatusCode 200, ResponseBody `{"replayed":true}`
	// msg.modelID == model.(App).request.id; applyMsg(model.(App), msg).request.lastEntry.StatusCode == 200
}
```

- [ ] **Step 3: 실패 확인**

Run: `go test . -run 'TestParseArgs|TestResolveVersion' && go test ./internal/tui/ -run 'TestDescribeSendError_Timeout|TestRequestModel_SendTimeoutDefaultsTo30s|TestApp_RerunMsgSends'`
Expected: FAIL. `parseArgs`, `Options`, `describeSendError` 등이 정의되지 않아 컴파일 오류가 난다.

- [ ] **Step 4: `main.go` 구현**

- `flag.NewFlagSet("rest-tui", flag.ContinueOnError)`을 쓰고 출력은 `stderr`로 보낸다. `cfg.root`는 위치 인자(없으면 `-dir` 값)를 `filepath.Abs`로 바꾼 값이고, Abs 오류는 그대로 돌려준다. 플래그는 `-dir`(기본 `.`, "directory to search for .http files"), `-config`(기본 `defaultHistory`, 설명은 정확히 `history file path`), `-timeout`(`30*time.Second`, "per-request timeout, e.g. 10s or 2m"), `-version`이다.
- 위치 인자 사이의 플래그도 해석하도록 다음 루프로 처리한다.

```go
var positional []string
rest := args
for {
	if err := fs.Parse(rest); err != nil {
		return cliConfig{}, err
	}
	if fs.NArg() == 0 {
		break
	}
	positional = append(positional, fs.Arg(0))
	rest = fs.Args()[1:]
}
```

- 오류 문구는 정확히 다음과 같다. 위치 인자가 2개 이상이면 `expected at most one directory argument, got %d`, `-dir`가 명시되었고(`fs.Visit`로 확인) 위치 인자와 `filepath.Abs` 결과가 다르면 `-dir %s and argument %s point to different directories`, `timeout <= 0`이면 `-timeout must be positive, got %s`다.
- `fs.Usage`는 아래 문구를 출력한 뒤 `fs.PrintDefaults()`를 호출한다.

```text
rest-tui - browse and run IntelliJ HTTP Client (.http) files in the terminal

Usage:
  rest-tui [flags] [dir]

Examples:
  rest-tui                 scan the current directory
  rest-tui ./api           scan ./api
  rest-tui -timeout 2m .   allow slow requests

Press ? inside the app to see all keys.

Flags:
```

- `resolveVersion`: `linked != "dev"`이면 `linked`를 돌려준다. 그렇지 않고 `ok`이며 `info.Main.Version`이 `""`도 `"(devel)"`도 아니면 그 값을, 나머지 경우에는 `"dev"`를 돌려준다.
- `main()`: `flag.ErrHelp`면 종료 코드 0으로 끝낸다. 그 밖의 오류는 `rest-tui: <err>`를 stderr에 쓰고 종료 코드 2로 끝낸다. `-version`이면 `debug.ReadBuildInfo()` 결과로 `rest-tui <resolveVersion(...)>`를 출력한다. TUI는 `tui.NewApp(cfg.root, store, dirStore, tui.Options{Timeout: cfg.timeout})`로 시작한다.

- [ ] **Step 5: tui 구현** — Interfaces의 이름을 그대로 만든다. `sendCmd`는 `m.sendTimeout()`으로 `executor.Execute`를 부르고, 실패하면 `entry.Error = describeSendError(err, m.sendTimeout())`를 넣는다. `openRequestMsg`, `openRequestFromEntryMsg`, `rerunMsg` 세 분기는 모두 `openRequestView`를 거친다. `openRequestMsg` 분기의 생성 호출은 P2 그대로 `newRequestModel(msg.filePath, a.root, msg.file, msg.req, a.store, a.lastEnv)`다(root·lastEnv를 빼면 env 탐색 범위와 세션 env가 조용히 사라진다). Send 키 분기는 P2의 `executing`·`!canSend()`(→ `blockedFlash`) 검사를 그대로 두고 그 뒤에서만 `startSend`를 부른다.

- [ ] **Step 6: 통과 확인**

Run: `go test . && go test ./internal/tui/`
Expected: PASS

- [ ] **Step 7: 커밋**

```bash
git add main.go main_test.go internal/tui/app.go internal/tui/requestview.go internal/tui/cmd_helpers_test.go internal/tui/app_test.go internal/tui/layout_test.go internal/tui/requestview_test.go
git commit -m "cli: accept a positional dir, add -timeout, richer -h and go-install version"
```

---

### Task 4: Request view 응답 요약 줄·상태/메서드 색·리다이렉트 표시·`g`/`G`·스크롤 % (P4-6, P4-5 Request view, P4-4 표시)

**Files:**
- Modify: `internal/tui/styles.go` (`statusCodeStyle`, `methodStyle`)
- Modify: `internal/tui/keys.go` (`Top`, `Bottom`)
- Modify: `internal/tui/requestview.go` (`responseFromEntry`, `sendCmd`, `SetSize`, `buildContent`의 요청 줄, `statusLine`, `View`, `Update`)
- Modify: `internal/output/render.go` (`RenderResponse`)
- Modify: `internal/tui/app.go` (`footerHints`의 Request view slice, `helpLines`)
- Test: `internal/tui/requestview_test.go`, `internal/tui/styles_test.go`(신규), `internal/output/render_test.go`

**Interfaces:**
- Consumes: Task 1 `executor.Response.Redirects`, Task 2 `history.Entry.Redirects`, P1 `statusLine()`·`layoutScenarios`, P2 `colorWarning`
- Produces:
  - `func statusCodeStyle(code int) lipgloss.Style`. 2xx는 `colorSuccess`, 3xx는 `colorAccent`, 4xx는 `colorWarning`, 5xx와 `code <= 0`(전송 오류)은 `colorError`, 1xx는 `colorFaintText`다.
  - `func methodStyle(method string) lipgloss.Style`. 대소문자를 가리지 않는다. GET은 `colorSuccess`, POST는 `colorWarning`, PUT·PATCH는 `colorAccent`, DELETE는 `colorError`, 그 밖은 `colorFaintText`다.
  - `func formatSize(n int) string`. `n < 1024`이면 `"%d B"`, `n < 1024*1024`이면 `"%.1f KB"`, 그 이상은 `"%.1f MB"`다(1024 단위).
  - `func (m requestModel) summaryLine() string`. env 줄 바로 아래의 고정 1행이다. 응답 전에는 muted `not sent`, 응답이 있으면 `<Status> · <Duration.Round(ms)> · <formatSize(len(ResponseBody))>`(Status 부분만 `statusCodeStyle`), 오류면 `ERR · <entry.Error>`(오류색, 안쪽 폭으로 자름)다. Task 5에서 전송 중 문구가 추가된다.
  - `keys.Top`(`g`, `home`), `keys.Bottom`(`G`, `end`). 편집 중이 아닐 때 `viewport.GotoTop()` / `GotoBottom()`을 부른다.
  - `output.RenderResponse`는 `resp.Redirects`가 있으면 첫 섹션으로 `redirected: <hop>` 줄들(홉마다 1줄, `\n`으로 연결)을 넣는다. 기존 섹션은 그 뒤에 `\n\n`으로 이어진다. `responseFromEntry`는 `Redirects`를 복사하고, `sendCmd`는 `entry.Redirects = resp.Redirects`를 넣는다.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
func TestFormatSize(t *testing.T) {
	// 0 → "0 B", 512 → "512 B", 1229 → "1.2 KB", 1536 → "1.5 KB", 5*1024*1024 → "5.0 MB"
}

func TestStatusCodeStyleColors(t *testing.T) {
	// statusCodeStyle(200).GetForeground() == colorSuccess; 301 → colorAccent; 404 → colorWarning
	// 500 → colorError; 0 → colorError
}

func TestMethodStyleColors(t *testing.T) {
	// "GET"·"get" → colorSuccess, "POST" → colorWarning, "PUT"·"PATCH" → colorAccent, "DELETE" → colorError, "TRACE" → colorFaintText
}

func TestRequestModel_SummaryLineAfterResponse(t *testing.T) {
	m := newTestRequestModel(t, httpfile.Request{Method: "POST", URL: "http://x/users"})
	entry := history.Entry{StatusCode: 201, Status: "201 Created", Duration: 4*time.Millisecond + 300*time.Microsecond,
		ResponseBody: strings.Repeat("x", 1229)}
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: entry})
	// ansi.Strip(m.summaryLine()) == "201 Created · 4ms · 1.2 KB"
}

func TestRequestModel_SummaryLineBeforeSendAndOnError(t *testing.T) {
	// 새 모델: ansi.Strip(m.summaryLine()) == "not sent"
	// Entry{Error: "boom"} 수신 후: == "ERR · boom"
}

func TestRequestModel_ShowsRedirectsInResponse(t *testing.T) {
	// m := newTestRequestModel(t, …) → Entry{StatusCode: 200, Status: "200 OK", Redirects: []string{"302 → GET http://x/new"}} 수신
	// strings.Contains(ansi.Strip(m.viewport.View()), "redirected: 302 → GET http://x/new")
	// strings.Contains(ansi.Strip(renderEntryDetail(entry)), "redirected: 302 → GET http://x/new")
}

func TestRequestModel_TopBottomKeysAndScrollPercent(t *testing.T) {
	// SetSize(80, 20), 200줄 본문 응답 수신
	// "G" → m.viewport.AtBottom(), strings.HasSuffix(ansi.Strip(m.statusLine()), "100%")
	// "g" → m.viewport.YOffset == 0, HasSuffix(..., "0%")
	// tea.KeyEnd → AtBottom, tea.KeyHome → YOffset == 0
}

func TestRequestModel_StatusLineOmitsPercentWhenContentFits(t *testing.T) {
	// SetSize(120, 35), 짧은 응답 → !strings.Contains(ansi.Strip(m.statusLine()), "%")
}

```

`internal/tui/layout_test.go`의 `layoutScenarios`에 행 `request-summary-redirect`를 추가한다. 순서는 `newLayoutApp` → `applyMsg(openRequestMsg{…})` → `applyMsg(execResultMsg{modelID: a.request.id, entry: <200줄 본문, Redirects 2개(URL 200자)>})`다.

`internal/output/render_test.go`:

```go
func TestRenderResponse_ShowsRedirectsFirst(t *testing.T) {
	resp := &executor.Response{Status: "200 OK", Duration: 4 * time.Millisecond, Redirects: []string{"302 → GET http://x/new"}}
	// strings.HasPrefix(RenderResponse(resp, Options{}), "redirected: 302 → GET http://x/new\n\n200 OK (4ms)")
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestFormatSize|TestStatusCodeStyleColors|TestMethodStyleColors|TestRequestModel_(SummaryLine|ShowsRedirects|TopBottomKeys|StatusLineOmitsPercent)|TestApp_ViewFitsTerminal' && go test ./internal/output/ -run TestRenderResponse_ShowsRedirectsFirst`
Expected: FAIL. 아직 정의되지 않은 식별자 때문에 컴파일 오류가 난다.

- [ ] **Step 3: 구현**

- Interfaces의 함수와 키를 만든다. `View`는 env 줄, `summaryLine()`, viewport, `statusLine()` 순서로 쌓는다. 편집 모드에서도 요약 줄 자리를 유지해 높이 계산을 한 가지로 둔다. `SetSize`가 viewport·editor 높이에서 빼는 고정 줄 수에 1을 더한다.
- `buildContent`의 요청 줄(`METHOD URL`)에서 METHOD만 `methodStyle`로 칠한다. `copyText`와 `serializeRequest`는 색 없이 그대로 둔다.
- `statusLine()`의 스크롤 %: `m.viewport.TotalLineCount() > m.viewport.Height`일 때만 붙인다. `pct := fmt.Sprintf("%d%%", int(m.viewport.ScrollPercent()*100))`이다. 기존 메시지는 `ansi.Truncate(msg, w-len(pct)-1, "…")`로 자르고, `pct`가 안쪽 폭 `w`의 오른쪽 끝에 오도록 공백으로 채운다.
- footer Request view slice에 `hint{"g/G", "top/bottom"}`을 추가한다. `helpLines()`의 Request view 단락에는 `"  g/G, home/end top / bottom of response"`를 추가한다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/tui/ ./internal/output/`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/tui/styles.go internal/tui/styles_test.go internal/tui/keys.go internal/tui/requestview.go internal/tui/requestview_test.go internal/tui/layout_test.go internal/tui/app.go internal/output/render.go internal/output/render_test.go
git commit -m "tui: add response summary line, status/method colors, redirect trail and g/G scrolling"
```

---

### Task 5: 전송 중 spinner·경과 시간·`esc` 취소·`q` 두 번 종료 (P4-1, P4-2)

**Files:**
- Modify: `internal/tui/requestview.go` (`requestModel` 필드, 두 생성자(`newRequestModel`, `newRequestModelFromEntryNoSend`)의 spinner 초기화, `sendCmd`, `startSend`, `Update`, `summaryLine`, `describeSendError`)
- Modify: `internal/tui/app.go` (`App.quitArmed`, 키 스위치의 `keys.Quit` 분기, `execResultMsg`·`spinner.TickMsg` 라우팅, `footerHints`, `helpLines`)
- Test: `internal/tui/requestview_test.go`, `internal/tui/app_test.go`

**Interfaces:**
- Consumes: Task 1 오류 분류, Task 3 `startSend`·`describeSendError`·`awaitMsg`, Task 4 `summaryLine`, P1 `execResultMsg.modelID`
- Produces:
  - 필드 `requestModel.spinner spinner.Model`(`spinner.New(spinner.WithSpinner(spinner.MiniDot))`), `requestModel.sendStarted time.Time`, `requestModel.cancel context.CancelFunc`
  - `func (m *requestModel) sendCmd() tea.Cmd`(포인터 리시버로 바뀜), `func (m requestModel) isSending() bool`
  - `describeSendError`에 `errors.Is(err, context.Canceled)` → `"canceled"` 분기를 추가한다(타임아웃 분기보다 먼저).
  - `App.quitArmed bool`, `const quitConfirmMsg = "request in flight — press q again to quit"`

**cancel 함수 보관 규칙 (값 모델에서 안전하게):**
- `sendCmd`는 포인터 리시버다. P1의 `id` 캡처(`execResultMsg.modelID`), P2의 `Env` 기록, Task 3의 `sendTimeout`·`describeSendError`는 그대로 둔다. 안에서 `ctx, cancel := context.WithCancel(context.Background())`를 만들고 `m.cancel = cancel`로 보관한다. 반환하는 클로저는 `ctx`와 `cancel`을 캡처하고 `defer cancel()`로 자원을 푼다.
- `sendCmd`는 `startSend`(값 리시버) 안에서 그 지역 복사본에 대해서만 호출된다. 호출은 `cmd := tea.Batch(m.sendCmd(), m.spinner.Tick)`처럼 별도 문장으로 하고 그다음 `return m, cmd`로 돌려준다. `return m, tea.Batch(m.sendCmd(), …)`는 Go가 `m`을 읽는 시점과 호출 순서를 정하지 않아 cancel이 빠진 복사본이 반환될 수 있다. 그리고 `startSend`가 그 복사본을 돌려주고, `Update`가 그대로 반환한다. 값 리시버 메서드 안에서 보관하고 반환하지 않으면 대입이 사라진다. 그러면 `esc`가 아무것도 취소하지 못하는데 컴파일도 테스트도 그것을 잡지 못한다. 그래서 `TestRequestModel_SendStoresCancelOnReturnedModel`로 고정한다.
- `context.CancelFunc`는 func 값이다. Bubble Tea가 모델을 값으로 복사해도 모든 복사본이 같은 클로저를 가리키고, 여러 번 호출해도 되며 goroutine에 안전하다(표준 라이브러리 보장). 그래서 별도 포인터나 mutex를 두지 않는다. 패키지 변수나 App 쪽 맵에도 두지 않는다.
- `esc`(전송 중)는 `m.cancel()`만 부르고 `executing`을 바꾸지 않는다. `executing`은 같은 `modelID`의 `execResultMsg`가 도착했을 때만 `false`가 되고, 이때 `m.cancel = nil`로 비운다. 그래서 취소된 결과가 도착하기 전에 새 전송이 시작될 수 없다(Send는 `executing` 동안 막혀 있다).
- 다른 요청을 열어 `a.request`가 바뀌면 이전 cancel은 잃는다. 그래도 진행 중인 클로저의 `defer cancel()`이 컨텍스트를 풀고, 결과는 P1-1대로 히스토리에만 남는다.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
func TestDescribeSendError_Canceled(t *testing.T) {
	// describeSendError(fmt.Errorf("x: %w", context.Canceled), 30*time.Second) == "canceled"
}

func TestRequestModel_SendStoresCancelOnReturnedModel(t *testing.T) {
	// srv: 200
	m := newTestRequestModel(t, httpfile.Request{Method: "GET", URL: srv.URL})
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// m.isSending(), m.cancel != nil, !m.sendStarted.IsZero()
	_ = awaitMsg[spinner.TickMsg](t, cmd)
}

func TestRequestModel_SpinnerTickStopsWhenNotSending(t *testing.T) {
	m := newTestRequestModel(t, req)
	m.executing = true
	_, cmd := m.Update(spinner.TickMsg{ID: m.spinner.ID()})
	// cmd != nil
	m.executing = false
	_, cmd = m.Update(spinner.TickMsg{ID: m.spinner.ID()})
	// cmd == nil
}

func TestRequestModel_SummaryLineWhileSendingShowsElapsed(t *testing.T) {
	m.executing = true
	m.sendStarted = time.Now().Add(-2500 * time.Millisecond)
	// regexp.MustCompile(`sending… 2\.[5-9]s · esc cancel$`).MatchString(ansi.Strip(m.summaryLine()))
}

func TestRequestModel_EscWhileSendingCancelsAndRecordsCanceled(t *testing.T) {
	// srv 핸들러: <-r.Context().Done() 또는 5s
	store := newTestHistoryStore(t)
	m := newRequestModel("", "", &httpfile.File{}, httpfile.Request{Method: "GET", URL: srv.URL}, store, "").SetSize(120, 35)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m, back := m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // send cmd를 실행하기 전에 취소: 같은 cancel을 공유하는지 확인
	// back == nil (뒤로 가지 않음), m.isSending() 여전히 true
	msg := awaitMsg[execResultMsg](t, cmd)
	// msg.entry.Error == "canceled"; store.List(1)[0].Error == "canceled"
	m, _ = m.Update(msg)
	// !m.isSending(), m.cancel == nil, ansi.Strip(m.summaryLine()) == "ERR · canceled"
}

func TestApp_QuitWhileSendingNeedsSecondQ(t *testing.T) {
	a := newTestApp(t) // openRequestMsg로 Request view를 연 뒤 a.request.executing = true
	model, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	// cmd == nil, strings.Contains(model.(App).View(), "request in flight — press q again to quit")
	_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	// cmd() == tea.Quit()
}

func TestApp_OtherKeyDisarmsQuit(t *testing.T) {
	// q → j → q 를 누르면 두 번째 q 뒤에도 cmd == nil(다시 확인 상태), View에 quitConfirmMsg가 있다
}

func TestApp_CtrlCQuitsEvenWhileSending(t *testing.T) {
	// executing = true 상태에서 tea.KeyCtrlC → cmd() == tea.Quit()
}

func TestApp_ResultArrivesWhileOnOtherScreen(t *testing.T) {
	// Request view를 열고 executing = true → "h"로 History로 이동
	// a.Update(spinner.TickMsg{ID: a.request.spinner.ID()})의 cmd != nil (screen과 무관하게 a.request로 전달)
	// a.Update(execResultMsg{modelID: a.request.id, entry: history.Entry{StatusCode: 200}}) → !a.request.isSending()
	// 그다음 "q" 한 번 → cmd() == tea.Quit()
}
```

`layoutScenarios`에 행 `request-sending-quit-confirm`을 추가한다. 순서는 Request view 열기 → `a.request.executing = true` → `pressKeys(a, "q")`다. 상태 줄에 `quitConfirmMsg`가 있어도 화면을 넘지 않아야 한다.

Send 키나 rerun의 cmd 결과를 직접 꺼내는 기존 테스트는 Batch가 되면서 깨진다. `grep -n 'cmd()\.(execResultMsg)\|applyMsg(a, [a-zA-Z]*[cC]md())' internal/tui/*_test.go`로 찾아 `awaitMsg[execResultMsg](t, cmd)`(App 테스트는 `applyMsg(a, awaitMsg[execResultMsg](t, cmd))`)로 바꾼다. P1의 `applyMsg(a, sendCmd())`, P2의 `cmd().(execResultMsg)`, P3의 `res := cmd().(execResultMsg)`가 대상이다.

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestDescribeSendError_Canceled|TestRequestModel_(SendStoresCancel|SpinnerTickStops|SummaryLineWhileSending|EscWhileSending)|TestApp_(QuitWhileSending|OtherKeyDisarmsQuit|CtrlCQuitsEvenWhileSending|ResultArrivesWhileOnOtherScreen|ViewFitsTerminal)'`
Expected: FAIL. `spinner`·`isSending`·`cancel` 필드가 없어 컴파일 오류가 난다.

- [ ] **Step 3: requestModel 구현**

- `startSend`는 `m.sendStarted = time.Now()`를 기록하고 위 보관 규칙대로 `tea.Batch(m.sendCmd(), m.spinner.Tick)`을 돌려준다.
- `Update`: `spinner.TickMsg`는 `executing`일 때만 `m.spinner.Update`로 넘기고, 아니면 `nil`을 돌려주어 tick 사슬을 끊는다. `keys.Back`은 `executing`이면 `m.cancel`(nil이 아니면)을 부르고 `return m, nil`이다. 그렇지 않으면 기존대로 뒤로 간다.
- `summaryLine`: `executing`이면 `<spinner.View()> sending… <time.Since(sendStarted).Truncate(100ms)> · esc cancel`이다.

- [ ] **Step 4: App 구현**

- `case execResultMsg`와 `case spinner.TickMsg`는 `a.screen`과 관계없이 `a.request.Update`로 넘긴다(P1이 `execResultMsg`를 이미 이렇게 라우팅하면 tick만 추가한다). 넘긴 뒤 `!a.request.isSending()`이면 `quitArmed = false`로 두고, `statusMsg == quitConfirmMsg`면 비운다.
- `func (a App) handleQuit(msg tea.KeyMsg) (App, tea.Cmd)`: `msg.String() == "q"`이고 `a.request.isSending()`인데 `!a.quitArmed`이면 `quitArmed = true`, `statusMsg = quitConfirmMsg`로 두고 `resize()` 후 `nil`을 돌려준다. 그 밖(`ctrl+c`, 두 번째 `q`, 전송 중이 아님)이면 `tea.Quit`이다. 전역 키 스위치의 `keys.Quit` 분기와 P1 `updateHelp`의 `keys.Quit` 분기가 모두 이것을 부른다. `q`가 아닌 다른 키를 누르면 `quitArmed = false`가 되고, `statusMsg == quitConfirmMsg`면 비운 뒤 `resize()`한다. 이 해제는 `App.Update`의 `tea.KeyMsg` 처리 맨 앞(P1 도움말 분기보다 먼저)에서 한다.
- footer: Request view가 전송 중이면 `hint{"esc", "back"}` 자리에 `hint{"esc", "cancel"}`을 둔다. `helpLines()`의 Global `q` 줄은 `"  q, ctrl+c     quit (q twice while a request is in flight)"`, Request view에 `"  esc           cancel send (while sending) / back"`이다.

- [ ] **Step 5: 통과 확인**

Run: `go test -race ./internal/tui/`
Expected: PASS

- [ ] **Step 6: 커밋**

```bash
git add internal/tui/requestview.go internal/tui/requestview_test.go internal/tui/app.go internal/tui/app_test.go internal/tui/layout_test.go
git commit -m "tui: show spinner and elapsed time while sending, esc cancels, q asks twice"
```

---

### Task 6: 출발 화면으로 돌아가기·breadcrumb·rerun 확인 (P4-8, P4-9)

**Files:**
- Modify: `internal/tui/messages.go` (`backFromRequestMsg`)
- Modify: `internal/tui/requestview.go` (`Update`의 `keys.Back` → `backFromRequestMsg`)
- Modify: `internal/tui/app.go` (`App.requestOrigin`, `openRequestView` 시그니처, `backFromRequestMsg` 분기, `breadcrumb`, `footerHints`, `helpLines`)
- Modify: `internal/tui/historyview.go` (`historyModel.pendingRerun`, `refresh`, `Update`의 `keys.Rerun`, `View`), `internal/tui/keys.go` (`Confirm`)
- Test: `internal/tui/requestview_test.go`(`TestRequestModel_BackEmitsBackToBrowserMsg`를 아래 `TestRequestModel_BackEmitsBackFromRequestMsg`로 이름과 기대 타입을 바꿈. `TestRequestModel_EscCancelsEditWithoutMutatingResolved`의 "편집 중 esc는 뒤로 가지 않는다" 단언도 `backFromRequestMsg`로 바꿈. 바꾸지 않으면 그 단언은 항상 통과한다), `internal/tui/app_test.go`, `internal/tui/historyview_test.go`

**Interfaces:**
- Consumes: Task 3 `openRequestView`, `startSend`; Task 5 `isSending`
- Produces:
  - `type backFromRequestMsg struct{}`. Request view는 이제 `backToBrowserMsg` 대신 이것을 낸다. History와 Directories의 `backToBrowserMsg`는 그대로다.
  - `App.requestOrigin screen`, `func (a App) openRequestView(m requestModel, origin screen) App`. `openRequestMsg`·`openRequestFromEntryMsg`는 `screenBrowser`, `rerunMsg`는 `screenHistory`로 연다.
  - `backFromRequestMsg` 분기: 출발 화면이 History면 `a.screen = screenHistory`로 두고 `a.history = a.history.refresh()`를 한다. 그 밖은 기존 `backToBrowserMsg` 처리와 같다.
  - `breadcrumb()`: Request view의 출발 화면이 History면 정확히 `rest-tui › History › rerun`이다.
  - `historyModel.pendingRerun *history.Entry`, `func isSafeMethod(method string) bool`(GET·HEAD·OPTIONS, 대소문자 무시), `keys.Confirm`(`y`)
  - `historyModel.refresh()`는 새로 읽기 전에 선택된 항목의 ID를 기억했다가, 같은 ID를 찾으면 `list.Select`로 커서를 되돌린다. 없으면 0이다. `pendingRerun`은 비운다.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
func TestRequestModel_BackEmitsBackFromRequestMsg(t *testing.T) {
	// esc → cmd().(backFromRequestMsg) ok
}

func TestApp_EscFromRerunReturnsToHistory(t *testing.T) {
	a := newTestApp(t)
	model, _ := a.Update(rerunMsg{entry: history.Entry{Method: "GET", URL: "http://127.0.0.1:1/x"}})
	a = model.(App)
	// a.breadcrumb() == "rest-tui › History › rerun"
	model, _ = a.Update(execResultMsg{modelID: a.request.id, entry: history.Entry{Error: "refused"}})
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model, _ = model.Update(cmd())
	// model.(App).screen == screenHistory, model.(App).breadcrumb() == "rest-tui › History"
}

func TestApp_EscFromBrowserOpenedRequestReturnsToBrowser(t *testing.T) {
	// openRequestMsg → esc → cmd() 처리 → screen == screenBrowser
}

func TestIsSafeMethod(t *testing.T) {
	// "GET", "head", "OPTIONS" → true; "POST", "PUT", "PATCH", "DELETE" → false
}

func TestHistoryModel_RerunUnsafeMethodAsksConfirmation(t *testing.T) {
	// store에 POST https://example.com/users 1건 → refresh → SetSize(120, 20)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	// cmd == nil, strings.Contains(ansi.Strip(m.View()), "rerun POST https://example.com/users? (y/n)")
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	// cmd().(rerunMsg).entry.URL == "https://example.com/users"
}

func TestHistoryModel_RerunConfirmationOtherKeyCancels(t *testing.T) {
	// r → n: cmd == nil, m.pendingRerun == nil, View에 "(y/n)" 없음
}

func TestHistoryModel_RefreshKeepsCursorOnSameEntry(t *testing.T) {
	// A(GET /a), B(GET /b) 저장 → refresh(목록 B, A) → SetSize(120, 20) → 커서를 A(index 1)로
	// store.Append(C) → m = m.refresh()
	// m.list.SelectedItem().(entryItem).entry.ID == A.ID
}

```

`layoutScenarios`에 행 `history-rerun-confirm`을 추가한다. URL이 300자인 POST 항목을 저장하고 `pressKeys(a, "h", "r")`를 누른 상태다.

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestRequestModel_BackEmitsBackFromRequestMsg|TestApp_EscFrom|TestIsSafeMethod|TestHistoryModel_(RerunUnsafe|RerunConfirmation|RefreshKeepsCursor)|TestApp_ViewFitsTerminal'`
Expected: FAIL. `backFromRequestMsg`·`isSafeMethod`·`pendingRerun`이 없어 컴파일 오류가 난다.

- [ ] **Step 3: 구현**

- `historyModel.Update`: `pendingRerun != nil`이면 그 키를 먼저 소비한다. `keys.Confirm`이면 `rerunMsg`를 내고, 다른 키면 확인만 취소한다. 둘 다 `pendingRerun`을 비운다. `keys.Rerun`은 안전한 메서드면 바로 `rerunMsg`를 내고, 아니면 `pendingRerun`에 둔다. App의 전역 키(`q`, `?`, `h`, `d`)는 지금처럼 먼저 처리된다. `h`는 `refresh`를 거치므로 확인도 함께 사라진다.
- 확인 문구는 `fmt.Sprintf("rerun %s %s? (y/n)", method, url)`이고, 목록·상세 아래 한 줄에 둔다. 확인 중에는 목록(상세 모드면 detail viewport) 높이를 1줄 줄인다. 안쪽 폭 `w`를 넘으면 URL만 `ansi.Truncate(url, w-lipgloss.Width("rerun "+method+" ")-len("? (y/n)"), "…")`로 줄여 `? (y/n)`이 항상 보이게 한다.
- footer History slice: 확인 중이면 `hint{"y", "rerun"}, hint{"n", "cancel"}`이다. `helpLines()` History의 `r` 줄은 `"  r             rerun (asks y/n unless GET/HEAD/OPTIONS)"`이다. `helpLines()` Request view의 `esc` 줄(Task 5)은 `"  esc           cancel send (while sending) / back to Browser or History"`로 바꾼다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/tui/messages.go internal/tui/requestview.go internal/tui/requestview_test.go internal/tui/app.go internal/tui/app_test.go internal/tui/layout_test.go internal/tui/historyview.go internal/tui/historyview_test.go internal/tui/keys.go
git commit -m "tui: return to the screen a request was opened from and confirm unsafe reruns"
```

---

### Task 7: History·Recent 한 줄 행 형식과 색 (P4-10, P4-5 목록)

**Files:**
- Modify: `internal/tui/historyview.go` (`formatEntryRow`, `relativeTime`, `entryRowDelegate`, `newHistoryModel`)
- Modify: `internal/tui/browser.go` (`refreshRecent`, `selectFile`의 delegate 교체)
- Modify: `internal/tui/requestview.go` (`sendCmd`가 `entry.Name = m.req.Name`, `newRequestModelFromEntryNoSend`가 `req.Name = entry.Name`)
- Test: `internal/tui/historyview_test.go`, `internal/tui/browser_test.go`

**Interfaces:**
- Consumes: Task 2 `history.Entry.Name`, P2 `history.Entry.Env`, Task 4 `statusCodeStyle`, `methodStyle`
- Produces:
  - `func relativeTime(t, now time.Time) string`. 1분 미만은 `just now`, 1시간 미만은 `%dm ago`, 24시간 미만은 `%dh ago`, 그 밖은 `%dd ago`다.
  - `func formatEntryRow(e history.Entry, now time.Time, width int) string`. 조립 순서는 다음과 같다.
    1. 상태 칸: 폭 3이고 `statusCodeStyle`로 칠한다. 오류면 `ERR`, 아니면 `%3d`다.
    2. 공백 1칸, 메서드(`%-7s`, `methodStyle`), 공백 1칸을 붙인다.
    3. env가 있으면 `env + " "`를 붙인다.
    4. 이름(`e.Name`, 비었으면 `e.URL`)을 붙인다. 폭 = `width - 앞부분 폭 - 2 - lipgloss.Width(rel)`이고, 넘으면 `ansi.Truncate(…, 폭, "…")`로 자른다. 폭이 1 미만이면 이름을 생략한다.
    5. 공백 2칸과 `rel`을 붙인다.
    6. 마지막으로 `ansi.Truncate(row, width, "")`로 폭을 보장한다. 상태 칸이 맨 앞이므로 잘리지 않는다.
  - `type entryRowDelegate struct{}`(`list.ItemDelegate`). `Height() == 1`, `Spacing() == 0`, `Update`는 `nil`이다. `Render`는 선택 항목이면 `"> "`, 아니면 `"  "`를 앞에 붙이고 `formatEntryRow(item.entry, time.Now(), m.Width()-2)`를 쓴다. 필터 일치 강조는 하지 않는다. 기본 delegate의 rune 단위 강조는 ANSI가 섞인 제목을 깨뜨린다.
  - History 목록과 Browser Recent 목록이 이 delegate를 쓴다. `refreshRecent`는 `m.requests.SetDelegate(entryRowDelegate{})`를 하고, `selectFile`은 기존 기본 delegate로 되돌린다.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
func TestRelativeTime(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	// now-30s → "just now", now-3m → "3m ago", now-5h → "5h ago", now-49h → "2d ago"
}

func TestFormatEntryRow_Layout(t *testing.T) {
	e := history.Entry{StatusCode: 201, Method: "POST", Env: "dev", Name: "Create user", Timestamp: now.Add(-3 * time.Minute)}
	// ansi.Strip(formatEntryRow(e, now, 80)) == "201 POST    dev Create user  3m ago"
}

func TestFormatEntryRow_ErrorFallsBackToURL(t *testing.T) {
	e := history.Entry{Error: "canceled", Method: "GET", URL: "https://example.com/x", Timestamp: now}
	// ansi.Strip(formatEntryRow(e, now, 80)) == "ERR GET     https://example.com/x  just now"
}

func TestFormatEntryRow_TruncatesNameNotStatus(t *testing.T) {
	e := history.Entry{StatusCode: 200, Method: "GET", URL: "https://example.com/" + strings.Repeat("a", 100), Timestamp: now}
	row := ansi.Strip(formatEntryRow(e, now, 30))
	// lipgloss.Width(row) <= 30, strings.HasPrefix(row, "200 GET     "), strings.HasSuffix(row, "just now"), strings.Contains(row, "…")
}

func TestEntryRowDelegate_RendersOneLineWithCursor(t *testing.T) {
	// d := entryRowDelegate{}; d.Height() == 1, d.Spacing() == 0
	// l := list.New(items(2개), d, 60, 10); var b bytes.Buffer; d.Render(&b, l, 0, items[0])
	// strings.HasPrefix(ansi.Strip(b.String()), "> 200"); index 1은 "  " 로 시작
}

func TestBrowserModel_RecentUsesEntryRows(t *testing.T) {
	// store에 GET https://example.com/a (200) 1건 → newBrowserModel(dir, store).SetSize(120, 20)
	// strings.Contains(ansi.Strip(m.View()), "200 GET")
}

```

`layoutScenarios`에 행 `history-rows-long-url`을 추가한다. Name이 없고 URL이 300자인 항목 3건(200, 500, 오류)을 저장하고 `pressKeys(a, "h")`를 누른 상태다.

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestRelativeTime|TestFormatEntryRow|TestEntryRowDelegate|TestBrowserModel_RecentUsesEntryRows|TestApp_ViewFitsTerminal'`
Expected: FAIL. `formatEntryRow` 등이 정의되지 않아 컴파일 오류가 난다.

- [ ] **Step 3: 구현** — Interfaces의 조립 순서와 delegate 교체를 그대로 만든다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/tui/historyview.go internal/tui/historyview_test.go internal/tui/browser.go internal/tui/browser_test.go internal/tui/layout_test.go internal/tui/requestview.go
git commit -m "tui: one-line history rows with colored fixed-width status, method, env, name and age"
```

---

### Task 8: 모든 목록에서 `/` 필터, 파일 항목 한 줄 (P4-7)

**Files:**
- Modify: `internal/tui/browser.go` (`newBrowserModel`, `requestItem.FilterValue`, `Update`, `isFiltering`)
- Modify: `internal/tui/historyview.go` (`newHistoryModel`, `entryItem.FilterValue`, `Update`, `isFiltering`)
- Modify: `internal/tui/dirhistoryview.go` (`newDirHistoryModel`, `Update`, `isFiltering`)
- Modify: `internal/tui/app.go` (`activeScreenIsEditing`, `footerHints`, `helpLines`)
- Test: `internal/tui/browser_test.go`, `internal/tui/historyview_test.go`, `internal/tui/dirhistoryview_test.go`, `internal/tui/app_test.go`

**Interfaces:**
- Consumes: Task 7 `entryRowDelegate`
- Produces:
  - `func listCapturesKey(l list.Model, msg tea.KeyMsg) bool`(`browser.go`). `l.FilterState() == list.Filtering`이면 모든 키에 참이고, `FilterApplied`이면 `esc`에만 참이다. 세 화면의 `Update`는 맨 앞에서 이것을 확인하고, 참이면 키를 목록에만 넘긴다(Tab·Back·Enter·Rerun 분기를 거치지 않음).
  - `func (m browserModel) isFiltering() bool`(포커스 목록), `func (m historyModel) isFiltering() bool`(목록 모드), `func (m dirHistoryModel) isFiltering() bool`. 모두 `FilterState() == list.Filtering`이다. `App.activeScreenIsEditing`이 활성 화면의 이 값을 OR로 더한다.
  - 네 목록 모두 `SetFilteringEnabled(true)`이다. Files 목록 delegate는 `ShowDescription = false`, `SetSpacing(0)`이다.
  - `requestItem.FilterValue()`는 `Name + " " + Method + " " + URL`, `entryItem.FilterValue()`는 `Method + " " + Name + " " + URL + " " + Env`다.
  - 목록 내용을 바꾸는 곳(`historyModel.refresh`, `dirHistoryModel.refresh`, `browserModel.refreshRecent`, `browserModel.selectFile`)은 `SetItems` 전에 `ResetFilter()`를 부른다. 필터가 걸린 채 `SetItems`를 부르면 bubbles `list`가 걸러진 목록을 비우고 다시 거르는 cmd를 돌려주는데, 이 함수들은 cmd를 버리므로 목록이 빈 채로 남는다. `historyModel.refresh`의 커서 복원(Task 6)은 필터를 푼 뒤의 `Items()` index로 한다.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
func TestBrowserModel_FilesRenderOneLineEach(t *testing.T) {
	// a.http, b.http, c.http → SetSize(120, 20)
	// m.files.View()의 줄 목록에서 세 이름이 있는 줄 번호가 연속(차이 1)
}

func TestBrowserModel_FilterNarrowsFiles(t *testing.T) {
	// a.http, users.http, orders.http → m.files.SetFilterText("users") → len(m.files.VisibleItems()) == 1
}

func TestBrowserModel_EnterWhileFilteringAppliesFilterNotOpen(t *testing.T) {
	// "/" → "u" → m.files.FilterState() == list.Filtering → Enter
	// m.files.FilterState() == list.FilterApplied, m.parsedFile == nil, m.focus == paneFiles
}

func TestRequestItemFilterValueIncludesMethodAndURL(t *testing.T) {
	// requestItem{req: httpfile.Request{Method: "GET", URL: "https://x/users"}}.FilterValue()에 "GET"과 "users"가 있다
}

func TestApp_FilterTypingSwallowsGlobalKeys(t *testing.T) {
	// root에 a.http → a := newLayoutApp(t, root, newTestHistoryStore(t), 120, 35)
	// "/" 다음 "q", "h", "d", "?", "R", "o"를 차례로 누른다
	// a.screen == screenBrowser, !a.showHelp, a.browser.files.FilterValue() == "qhd?Ro"
	// tea.KeyEsc → a.browser.files.FilterState() == list.Unfiltered, a.screen == screenBrowser
}

func TestHistoryModel_FilterThenEnterOpensFilteredEntry(t *testing.T) {
	// /users, /orders 2건 → m.list.SetFilterText("orders") → Enter → m.selected.URL가 "/orders"로 끝남
}

func TestHistoryModel_EscWithFilterAppliedClearsFilterFirst(t *testing.T) {
	// SetFilterText("orders") → esc → m.list.FilterState() == list.Unfiltered, cmd가 nil이 아니면 cmd()는 backToBrowserMsg가 아니다
}

func TestHistoryModel_RefreshWithFilterAppliedShowsAllAndKeepsCursor(t *testing.T) {
	// /users, /orders 2건 → SetSize(120, 20) → SetFilterText("orders") → store.Append(/new) → m = m.refresh()
	// m.list.FilterState() == list.Unfiltered, len(m.list.VisibleItems()) == 3
	// m.list.SelectedItem().(entryItem).entry.URL가 "/orders"로 끝남
}

func TestBrowserModel_SelectFileAfterRecentFilterShowsRequests(t *testing.T) {
	// 히스토리 1건(Recent 표시), a.http(요청 2개) → Tab(Recent) → m.requests.SetFilterText("x") → Tab(Files) → Enter
	// len(m.requests.VisibleItems()) == 2
}

func TestDirHistoryModel_FilterNarrowsDirs(t *testing.T) {
	// Touch 두 경로 → refresh → SetFilterText(<한쪽 base>) → len(VisibleItems()) == 1
}
```

`?`·`R`·`o`가 아직 전역 키가 아니어도 이 테스트는 의미가 있다. Task 10과 P1-3 이후에도 필터 입력으로 들어가는지 고정한다.

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestBrowserModel_(FilesRenderOneLineEach|FilterNarrowsFiles|EnterWhileFiltering|SelectFileAfterRecentFilter)|TestRequestItemFilterValue|TestApp_FilterTypingSwallowsGlobalKeys|TestHistoryModel_(FilterThenEnter|EscWithFilterApplied|RefreshWithFilterApplied)|TestDirHistoryModel_FilterNarrowsDirs'`
Expected: FAIL. 필터가 꺼져 있어 `VisibleItems`가 줄지 않고, `/`가 Filtering 상태로 들어가지 않는다. 파일 항목도 3행이다.

- [ ] **Step 3: 구현** — Interfaces대로 만든다. footer의 Browser·History 목록·Directories slice에 `hint{"/", "filter"}`를 추가한다. `helpLines()`에는 Browser·History·Directory History 단락마다 `"  /             filter list (esc clears)"`를 추가한다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/tui/browser.go internal/tui/browser_test.go internal/tui/historyview.go internal/tui/historyview_test.go internal/tui/dirhistoryview.go internal/tui/dirhistoryview_test.go internal/tui/app.go internal/tui/app_test.go
git commit -m "tui: enable / filtering in every list and render file items on one line"
```

---

### Task 9: 디렉터리 스캔 견고성·디렉터리 이력 기록 조건·빈 상태 안내 (P4-12, P4-15)

**Files:**
- Modify: `internal/tui/browser.go` (`discoverHTTPFiles`, `newBrowserModel`, `browserModel.scanNotice`, `Update`의 `keys.Tab`, `View`)
- Modify: `internal/tui/app.go` (`NewApp`, `switchDirMsg` 분기, `replaceBrowser`, `isDir`)
- Modify: `internal/tui/cmd_helpers_test.go` (`skipIfNoPermEnforcement`)
- Test: `internal/tui/browser_test.go`(`TestDiscoverHTTPFiles_FindsHttpFilesRecursively`를 반환값 3개로 수정), `internal/tui/app_test.go`

**Interfaces:**
- Consumes: Task 3 `NewApp`/`Options`, P1 `layoutScenarios`·`resize()`·`fitLines`
- Produces:
  - `func discoverHTTPFiles(root string) (rels []string, skipped int, err error)`. `WalkDir` 콜백에서 `err != nil`이면 `path == root`일 때 그 오류를 돌려주고, 아니면 `skipped++` 후 `filepath.SkipDir`(디렉터리가 아닌 항목이면 `nil`)를 돌려준다.
  - `browserModel.scanNotice string`. `skipped > 0`이면 `fmt.Sprintf("skipped %d unreadable dirs", skipped)`, 아니면 `""`다.
  - `func (a App) replaceBrowser(b browserModel) App`. `a.browser.scanNotice != "" && a.statusMsg == a.browser.scanNotice`이면 `statusMsg`를 비우고, `a.browser = b`로 바꾼 뒤 `b.scanNotice != ""`이면 `statusMsg`에 넣는다. 마지막에 항상 `resize()`한다(`statusMsg` 유무가 `contentHeight()`를 바꾸고, 새 Browser도 크기를 받아야 한다). `NewApp`, `switchDirMsg`, Task 10의 재스캔이 모두 이것으로 Browser를 바꾼다.
  - `func isDir(path string) bool`(`os.Stat` 성공 && `IsDir()`). `NewApp`과 `switchDirMsg`는 `isDir`이 참일 때만 `dirStore.Touch`를 부른다.
  - 빈 상태 문구(정확히): Files 패널은 `rootErr == nil && len(files.Items()) == 0`이면 `No .http files under <root> — d recent dirs · o open path`이다. Requests 패널은 `parsedFile == nil && parseErr == nil && len(requests.Items()) == 0`이면 `Select a file and press enter`이다. 둘 다 패널 제목은 남기고 목록 자리에 muted로 둔다. 패널 안쪽 폭·높이에 맞게 P1 `fitLines`로 감싼다.
  - `keys.Tab`: Files에서 Requests로는 `len(m.requests.Items()) > 0`일 때만 옮긴다(`parsedFile != nil` 조건은 뺀다).

- [ ] **Step 1: 실패하는 테스트 작성**

```go
func TestDiscoverHTTPFiles_SkipsUnreadableSubdirs(t *testing.T) {
	skipIfNoPermEnforcement(t)
	// root/ok/a.http, root/locked(chmod 000, t.Cleanup에서 0o755 복구)
	rels, skipped, err := discoverHTTPFiles(root)
	// err == nil, rels == []string{filepath.Join("ok", "a.http")}, skipped == 1
}

func TestDiscoverHTTPFiles_UnreadableOrMissingRootIsError(t *testing.T) {
	// 없는 경로 → err != nil; chmod 000 루트(skipIfNoPermEnforcement) → err != nil
}

func TestNewBrowserModel_ScanNoticeCountsSkippedDirs(t *testing.T) {
	// 위와 같은 구조 → newBrowserModel(root, store).scanNotice == "skipped 1 unreadable dirs"
}

func TestApp_ShowsScanNotice(t *testing.T) {
	// newLayoutApp(t, root(locked 하위 폴더), store, 120, 35) → strings.Contains(ansi.Strip(a.View()), "skipped 1 unreadable dirs")
}

func TestApp_DoesNotRecordMissingOrNonDirRoot(t *testing.T) {
	// NewApp(filepath.Join(t.TempDir(), "nope"), ...) 그리고 NewApp(<파일 경로>, ...) → dirStore.List(0) 길이 0
}

func TestApp_SwitchDirMsgDoesNotRecordMissingPath(t *testing.T) {
	// newTestApp → switchDirMsg{path: <없는 경로>} → dirStore.List(0)에 그 경로 없음
}

func TestBrowserModel_EmptyRootShowsGuidance(t *testing.T) {
	m := newBrowserModel(root, store).SetSize(600, 20)
	// strings.Contains(ansi.Strip(m.View()), "No .http files under "+root+" — d recent dirs · o open path")
}

func TestBrowserModel_RequestsPaneHintBeforeFileChosen(t *testing.T) {
	// a.http 하나, 히스토리 없음 → SetSize(120, 20) → View에 "Select a file and press enter"
}

func TestBrowserModel_TabStaysOnFilesWhenRequestsEmpty(t *testing.T) {
	// 빈 empty.http 선택(Enter) → esc로 Files → Tab → m.focus == paneFiles
}

```

`layoutScenarios`에 행 `browser-skipped-dirs`(locked 하위 폴더가 있는 루트, 권한을 강제하지 않는 환경에서는 `skipIfNoPermEnforcement`)를 추가한다. 빈 루트의 안내 문구는 P1의 `browser-empty` 행이 이미 같은 상태(빈 `t.TempDir()` 루트)를 모든 크기에서 검사하므로 행을 따로 만들지 않는다.

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestDiscoverHTTPFiles|TestNewBrowserModel_ScanNotice|TestApp_(ShowsScanNotice|DoesNotRecordMissing|SwitchDirMsgDoesNotRecord|ViewFitsTerminal)|TestBrowserModel_(EmptyRootShowsGuidance|RequestsPaneHint|TabStaysOnFiles)'`
Expected: FAIL. `discoverHTTPFiles`가 반환값 2개라 컴파일 오류가 난다.

- [ ] **Step 3: 구현** — Interfaces대로 만든다. `skipIfNoPermEnforcement`는 Task 2와 같은 본문으로 `cmd_helpers_test.go`에 둔다(패키지가 달라 공유할 수 없다).

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/tui/browser.go internal/tui/browser_test.go internal/tui/app.go internal/tui/app_test.go internal/tui/layout_test.go internal/tui/cmd_helpers_test.go
git commit -m "tui: skip unreadable subdirs, record only real dirs, add empty-state guidance"
```

---

### Task 10: Browser `R` 재스캔, `o` 경로 열기 (P4-14)

**Files:**
- Modify: `internal/tui/keys.go` (`Rescan`, `OpenPath`)
- Modify: `internal/tui/browser.go` (`rescan`)
- Modify: `internal/tui/dirhistoryview.go` (프롬프트 필드, `startPrompt`, `isPrompting`, `Update`, `View`, `expandHome`)
- Modify: `internal/tui/app.go` (키 스위치, `activeScreenIsEditing`, `footerHints`, `helpLines`)
- Test: `internal/tui/browser_test.go`, `internal/tui/dirhistoryview_test.go`, `internal/tui/app_test.go`

**Interfaces:**
- Consumes: Task 9 `replaceBrowser`, `isDir`; Task 8 `isFiltering`
- Produces:
  - `keys.Rescan`(`R`), `keys.OpenPath`(`o`)
  - `func (m browserModel) rescan() browserModel`. 알고리즘은 다음과 같다.
    1. 기존 상태에서 Files 커서의 rel, 열린 파일의 rel(`parsedFile != nil`일 때 `selectedFile`의 root 기준 rel), Requests 커서 index, focus를 기억한다.
    2. `n := newBrowserModel(m.root, m.store).SetSize(m.width, m.height)`를 만든다.
    3. 열린 파일 rel이 새 목록에 있으면 그 index로 `n.files.Select`, `n = n.selectFile()`을 하고, `n.requests.Select(min(idx, len-1))`, `n.focus = 기존 focus`로 둔다. Requests가 비면 `paneFiles`다.
    4. 열린 파일이 사라졌으면 Files 커서 rel을 찾아(없으면 0) 선택하고 `paneFiles`로 둔다. 필터 상태는 이어받지 않는다.
  - App: `screenBrowser`에서 `keys.Rescan`이면 `a = a.replaceBrowser(a.browser.rescan())`이다. `screenBrowser`에서 `keys.OpenPath`이면 `a.screen = screenDirHistory`, `refresh`를 한 뒤 `a.dirHistory.startPrompt()`의 cmd를 돌려준다. 두 키 모두 `activeScreenIsEditing`이 거짓일 때만 동작한다.
  - `dirHistoryModel.prompt textinput.Model`, `promptErr string`, `func (m dirHistoryModel) startPrompt() (dirHistoryModel, tea.Cmd)`, `func (m dirHistoryModel) isPrompting() bool`. `App.activeScreenIsEditing`에 `screenDirHistory && isPrompting()`을 더한다.
  - `func expandHome(p string) (string, error)`. `strings.TrimSpace` 뒤 `~`이면 홈, `~/…`이면 홈 아래로 바꾸고 그 밖(`~bob/x` 포함)은 그대로 둔다. 홈은 `os.UserHomeDir`로 구한다.
  - `dirHistoryModel.Update`의 키 처리 순서: (1) 프롬프트 중이면 `msg.Type == tea.KeyEnter`·`tea.KeyEsc`만 따로 처리하고 나머지 키는 모두 `prompt.Update`로 넘긴다. `keys.Back`(esc와 backspace)으로 닫으면 backspace가 글자를 지우지 못하므로 `tea.KeyEsc`로만 닫는다. (2) Task 8의 `listCapturesKey`가 참이면 목록으로 넘긴다. (3) `keys.OpenPath`, `keys.Back`, `keys.Enter` 순으로 처리한다.
  - 프롬프트 표시: 프롬프트 중에는 목록 높이를 2줄 줄이고 목록 아래에 프롬프트 줄과 오류 줄(`promptErr`, errorTextStyle, 비었으면 빈 줄)을 둔다. `prompt.Width`는 안쪽 폭 − `lipgloss.Width("open: ")` − 1이고, 오류 줄은 `ansi.Truncate(…, 안쪽 폭, "…")`로 자른다.
  - 프롬프트 동작: Directories에서 `o`를 누르면 프롬프트가 열린다. Enter를 누르면 `expandHome` → `filepath.Abs` → `os.Stat`을 거친다. 실패하면 `no such directory: <입력값>`, 디렉터리가 아니면 `not a directory: <입력값>`을 `promptErr`에 두고 프롬프트를 유지한다. 성공하면 `switchDirMsg{path: abs}`를 낸다. `esc`는 프롬프트만 닫는다(화면은 그대로). 프롬프트 Prompt 문자열은 `open: `다.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
func TestBrowserModel_RescanPicksUpNewFileAndKeepsSelection(t *testing.T) {
	// a.http, b.http(요청 2개) → SetSize(120, 30) → ↓, Enter(b 선택) → ↓(요청 index 1)
	// c.http 추가, b.http에 요청 1개 추가 → m = m.rescan()
	// len(m.files.Items()) == 3, m.files.SelectedItem().(fileItem).rel == "b.http"
	// len(m.requests.Items()) == 3, m.requests.Index() == 1, m.focus == paneRequests
}

func TestBrowserModel_RescanAfterSelectedFileDeleted(t *testing.T) {
	// b.http를 열어 둔 뒤 삭제 → rescan → m.parsedFile == nil, m.focus == paneFiles, len(m.files.Items()) == 1
}

func TestApp_RKeyRescans(t *testing.T) {
	// root에 a.http → 앱 생성 후 b.http 추가 → "R" → len(a.browser.files.Items()) == 2
}

func TestExpandHome(t *testing.T) {
	t.Setenv("HOME", home)
	// "~" → home, "~/api" → filepath.Join(home, "api"), " /abs " → "/abs", "~bob/x" → "~bob/x"
}

func TestDirHistoryModel_OpenPathSwitchesToDir(t *testing.T) {
	t.Setenv("HOME", home) // home/proj 디렉터리 생성
	m, _ := newDirHistoryModel(store).refresh().Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	// m.isPrompting()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("~/proj")})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// cmd().(switchDirMsg).path == filepath.Join(home, "proj")
}

func TestDirHistoryModel_OpenPathRejectsMissingAndFile(t *testing.T) {
	// 없는 경로 Enter → cmd == nil, m.isPrompting(), View에 "no such directory: <입력값>"
	// 파일 경로 Enter → View에 "not a directory: <입력값>"
}

func TestDirHistoryModel_EscClosesPromptOnly(t *testing.T) {
	// o → esc → !m.isPrompting(), cmd == nil (backToBrowserMsg 아님)
}

func TestDirHistoryModel_BackspaceInPromptDeletesChar(t *testing.T) {
	// o → "ab" → tea.KeyBackspace → m.isPrompting(), m.prompt.Value() == "a"
}

func TestApp_OKeyFromBrowserOpensPrompt(t *testing.T) {
	// "o" → a.screen == screenDirHistory, a.dirHistory.isPrompting()
}

func TestApp_PromptTypingSwallowsGlobalKeys(t *testing.T) {
	// o → "q","h","d" → a.screen == screenDirHistory, a.dirHistory.prompt.Value() == "qhd", 종료 cmd 없음
}

```

`layoutScenarios`에 행 `dirs-open-path-error`를 추가한다. `pressKeys(a, "o", "/no/such/" + strings.Repeat("d", 200), "enter")`로 오류 문구가 보이는 상태다.

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestBrowserModel_Rescan|TestApp_(RKeyRescans|OKeyFromBrowserOpensPrompt|PromptTypingSwallowsGlobalKeys|ViewFitsTerminal)|TestExpandHome|TestDirHistoryModel_(OpenPath|EscClosesPromptOnly|BackspaceInPrompt)'`
Expected: FAIL. `rescan`·`expandHome`·`isPrompting`이 없어 컴파일 오류가 난다.

- [ ] **Step 3: 구현** — Interfaces대로 만든다. footer는 다음과 같다. Browser slice에 `hint{"R", "rescan"}, hint{"o", "open path"}`, Directories slice에 `hint{"o", "open path"}`를 추가하고, 프롬프트 중에는 `hint{"enter", "open"}, hint{"esc", "cancel"}`이다. `helpLines()`의 Browser에는 `"  R             rescan directory"`, `"  o             open a directory by path"`를, Directory History에는 `"  o             open a directory by path (~ ok)"`를 추가한다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/tui/keys.go internal/tui/browser.go internal/tui/browser_test.go internal/tui/dirhistoryview.go internal/tui/dirhistoryview_test.go internal/tui/app.go internal/tui/app_test.go internal/tui/layout_test.go
git commit -m "tui: add R to rescan the current root and o to open a directory by path"
```

---

### Task 11: README·CLAUDE.md 정리 (P4-17, P4-13·P4-3 문서)

**Files:**
- Modify: `README.md` (`## Installation` 하위 curl 절, `## Usage`, `## Keybindings`, `## History`, `## Directory history`)
- Modify: `CLAUDE.md` (`## Project overview`의 플래그 설명, `## Architecture`의 `historyview.go`·`messages.go` 항목)

**Interfaces:**
- Consumes: Task 3~10의 키·플래그 이름(아래 표가 기준). P3-8이 README에 추가한 IntelliJ 지원표와 env 탐색 규칙은 건드리지 않는다.
- Produces: 없음

- [ ] **Step 1: README 수정** (README 본문은 기존대로 한국어)
  - curl 절의 버전 지정 줄을 `` `curl -fsSL https://raw.githubusercontent.com/Ahngbeom/rest-tui/main/scripts/install.sh | REST_TUI_VERSION=v0.1.0 bash` ``로 바꾼다.
  - Usage: 첫 예시를 `rest-tui [flags] [dir]`로 바꾼다. 플래그 표의 `-config` 설명을 "히스토리 파일 경로(history file path)"로 고친다. `-timeout`(기본 `30s`, 요청 하나의 제한 시간) 행을 추가하고, 위치 인자 `dir`가 `-dir`와 같다는 점과 둘이 다르면 오류로 종료한다는 점을 한 줄 덧붙인다.
  - Keybindings 표를 다음 행으로 바꾼다(화면 열 추가).

| 화면 | 키 | 동작 |
|---|---|---|
| 전체 | `q`, `Ctrl+C` | 종료(전송 중이면 `q`를 두 번) |
| 전체 | `?` | 도움말 |
| 전체 | `h` / `d` | History / Directories로 이동 |
| 목록 | `↑`/`k`, `↓`/`j` | 이동 |
| 목록 | `/` | 필터(`Esc`로 해제) |
| Browser | `Enter` | 파일 열기 / Request view 열기 |
| Browser | `Tab`, `Esc` | Files ↔ Requests 포커스 |
| Browser | `R` | 현재 디렉터리 다시 스캔 |
| Browser·Directories | `o` | 경로를 입력해 디렉터리 열기 |
| Request view | `Enter`, `Ctrl+R` | 전송 |
| Request view | `Esc`, `Backspace` | 전송 중이면 취소, 아니면 출발 화면(Browser/History)으로 |
| Request view | `e` | env 순환 |
| Request view | `i` / `Ctrl+S` / `Esc` | 편집 시작 / 적용 / 취소 |
| Request view | `c`, `y` | 요청·응답 복사 |
| Request view | `g`/`Home`, `G`/`End` | 응답 맨 위 / 맨 아래 |
| History | `Enter` | 상세 보기 |
| History | `r` | 재실행(GET·HEAD·OPTIONS 외에는 `y/n` 확인) |
| History 상세 | `c`, `y` | 복사 |
| Directories | `Enter` | 선택한 디렉터리로 전환 |

  - History 절에 다음 세 가지를 적는다. 요청 헤더·본문·응답이 평문으로 저장되므로 `Authorization` 같은 비밀값도 그대로 남는다. 파일 권한은 0600이다. 최근 500건만 유지한다.
  - Directory history 절의 "`-dir`로 실행할 때마다"를 "실행할 때마다(존재하는 디렉터리만)"로 고친다.
- [ ] **Step 2: CLAUDE.md 수정** — Project overview의 플래그 목록에 위치 인자 `[dir]`, `-timeout`(기본 30s)을 넣고 `-config`를 history file path로 표기한다. "Every `-dir` run is also recorded" 문장은 "every run whose root is an existing directory is recorded"로 고친다. Architecture의 `historyview.go` 항목에서 `newRequestModelFromEntry`를 "`newRequestModelFromEntryNoSend` + `startSend`"로 바꾸고, `messages.go` 목록에 `backFromRequestMsg`를 추가한다.
- [ ] **Step 3: 검증**

Run: `grep -n 'REST_TUI_VERSION=v0.1.0 bash' README.md && grep -n -- '-timeout' README.md CLAUDE.md && grep -n '0600' README.md && ! grep -nE 'newRequestModelFromEntry([^N]|$)' CLAUDE.md`
Expected: 앞의 세 grep은 각각 1줄 이상 출력하고, 마지막 부정 grep은 출력 없이 성공한다.

Run: `awk '/^## Keybindings/{f=1;next} /^## /{f=0} f' README.md | grep -c '^| [^-]'`
Expected: `20` (헤더 1행 + 키 19행)

- [ ] **Step 4: 커밋**

```bash
git add README.md CLAUDE.md
git commit -m "docs: sync keybindings, flags, pinned-version install and history file safety"
```

---

### Task 12: tmux로 실제 바이너리 확인과 바이너리 재빌드 (P4 전체)

**Files:**
- Modify (재생성): `rest-tui`
- 임시: `$WORK`(저장소 밖, `mktemp -d`) 아래 fixture·서버·HOME

**Interfaces:**
- Consumes: Task 1~11 전부
- Produces: 없음

- [ ] **Step 1: 전체 게이트**

Run: `go test -race ./... && go vet ./... && test -z "$(gofmt -l .)"`
Expected: 모든 패키지 `ok`, 종료 코드 0

- [ ] **Step 2: 바이너리 빌드**

Run: `scripts/build.sh`
Expected: `[build.sh] rest-tui updated`

- [ ] **Step 3: fixture와 테스트 서버 준비**

```bash
WORK=$(mktemp -d); mkdir -p "$WORK/fixture/locked" "$WORK/empty" "$WORK/server" "$WORK/other"
chmod 000 "$WORK/fixture/locked"
cat > "$WORK/fixture/api.http" <<'EOF'
### slow
GET http://127.0.0.1:18094/slow

### redirect
GET http://127.0.0.1:18094/old

### create
POST http://127.0.0.1:18094/users
Content-Type: application/json

{"name":"bob"}
EOF
cp "$WORK/fixture/api.http" "$WORK/other/other.http"
```

`$WORK/server/main.go`:

```go
package main

import (
	"net/http"
	"strings"
	"time"
)

func main() {
	http.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(20 * time.Second):
		case <-r.Context().Done():
		}
	})
	http.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/new", http.StatusFound) })
	http.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(strings.Repeat("line\n", 200))) })
	http.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(strings.Repeat("x", 1229)))
	})
	_ = http.ListenAndServe("127.0.0.1:18094", nil)
}
```

Run: `go build -o "$WORK/srv" "$WORK/server/main.go" && ("$WORK/srv" &) && curl -s --retry 5 --retry-connrefused -o /dev/null -w '%{http_code}' http://127.0.0.1:18094/users`
Expected: `201`

- [ ] **Step 4: CLI 확인**

Run: `./rest-tui -h 2>&1 | head -20; ./rest-tui -dir "$WORK/fixture" "$WORK/empty"; echo "exit=$?"`
Expected: 도움말에 `rest-tui [flags] [dir]`, `Press ? inside the app`, `history file path`가 보인다. 두 번째 명령은 `rest-tui: -dir … point to different directories`와 `exit=2`를 출력한다.

- [ ] **Step 5: 120x35 화면 확인**

`T="/opt/homebrew/bin/tmux -L rest-tui-p4"`로 두고 `$T new-session -d -s p4 -x 120 -y 35 -c "$PWD" "env HOME=$WORK $PWD/rest-tui -config $WORK/history.json $WORK/fixture -timeout 8s"`를 실행한다. 위치 인자 뒤에 둔 `-timeout`이 적용되는지도 6번에서 함께 확인된다. `HOME=$WORK`이므로 사용자의 실제 `~/.config/rest-tui/dirs.json`은 건드리지 않는다. 이후 단계마다 `$T send-keys -t p4 <keys>; sleep 1; $T capture-pane -t p4 -p`로 확인한다. 모든 캡처의 첫 줄에 `rest-tui ›`가 있어야 한다(header 왼쪽 padding 때문에 첫 글자는 공백이다).

| # | 입력 | 화면에서 확인할 문구 | 요구사항 |
|---|---|---|---|
| 1 | (시작 직후) | 상태 줄 `skipped 1 unreadable dirs`, Files 항목 1줄 `api.http`, Requests `Select a file and press enter` | P4-12, P4-7, P4-15 |
| 2 | `/` `a` `p` `i` `Enter` `Enter` | 필터 뒤 `api.http`가 열리고 Requests 3개 | P4-7 |
| 3 | `Enter`(slow 열기) `Enter`(전송) | 요약 줄이 `sending… <경과>s · esc cancel` 형식이고 캡처할 때마다 경과 시간이 늘어남, footer `esc cancel` | P4-1 |
| 4 | `q` | `request in flight — press q again to quit`, 앱이 계속 실행 중 | P4-2 |
| 5 | `Escape` | 요약 줄 `ERR · canceled` | P4-1 |
| 6 | `Enter` 후 9초 대기 | `timed out after 8s (use -timeout to change)` | P4-3 |
| 7 | `Escape`, `Down`, `Enter`, `Enter`(redirect 전송) | `redirected: 302 → GET http://127.0.0.1:18094/new`, `200 OK · `, ` · 1000 B` | P4-4, P4-6 |
| 8 | `G` 그다음 `g` | 상태 줄 끝 `100%` 다음 `0%` | P4-6 |
| 9 | `Escape`, `Down`, `Enter`, `Enter`(create 전송) | `201 Created · ` … ` · 1.2 KB` | P4-6 |
| 10 | `h` | 행 `201 POST    create`, `200 GET     redirect`, `ERR GET     slow`, 끝에 `just now` | P4-10, P4-5 |
| 11 | (맨 위 POST 행에서) `r` | `rerun POST http://127.0.0.1:18094/users? (y/n)` → `n` 누르면 사라짐 | P4-9 |
| 12 | `Down`(redirect 행) `r` | breadcrumb `rest-tui › History › rerun`, 결과 뒤 `Escape` → `rest-tui › History` | P4-8, P4-9 |
| 13 | `Escape`(Browser로), 셸에서 `cp $WORK/fixture/api.http $WORK/fixture/new.http`, 그다음 `R` | Files에 `new.http` 추가, 열려 있던 `api.http` 선택 유지 | P4-14 |
| 14 | `o`, `~/other`, `Enter` | breadcrumb에 `$WORK/other`, Files `other.http` | P4-14 |
| 15 | `d` | Directories에 `$WORK/other`, `$WORK/fixture`. 없는 경로는 없음 | P4-12 |

색은 `$T capture-pane -t p4 -p -e`로 확인한다. `201`·`200` 앞에 초록(`ESC[92m` 또는 `ESC[32m`), `ERR` 앞에 빨강(`ESC[91m`/`ESC[31m`) SGR이 있어야 한다(P4-5).

- [ ] **Step 6: 80x24 확인**

Run: `$T kill-server; $T new-session -d -s s -x 80 -y 24 -c "$PWD" "env HOME=$WORK $PWD/rest-tui -config $WORK/history.json $WORK/fixture"`. 그다음 `Enter`(api.http) → `Down` `Down` `Enter` `Enter`(create 전송) 뒤와 `h` 뒤에 각각 `$T capture-pane -t s -p`를 실행한다.
Expected: 두 캡처 모두 첫 줄에 `rest-tui ›`가 있고, 마지막 줄에 `? help`와 `q quit`가 있다. Request view에 `201 Created · `가 보인다. History 행은 상태 코드로 시작하고, `…`로 잘려도 끝의 `just now`가 보인다.

- [ ] **Step 7: 빈 디렉터리·권한·정리**

Run: `$T kill-server; $T new-session -d -s e -x 120 -y 35 -c "$PWD" "env HOME=$WORK $PWD/rest-tui -config $WORK/history.json $WORK/empty"; sleep 1; $T capture-pane -t e -p | grep -c 'No .http files under'; stat -f '%Lp' "$WORK/history.json"; $T kill-server; pkill -f "$WORK/srv"; chmod 755 "$WORK/fixture/locked"; rm -rf "$WORK"`
Expected: grep 수 `1` 이상, `stat`은 `600`

- [ ] **Step 8: 바이너리 커밋 (별도)**

```bash
git status --short --untracked-files=no   # 추적 파일 중 rest-tui 만 변경
git add rest-tui
git commit -m "build: rebuild committed dev binary"
```

---

## 요구사항 → 태스크 추적표

| 요구사항 | 내용 | 태스크 |
|---|---|---|
| P4-1 | spinner·경과 시간, 첫 `esc` 취소, `canceled` 기록, 전송 중 아니면 뒤로 | 1(원인 보존), 3(`startSend`), 5, 12 |
| P4-2 | 전송 중 `q` 두 번 | 5, 12 |
| P4-3 | `-timeout`(기본 30s), `timed out after 30s (use -timeout to change)` | 1(원인 보존), 3, 11, 12 |
| P4-4 | `redirected: 302 → GET …` 표시와 히스토리 기록 | 1, 2, 4, 12 |
| P4-5 | 상태 코드·메서드 색(Request view, History, Recent) | 4, 7, 12 |
| P4-6 | `201 Created · 4ms · 1.2 KB`, `g`/`G`/`home`/`end`, 스크롤 % | 4, 12 |
| P4-7 | 모든 목록 `/` 필터, 파일 항목 한 줄 | 8, 12 |
| P4-8 | `esc`로 출발 화면 복귀, `rest-tui › History › rerun` | 6, 12 |
| P4-9 | 안전한 메서드는 바로 재전송, 그 밖은 `rerun POST …? (y/n)` | 6, 12 |
| P4-10 | 행 순서(상태·메서드·env·이름/URL·상대시간), 상태가 잘리지 않음 | 7, 12 |
| P4-11 | 0600, 500건, 임시 파일 + rename | 2, 12 |
| P4-12 | 읽을 수 없는 하위 폴더 건너뛰기·`skipped N unreadable dirs`, 없는 경로는 이력에 기록 안 함 | 9, 12 |
| P4-13 | 위치 인자, `-dir`와 충돌 시 오류 | 3, 11, 12 |
| P4-14 | `R` 재스캔(선택 유지), `o` 경로 입력(`~`, 존재 확인) | 10, 12 |
| P4-15 | 빈 상태 문구 두 가지, 빈 Requests에서 `tab` 무시 | 9, 12 |
| P4-16 | `-h` 설명·예시·키 안내, `-config` 설명, `go install` 버전 | 3, 12 |
| P4-17 | README 키 표, 버전 지정 설치 명령, 히스토리 평문·0600 안내 | 11 |
