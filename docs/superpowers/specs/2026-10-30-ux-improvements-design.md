# rest-tui UX 개선 설계 (2026-10-30 UX 리뷰 후속)

## 배경

2026-10-30 UX 리뷰(HEAD `5d02c73`, v0.2.2)에서 네 관점(설치·첫 실행, 쉬운가, 간편한가, 직관적인가) 모두 5점 만점에 2.5점이 나왔다. 설치 경로와 기본 흐름은 탄탄하다. 문제는 두 갈래다.

- IntelliJ `.http` 파일을 그대로 가져온 사용자가 첫 요청에서 막히거나, 잘못된 요청을 성공으로 오인한다.
- 화면 상태 버그(다른 요청의 응답이 붙음, 커서 이탈, 도움말 뒤로 키 입력 통과, 레이아웃 높이 초과)가 체감 품질을 크게 떨어뜨린다.

## 결정 사항

| 주제 | 결정 |
|---|---|
| IntelliJ 호환 | 흔한 문법은 지원한다. 대상은 동적 변수 `$uuid`·`$timestamp`·`$isoTimestamp`·`$randomInt`·`$random.*`, `< file` 본문, 메서드 생략, 여러 줄 쿼리, 헤더 구간 주석이다. `> {% %}`·`> file.js`·`>> file` 응답 핸들러는 본문에서 빼고 "미지원" 경고만 띄운다. JS는 실행하지 않는다. |
| 히스토리 보안 | 파일 권한 0600, 최근 500건 상한, 임시 파일에 쓴 뒤 rename하는 원자적 쓰기. 기록 내용은 마스킹하지 않는다(rerun 정확성 유지). |
| 범위 | High·Medium 전부. Low는 README·CLI 정리처럼 비용이 낮은 것만 넣는다. |
| 실행 순서 | P1 → P2 → P3 → P4. 단계마다 단독으로 배포할 수 있어야 한다. 뒤 단계는 앞 단계의 "단계 간 계약"만 전제한다. |

## 요구사항

### P1. 화면 상태·레이아웃·출력 안전 (`docs/superpowers/plans/2026-10-30-ux-p1-state-layout.md`)

| ID | 요구사항 |
|---|---|
| P1-1 | 응답은 그 응답을 보낸 Request view 인스턴스에만 표시한다. 다른 요청을 연 뒤 이전 응답이 도착해도 현재 화면은 바뀌지 않는다. 히스토리 저장은 그대로 한다. |
| P1-2 | Browser에서 파일을 바꾸면 Requests 목록의 선택이 첫 항목으로 초기화되고, Enter가 바로 동작한다. |
| P1-3 | 도움말 오버레이는 모달이다. `?`·`esc`는 닫고, `q`·`ctrl+c`는 종료하고(P4 이후에는 전송 중이면 P4-2를 따른다), `↑/↓`·`j/k`는 도움말을 스크롤한다. 그 밖의 키는 무시하고 뒤 화면에 전달하지 않는다. 80x24에서도 위쪽이 잘리지 않는다(스크롤 가능). |
| P1-4 | 모든 화면과 상태(복사 알림, 편집 오류, 스토어 경고, 파싱 경고, 스캔 오류)에서 `App.View()`의 출력은 터미널 높이·폭을 넘지 않는다. 대상 크기는 60x20, 80x24, 120x35, 200x50이다. |
| P1-5 | Request view의 viewport 폭은 패널 안쪽 폭과 같다. 긴 줄은 한 번만 줄바꿈된다. |
| P1-6 | 파싱 경고는 패널 폭으로 줄바꿈하고 최대 3줄까지 보여 준다. 넘치면 `… +N more`로 줄인다. |
| P1-7 | footer 힌트는 폭이 모자라면 가운데 힌트부터 생략한다. `? help`, `q quit`은 항상 보인다. |
| P1-8 | Browser 두 패널은 내용 길이와 관계없이 고정 비율(Files 40% / Requests 60%)로 화면 폭 전체를 쓴다. History 목록도 전체 폭을 쓴다. |
| P1-9 | 응답 헤더와 본문에서 터미널 제어 시퀀스(ESC로 시작하는 CSI/OSC, `\n`·`\t`를 뺀 C0, C1)를 제거한다. 본문 색상은 앱이 렌더링 단계에서 입힌다. 바이너리 본문은 `(binary body: <N> bytes, <content-type>)`로 대체한다. |

### P2. env와 변수 (`…-ux-p2-env-variables.md`)

| ID | 요구사항 |
|---|---|
| P2-1 | env 파일의 숫자·bool 값은 문자열로 쓴다(`18091` → `"18091"`, `true` → `"true"`). 객체·배열 값(`SSLConfiguration` 등)은 건너뛴다. 이런 값 때문에 env 전체가 버려지지 않는다. |
| P2-2 | `$shared` env의 값은 모든 env에 깔린다. 우선순위는 fileVars > private[env] > public[env] > private[$shared] > public[$shared]이다. `$shared`는 env 목록에 나오지 않는다. |
| P2-3 | env 파일을 `.http` 파일의 폴더부터 `-dir` 루트까지 위로 올라가며 찾는다. 가장 가까운 폴더 하나를 쓴다. public과 private은 같은 폴더에서 짝으로 읽는다. |
| P2-4 | env 줄 문구: 찾았으면 `env: dev (1/3) · <루트 기준 상대 경로>/http-client.env.json — e cycle`(env가 하나뿐이면 `— e cycle` 생략). 없으면 `no http-client.env.json found (searched: api/, ./)`. 파싱에 실패하면 오류 스타일로 `<파일명>: <오류>`. |
| P2-5 | 선택한 env는 세션 동안 유지한다. 다음에 연 요청에 같은 이름의 env가 있으면 그것을 고른다. 없으면 알파벳순 첫 env를 고른다. env 이름에 `prod`가 들어 있으면 env 줄을 경고 색으로 표시한다. |
| P2-6 | 해결되지 않은 변수가 있으면 전송을 막는다. 이때 안내 문구는 `cannot send: unresolved {{token}} — e switch env · i edit`이다. Enter를 누르면 같은 문구가 강조되고, footer의 send 힌트는 숨긴다. |
| P2-7 | 편집을 적용한 상태에서 `e`를 누르면 바로 바꾸지 않는다. `edits will be discarded — press e again`을 띄우고, 두 번째 `e`에서 바꾼다. |
| P2-8 | 히스토리 항목에 env 이름을 기록한다. 응답을 받은 뒤 env를 바꾸면 응답 영역에 `(sent with env: dev)`를 표시한다. History 목록과 상세에도 env를 보여 준다. |
| P2-9 | 히스토리나 Recent에서 연 요청의 env 줄은 `resolved at send time (env: dev)`로 표시한다. env를 바꿀 수 없는 화면에서는 footer에서 `e cycle env`를 숨긴다. |

### P3. IntelliJ 호환 (`…-ux-p3-intellij-compat.md`)

| ID | 요구사항 |
|---|---|
| P3-1 | 메서드가 없는 요청 줄(`{{host}}/users`, `https://…`)은 GET으로 처리한다. |
| P3-2 | 요청 줄 다음에 들여 쓴 `?…`·`&…` 줄은 URL에 이어 붙인다. |
| P3-3 | 헤더 구간의 `#`·`//` 줄은 주석으로 건너뛴다. |
| P3-4 | 응답 핸들러(`> {% … %}` 여러 줄 포함, `> path.js`)와 출력 리디렉션(`>> file`, `>>! file`)은 본문에서 제거한다. 제거한 항목은 `Request.Unsupported`에 기록하고, Request view에 `unsupported (ignored): response handler script`처럼 경고한다. 전송은 허용한다. |
| P3-5 | 본문 안의 `< path` 줄은 `.http` 파일 기준 상대 경로의 파일 내용으로 바꾼다(multipart part 안 포함). 파일이 없으면 해결되지 않은 변수처럼 전송을 막고 `missing file: ./body.json`을 표시한다. |
| P3-6 | 동적 변수: `$uuid`, `$random.uuid`, `$timestamp`(Unix 초), `$isoTimestamp`(RFC3339 UTC), `$randomInt`(0–1000), `$random.integer(min, max)`, `$random.alphabetic(n)`, `$random.alphanumeric(n)`, `$random.hexadecimal(n)`, `$random.email`. 미리보기를 만들 때 값을 생성하고, 화면에 보이는 값을 그대로 보낸다. 전송이 끝나면 다음 전송을 위해 새로 생성한다. 알 수 없는 `$` 변수는 해결되지 않은 변수로 취급한다. 편집(`i`)을 적용할 때 편집 결과에 남은 동적 변수도 이때 생성해 보여 주고, 그 값을 보낸다. |
| P3-7 | 스킴이 없는 URL(`localhost:8080/x`)에는 `http://`를 붙인다. |
| P3-8 | README에 IntelliJ 기능 지원표(지원 / 무시+경고 / 미지원)와 env 탐색 규칙을 싣는다. |

### P4. 피드백·효율·안전 (`…-ux-p4-feedback-efficiency.md`)

| ID | 요구사항 |
|---|---|
| P4-1 | 전송 중에는 spinner와 경과 시간을 보여 준다. 첫 `esc`는 전송을 취소한다(context cancel). 취소 결과는 `canceled`로 히스토리에 남는다. 전송 중이 아니면 `esc`는 뒤로 간다. |
| P4-2 | 전송 중 `q`를 누르면 `request in flight — press q again to quit`을 띄우고, 두 번째 `q`에서 종료한다. |
| P4-3 | `-timeout` 플래그를 추가한다(기본 30s). 타임아웃 오류는 `timed out after 30s (use -timeout to change)`로 표시한다. |
| P4-4 | 리다이렉트를 따라갔으면 응답 상단에 `redirected: 302 → GET http://…/new`처럼 경로를 표시하고, 히스토리에 기록한다. 따라가는 동작 자체는 바꾸지 않는다. |
| P4-5 | 상태 코드에 색을 입힌다(2xx 성공색, 3xx 강조색, 4xx 경고색, 5xx·오류 오류색). HTTP 메서드에도 색을 입힌다. Request view·History 목록·Recent에 적용한다. |
| P4-6 | Request view 상단 고정 줄에 `201 Created · 4ms · 1.2 KB`를 보여 준다. viewport에 `g`/`G`/`home`/`end`를 추가하고, 스크롤 위치(%)를 상태 줄에 표시한다. |
| P4-7 | 모든 목록(Files, Requests, History, Directories)에서 `/` 필터를 켠다. 파일 항목은 한 줄로 표시한다. |
| P4-8 | `esc`는 Request view를 연 화면(Browser 또는 History)으로 돌아간다. breadcrumb도 출발 화면을 반영한다(`rest-tui › History › rerun`). |
| P4-9 | History의 `r`은 GET·HEAD·OPTIONS면 바로 재전송한다. 그 밖의 메서드는 `rerun POST …? (y/n)`으로 확인받는다. |
| P4-10 | History 목록 한 줄은 `상태(고정폭, 색) 메서드 env 이름-또는-URL 상대시간` 순서로 표시한다. 상태 코드가 줄 끝에서 잘리지 않는다. |
| P4-11 | 히스토리 저장소: 파일 권한 0600, 최근 500건 유지, 임시 파일에 쓴 뒤 rename. |
| P4-12 | 디렉터리 스캔: 읽을 수 없는 하위 폴더는 건너뛰고 `skipped N unreadable dirs`를 표시한다. 루트 자체를 읽을 수 없을 때만 오류로 처리한다. 존재하지 않거나 디렉터리가 아닌 경로는 디렉터리 이력에 기록하지 않는다. |
| P4-13 | 위치 인자를 디렉터리로 받는다(`rest-tui ./api`). `-dir`과 함께 다른 값을 주면 오류로 종료한다. |
| P4-14 | Browser의 `R`은 현재 루트를 다시 스캔한다(선택 위치는 가능하면 유지). Directories 화면의 `o`는 경로 입력 프롬프트를 띄운다(`~` 확장, 존재 확인 후 전환). |
| P4-15 | 빈 상태 안내: `.http`가 없으면 `No .http files under <root> — d recent dirs · o open path`. 파일을 고르기 전 Requests 패널에는 `Select a file and press enter`. 요청 목록이 비어 있을 때 `tab`을 누르면 포커스를 옮기지 않는다. |
| P4-16 | CLI: `-h`에 도구 설명·사용 예·키 도움말 안내를 넣는다. `-config`의 설명을 "history file path"로 고친다. 이름은 유지한다. `go install`로 설치한 경우 `-version`이 `debug.ReadBuildInfo`의 모듈 버전을 보여 준다. |
| P4-17 | README: 키바인딩 표를 실제 키와 맞춘다. 버전 지정 설치 명령을 `curl … \| REST_TUI_VERSION=v0.1.0 bash`로 고친다. 히스토리 파일에 비밀값이 평문으로 저장된다는 점과 권한 0600을 안내한다. |

## 단계 간 계약

뒤 단계 계획은 아래 이름과 시그니처만 전제한다. 앞 단계 계획은 이 이름 그대로 만들어야 한다.

| 만드는 단계 | 이름 | 내용 |
|---|---|---|
| P1 | `requestModel.id uint64`, `execResultMsg.modelID uint64` | 패키지 수준 카운터 `nextRequestModelID()`로 부여한다. `requestModel.Update`는 `msg.modelID != m.id`인 결과를 무시한다. |
| P1 | `func assertViewFits(t *testing.T, view string, width, height int)` (`internal/tui/layout_test.go`) | `lipgloss.Height(view) <= height`이고 각 줄의 `lipgloss.Width <= width`인지 확인한다. 뒤 단계의 레이아웃 테스트가 재사용한다. |
| P1 | `type hint struct{ key, desc string }`, `func renderHints(hints []hint, width int) string` (`internal/tui/footer.go`) | footer 렌더링 경로는 이것 하나다. 마지막 두 힌트(`? help`, `q quit`)는 항상 남긴다. 뒤 단계는 힌트 slice만 바꾼다. |
| P1 | Request view 고정 상태 줄 1행 | viewport 아래에 항상 1행을 예약한다. 복사 알림, 편집 오류, 전송 차단 안내(P2-6), 스크롤 %(P4-6)가 이 줄을 공유한다. 내용은 `requestModel.statusLine() string` 하나에서 만든다. |
| P1 | `output.Sanitize(s string) string`, `output.IsBinary(body []byte, contentType string) bool` | P1-9 |
| P2 | `env.LoadNearest(dir, root string) (env.Loaded, error)`, `type Loaded struct { Public, Private map[string]map[string]string; Dir string; Searched []string }` | `Dir`가 `""`이면 찾지 못한 것이다. |
| P2 | `newRequestModel(filePath, root string, file *httpfile.File, req httpfile.Request, store *history.Store, preferredEnv string) requestModel`, `requestModel.currentEnv() string`, `requestModel.canSend() bool` | App은 `lastEnv string`을 들고 있다가, Request view의 Update 전후로 `currentEnv()`가 바뀌었고 새 값이 빈 문자열이 아닐 때만 갱신한다(사용자가 실제로 고른 env만 세션 env가 된다). |
| P2 | `history.Entry.Env string` (JSON 키 `Env`, `omitempty`) | |
| P3 | `httpfile.Request.Unsupported []string` | 파서가 채우고, resolver는 그대로 보존한다. |
| P3 | `env.ResolveRequest(req httpfile.Request, vars map[string]string, opts env.ResolveOptions) (httpfile.Request, []string)`, `type ResolveOptions struct { BaseDir string; Now time.Time; Rand *rand.Rand }` | 반환하는 `[]string`은 해결되지 않은 변수와 `missing file: …` 항목을 합친 목록이다. |
| P4 | `tui.Options{ Timeout time.Duration }`, `NewApp(root string, store *history.Store, dirStore *dirhistory.Store, opts Options) App` | |
| P4 | `executor.Response.Redirects []string`, `history.Entry.Redirects []string` | |
| P1~P3 | 보조 이름: `layoutScenarios`, `newLayoutApp`, `pressKeys`, `applyMsg`, `App.resize()`, `fitLines`, `helpLines()`(P1) · `blockedMessage`, `edited`, `warnTextStyle`, `colorWarning`, `applyTestEdit`(P2) · `fromEntry`, `entryEnv`, `resolveOptions()`(P3) | 계약 밖이지만 뒤 단계 계획이 쓰는 이름이다. 앞 단계는 이 이름으로 만든다. 구현 중 이름이 바뀌면 뒤 단계 계획의 해당 줄도 같은 커밋에서 고친다. |

## 공통 규칙

- 각 태스크는 실패하는 테스트로 시작한다. 마칠 때 `go test ./...`, `go vet ./...`, `gofmt -l .`(출력 없음)이 모두 통과해야 한다.
- 레이아웃이 바뀌는 태스크는 `assertViewFits`로 60x20, 80x24, 120x35에서 검증한다.
- 각 계획의 마지막 태스크는 실제 바이너리를 tmux로 띄워 확인하고, `scripts/build.sh`로 바이너리를 다시 빌드한 뒤 `build: rebuild committed dev binary`로 따로 커밋한다.
- 커밋 메시지는 영어이고 `<area>: <summary>` 형식이다(기존 이력 관례, 예: `tui: …`, `env: …`, `httpfile: …`).
- 사용자에게 보이는 UI 문구는 영어다.

## 범위 밖

마우스 지원, 응답 본문 검색, 연속/일괄 실행, curl 복사·파일 저장, OSC52 클립보드, `NO_COLOR` 본문 적용, charset 변환, Windows 경로 차이, JS 핸들러 실행.
