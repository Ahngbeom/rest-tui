# rest-tui UX P2: env와 변수 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** IntelliJ에서 가져온 env 파일(숫자·bool·객체 값, `$shared`, 상위 폴더 배치)을 그대로 읽고, 어떤 env로 무엇을 보내는지 화면과 히스토리에서 항상 알 수 있게 한다(spec P2-1~P2-9).

**Architecture:** `internal/env`가 파일 읽기(스칼라 문자열화), `$shared` 병합, 상위 폴더 탐색(`LoadNearest`)을 맡는다. 이 패키지는 UI를 모른다. `internal/tui`의 Request view는 `LoadNearest` 결과로 env 줄을 만들고, 전송 가능 여부(`canSend`)와 env 전환 가능 여부(`canCycleEnv`)를 기준으로 상태 줄·footer를 바꾼다. App은 세션 동안 마지막 env(`lastEnv`)를 들고 다음 Request view에 넘긴다. 히스토리는 `Entry.Env`로 전송 당시 env를 남긴다.

**Tech Stack:** Go 1.26, Bubble Tea / Bubbles / Lipgloss, `github.com/charmbracelet/x/ansi`(이미 의존성에 있음), 표준 `testing`, tmux.

**Spec:** `docs/superpowers/specs/2026-10-30-ux-improvements-design.md` (P2 표, 단계 간 계약, 공통 규칙)

**전제:** P1 계획(`2026-10-30-ux-p1-state-layout.md`)이 머지된 상태에서 시작한다. P1이 바꾸는 파일(`internal/tui/requestview.go`, `app.go`, `browser.go`, `historyview.go`)은 줄 번호 대신 함수 이름으로 가리킨다. P1에서 가져다 쓰는 것은 spec "단계 간 계약"의 아래 다섯 가지뿐이다.

- `requestModel.id uint64`, `execResultMsg.modelID uint64`
- `func assertViewFits(t *testing.T, view string, width, height int)` (`internal/tui/layout_test.go`)
- `type hint struct{ key, desc string }`, `func renderHints(hints []hint, width int) string` (`internal/tui/footer.go`)
- `requestModel.statusLine() string` (viewport 아래 고정 1행)
- (P2에서는 쓰지 않음) `output.Sanitize`, `output.IsBinary`

**root 전달 방식 결정:** `openRequestMsg`는 바꾸지 않고, App이 `a.root`를 `newRequestModel`에 넘긴다. `a.root`와 `a.browser.root`는 `NewApp`과 `App.Update`의 `switchDirMsg` 분기에서만, 항상 같은 값으로 함께 대입된다. 그래서 Browser가 `filePath`를 만들 때 쓴 root와 App의 root는 같다. 메시지에 필드를 더하면 `openRequestMsg`를 만드는 기존 테스트가 모두 `root: ""`가 되어 탐색 범위가 조용히 바뀌므로 이 방식을 택한다.

## Global Constraints

- `go.mod`의 `go 1.26.4`를 유지한다. 새 외부 의존성을 추가하지 않는다.
- 각 태스크를 마칠 때 `go test ./...`, `go vet ./...`, `gofmt -l .`(출력 없음)이 모두 통과해야 한다.
- 레이아웃이 바뀌는 태스크(Task 4, 6)는 `assertViewFits`로 60x20, 80x24, 120x35에서 검증한다. env 줄은 `envLine()` 안에서 `m.viewport.Width`로 잘라 한 줄을 유지한다. P1 계획의 `View`도 env 줄을 자르지만 그것은 단계 간 계약이 아니므로 기대지 않는다(두 번 잘라도 결과는 같다).
- 커밋 메시지는 영어 `<area>: <summary>`. 사용자에게 보이는 문구는 영어이고, spec이 정한 문구는 글자 그대로 쓴다.
  - `env: dev (1/3) · <루트 기준 상대 경로>/http-client.env.json — e cycle`
  - `no http-client.env.json found (searched: api/, ./)`
  - `<파일명>: <오류>` (오류 스타일)
  - `cannot send: unresolved {{token}} — e switch env · i edit`
  - `edits will be discarded — press e again`
  - `(sent with env: dev)`
  - `resolved at send time (env: dev)`
- 변수 우선순위: fileVars > private[env] > public[env] > private[$shared] > public[$shared]. `$shared`는 env 목록에 나오지 않는다.
- 선택한 env는 세션 동안만 유지한다. 파일에 저장하지 않는다.
- 코드 주석은 "조용히 깨지는 것"을 설명할 때만 한 줄 쓴다.
- 범위 밖: env 직접 선택 메뉴, 디렉터리별 env 영구 저장, 동적 변수(`$uuid` 등, P3-6), README 지원표(P3-8).

## Review Focus

1. 차단 안내가 권하는 `i edit`로 편집하고도 `{{token}}`을 그대로 둔 채 `ctrl+s`로 적용했을 때: 지금 `applyEdit`는 `missingVars`를 비우므로 `Bearer {{token}}`이 글자 그대로 전송된다. 편집 결과에 남은 `{{…}}`도 해결되지 않은 변수로 보고 전송을 막아야 한다. → Task 6 `TestRequestModel_ApplyEditKeepsLeftoverPlaceholdersBlocked`
2. env 값이 `null`이거나, env 항목 자체가 객체가 아닐 때(`"dev": "oops"`), 또는 env 파일이 비어 있을 때(0바이트·공백만): env 파일 전체가 아니라 그 값·항목만 건너뛰고, 빈 파일은 `{}`로 읽어야 한다. → Task 1 `TestLoadFiles_SkipsNullAndNonObjectEnvs`, `TestLoadFiles_EmptyFileYieldsNonNilMap`
3. 큰 정수·소수(`12345678901234567890`, `1.5`): `1.2345678901234567e+19`처럼 바뀌면 안 되고 파일에 쓴 글자 그대로여야 한다. → Task 1 `TestLoadFiles_StringifiesScalarValues`
4. 상대 경로 root(`-dir .`, `-dir ./api/`처럼 끝에 `/`가 붙은 경우)와 root 밖의 폴더: 탐색이 root에서 정확히 멈추고 무한 반복하지 않아야 한다. → Task 3 `TestLoadNearest_RelativeRoot`, `TestLoadNearest_DirOutsideRootSearchesOnlyDir`
5. env 파일이 없는 폴더의 요청을 열었다 돌아왔을 때: 세션 env가 `""`로 지워지면 안 된다. → Task 5 `TestApp_SelectedEnvCarriesToNextRequest`

---

### Task 1: env 값 문자열화 (P2-1)

**Files:**
- Modify: `internal/env/envfile.go:14-17` (상수), `:33-46` (`loadOne`)
- Test: `internal/env/envfile_test.go`

**Interfaces:**
- Produces: `env.PublicFileName = "http-client.env.json"`, `env.PrivateFileName = "http-client.private.env.json"` (기존 비공개 상수 `publicFileName`/`privateFileName`의 이름만 바꿔 공개한다). `LoadFiles(dir string) (public, private map[string]map[string]string, err error)`의 시그니처는 그대로다. 바뀌는 동작은 세 가지다. 숫자·bool 값은 문자열로 바뀐다. 객체·배열·`null` 값과 객체가 아닌 env 항목은 건너뛴다. 파일이 있으면(`{}`이어도, 0바이트·공백만이어도) 그쪽 map은 nil이 아니다. 파싱 오류 메시지는 `<파일 base 이름>: <json 오류>` 형식이다.

- [ ] **Step 1: 실패하는 테스트 작성** (`internal/env/envfile_test.go` 끝에 추가)

```go
func TestLoadFiles_StringifiesScalarValues(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, PublicFileName, `{"local": {
		"host": "http://127.0.0.1:18091", "port": 18091, "debug": true, "off": false,
		"ratio": 1.5, "big": 12345678901234567890,
		"SSLConfiguration": {"clientCertificate": "a.pem"}, "list": [1, 2]
	}}`)

	public, _, err := LoadFiles(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{
		"host": "http://127.0.0.1:18091", "port": "18091", "debug": "true", "off": "false",
		"ratio": "1.5", "big": "12345678901234567890",
	}
	if !reflect.DeepEqual(public["local"], want) {
		t.Errorf("local = %v, want %v", public["local"], want)
	}
}

func TestLoadFiles_SkipsNullAndNonObjectEnvs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, PublicFileName, `{"dev": "oops", "qa": null, "prod": {"a": "b", "n": null}}`)

	public, _, err := LoadFiles(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]map[string]string{"prod": {"a": "b"}}
	if !reflect.DeepEqual(public, want) {
		t.Errorf("public = %v, want %v", public, want)
	}
}

func TestLoadFiles_EmptyFileYieldsNonNilMap(t *testing.T) {
	for _, content := range []string{`{}`, ``, " \n"} {
		dir := t.TempDir()
		writeFile(t, dir, PrivateFileName, content)

		public, private, err := LoadFiles(dir)
		if err != nil {
			t.Fatalf("content %q: unexpected error: %v", content, err)
		}
		if public != nil {
			t.Errorf("content %q: public = %v, want nil (file absent)", content, public)
		}
		if private == nil {
			t.Errorf("content %q: private = nil, want non-nil map (file present)", content)
		}
	}
}

func TestLoadFiles_ErrorNamesFileByBaseName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, PublicFileName, `{ not valid json `)

	_, _, err := LoadFiles(dir)
	if err == nil || !strings.HasPrefix(err.Error(), "http-client.env.json: ") {
		t.Fatalf("err = %v, want prefix %q", err, "http-client.env.json: ")
	}
}
```

`strings`를 import에 추가한다.

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/env/ -run 'TestLoadFiles_'`
Expected: FAIL. 처음에는 `undefined: PublicFileName` 컴파일 오류가 난다. 상수 이름만 바꾸면 `StringifiesScalarValues`는 json 타입 오류로, `SkipsNullAndNonObjectEnvs`는 `"oops"` 때문에, `EmptyFileYieldsNonNilMap`은 빈 내용의 `unexpected end of JSON input` 때문에, `ErrorNamesFileByBaseName`는 전체 경로 접두어 때문에 실패한다.

- [ ] **Step 3: 구현** (`internal/env/envfile.go`)

- 상수 `publicFileName`/`privateFileName`을 `PublicFileName`/`PrivateFileName`으로 바꾼다.
- `loadOne(path string) (map[string]map[string]string, error)`: 내용이 `bytes.TrimSpace` 후 비어 있으면 빈 map을 돌려준다. 아니면 `json.NewDecoder(bytes.NewReader(data))`에 `UseNumber()`를 켜고 파일 전체를 한 번에 `map[string]any`로 읽는다(항목을 `json.RawMessage`로 받아 다시 읽으면 두 번째 디코딩에 `UseNumber`가 빠져 큰 정수가 float64로 바뀐다). env 항목 값이 `map[string]any`가 아니면(`null`·문자열·배열) 건너뛴다. 그 안의 값은 `string`이면 그대로, `json.Number`이면 `.String()`, `bool`이면 `strconv.FormatBool`로 넣고, 그 밖의 값(`map[string]any`, `[]any`, `nil`)은 건너뛴다. 결과 map은 항상 `make`로 만든다. 디코딩 오류만 `fmt.Errorf("%s: %w", filepath.Base(path), err)`로 감싼다. `os.ReadFile`의 NotExist 외 오류는 지금처럼 그대로 돌려준다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/env/`
Expected: PASS (기존 `TestLoadFiles_*` 포함)

- [ ] **Step 5: 커밋**

```bash
git add internal/env/envfile.go internal/env/envfile_test.go
git commit -m "env: stringify number and bool values instead of rejecting the env file"
```

---

### Task 2: `$shared` env 병합 (P2-2)

**Files:**
- Modify: `internal/env/envfile.go` (`Merge`, `EnvNames`)
- Modify: `README.md` 기능 목록(8행)과 "Environment configuration" 절의 우선순위 문장, `CLAUDE.md` Architecture의 `internal/env/` 항목 우선순위 문장
- Test: `internal/env/envfile_test.go`

**Interfaces:**
- Produces: `env.SharedEnvName = "$shared"`. `Merge(public, private map[string]map[string]string, envName string, fileVars map[string]string) map[string]string`는 시그니처를 유지하고, 우선순위가 fileVars > private[envName] > public[envName] > private[$shared] > public[$shared]로 바뀐다. `envName == ""`이어도 `$shared`는 적용된다. `EnvNames`는 `$shared`를 뺀 정렬된 목록을 돌려준다.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
func TestMerge_SharedLayersUnderEveryEnv(t *testing.T) {
	public := map[string]map[string]string{
		"$shared": {"k1": "public-shared", "k2": "public-shared", "k3": "public-shared", "k4": "public-shared", "k5": "public-shared"},
		"dev":     {"k1": "public-dev", "k2": "public-dev", "k3": "public-dev"},
	}
	private := map[string]map[string]string{
		"$shared": {"k1": "private-shared", "k2": "private-shared", "k3": "private-shared", "k4": "private-shared"},
		"dev":     {"k1": "private-dev", "k2": "private-dev"},
	}
	got := Merge(public, private, "dev", map[string]string{"k1": "file"})

	want := map[string]string{"k1": "file", "k2": "private-dev", "k3": "public-dev", "k4": "private-shared", "k5": "public-shared"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Merge() = %v, want %v", got, want)
	}
}

func TestMerge_SharedAppliesWithNoEnvSelected(t *testing.T) {
	public := map[string]map[string]string{"$shared": {"host": "https://shared.example.com"}}
	got := Merge(public, nil, "", nil)
	if got["host"] != "https://shared.example.com" {
		t.Errorf("host = %q", got["host"])
	}
}

func TestEnvNames_ExcludesShared(t *testing.T) {
	public := map[string]map[string]string{"$shared": {}, "prod": {}}
	private := map[string]map[string]string{"$shared": {}, "dev": {}}
	want := []string{"dev", "prod"}
	if got := EnvNames(public, private); !reflect.DeepEqual(got, want) {
		t.Errorf("EnvNames() = %v, want %v", got, want)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/env/ -run 'TestMerge_Shared|TestEnvNames_ExcludesShared'`
Expected: FAIL. `k4`/`k5`와 `host`가 비어 있고, `EnvNames`에 `$shared`가 들어 있다.

- [ ] **Step 3: 구현**

`SharedEnvName` 상수를 추가한다. `Merge`는 public[$shared] → private[$shared] → public[env] → private[env] → fileVars 순서로 덮어쓴다. `EnvNames`는 `SharedEnvName`을 건너뛴다.

- [ ] **Step 4: 문서 문장 갱신**

- `README.md`: "Environment configuration" 절의 "변수 우선순위는 …" 문장을 **파일 스코프 변수 > private 환경 > public 환경 > private `$shared` > public `$shared`**로 고치고, `$shared` 값은 모든 환경에 기본값으로 깔리며 환경 목록에는 나오지 않는다는 문장 하나를 더한다. 맨 위 기능 목록(8행)의 괄호 `(우선순위: 파일 변수 > private 환경 > public 환경)`도 같은 순서로 고친다.
- `CLAUDE.md`: `internal/env/` 항목의 "Precedence: fileVars > private > public"를 위 우선순위로 고친다.

Run: `grep -n '우선순위\|Precedence' README.md CLAUDE.md`
Expected: 나온 줄이 모두 `$shared`를 포함한 새 순서다.

- [ ] **Step 5: 통과 확인**

Run: `go test ./internal/env/`
Expected: PASS (기존 `TestMerge_Precedence`, `TestMerge_UnknownEnvNameYieldsOnlyFileVars` 포함)

- [ ] **Step 6: 커밋**

```bash
git add internal/env/envfile.go internal/env/envfile_test.go README.md CLAUDE.md
git commit -m "env: layer \$shared values under every environment"
```

---

### Task 3: 상위 폴더 env 탐색 `LoadNearest` (P2-3)

**Files:**
- Create: `internal/env/nearest.go`
- Modify: `CLAUDE.md` Architecture의 `internal/env/` 항목
- Test: `internal/env/nearest_test.go`

**Interfaces:**
- Consumes: Task 1의 `LoadFiles`, `PublicFileName`, `PrivateFileName`
- Produces:
  - `type Loaded struct { Public, Private map[string]map[string]string; Dir string; Searched []string }`. `Dir == ""`이면 찾지 못한 것이다. `Searched`는 확인한 폴더 경로를 가까운 순서로 담는다(`filepath.Clean`한 값). 찾았으면 마지막 원소가 `Dir`다.
  - `func LoadNearest(dir, root string) (Loaded, error)`. `dir`부터 위로 올라가며 `PublicFileName`이나 `PrivateFileName` 중 하나라도 있는 첫 폴더를 찾고, 그 폴더에서 `LoadFiles`로 두 파일을 짝으로 읽는다. `dir`과 `root`는 먼저 `filepath.Clean`한다. `root`에서 멈춘다. `root == ""`이거나 `dir`이 `root` 밖이면 `dir` 하나만 확인한다. "밖"은 `filepath.Rel(root, dir)`이 오류를 내거나(절대·상대 경로가 섞인 경우), 결과가 `..`이거나 `".." + string(filepath.Separator)`로 시작하는 경우다(`..api` 같은 폴더 이름은 안쪽이다). 파싱 오류가 나면 `Dir`와 `Searched`를 채운 `Loaded`와 `LoadFiles`의 오류를 감싸지 않고 그대로 돌려준다.
  - `func (l Loaded) FileName() string`. `Public != nil`이면 `PublicFileName`, `Private != nil`이면 `PrivateFileName`, 둘 다 아니면 `""`.

- [ ] **Step 1: 실패하는 테스트 작성** (`internal/env/nearest_test.go`)

```go
func mkdirs(t *testing.T, parts ...string) string {
	t.Helper()
	p := filepath.Join(parts...)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	return p
}

func TestLoadNearest_NearestFolderWins(t *testing.T) {
	root := t.TempDir()
	api := mkdirs(t, root, "api")
	v1 := mkdirs(t, root, "api", "v1")
	writeFile(t, root, PublicFileName, `{"dev": {"host": "root"}}`)
	writeFile(t, api, PublicFileName, `{"dev": {"host": "api"}}`)

	got, err := LoadNearest(v1, root)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.Dir != api || got.Public["dev"]["host"] != "api" {
		t.Errorf("Dir = %q, host = %q", got.Dir, got.Public["dev"]["host"])
	}
	if want := []string{v1, api}; !reflect.DeepEqual(got.Searched, want) {
		t.Errorf("Searched = %v, want %v", got.Searched, want)
	}
}

func TestLoadNearest_WalksUpToRoot(t *testing.T) {
	root := t.TempDir()
	api := mkdirs(t, root, "api")
	writeFile(t, root, PublicFileName, `{"dev": {}}`)

	got, _ := LoadNearest(api, root)
	if got.Dir != root || got.FileName() != "http-client.env.json" {
		t.Errorf("Dir = %q, FileName = %q", got.Dir, got.FileName())
	}
}

func TestLoadNearest_PublicAndPrivateComeFromSameFolder(t *testing.T) {
	root := t.TempDir()
	api := mkdirs(t, root, "api")
	writeFile(t, root, PrivateFileName, `{"dev": {"token": "root-secret"}}`)
	writeFile(t, api, PublicFileName, `{"dev": {}}`)

	got, _ := LoadNearest(api, root)
	if got.Dir != api || got.Private != nil {
		t.Errorf("Dir = %q, Private = %v (root private must not be paired)", got.Dir, got.Private)
	}
}

func TestLoadNearest_PrivateOnlyFolderCounts(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, PrivateFileName, `{"dev": {}}`)

	got, _ := LoadNearest(root, root)
	if got.Dir != root || got.FileName() != "http-client.private.env.json" {
		t.Errorf("Dir = %q, FileName = %q", got.Dir, got.FileName())
	}
}

func TestLoadNearest_NotFoundStopsAtRoot(t *testing.T) {
	parent := t.TempDir()
	writeFile(t, parent, PublicFileName, `{"dev": {}}`)
	root := mkdirs(t, parent, "proj")
	api := mkdirs(t, root, "api")

	got, err := LoadNearest(api, root)
	if err != nil || got.Dir != "" {
		t.Fatalf("Dir = %q, err = %v; env above root must be ignored", got.Dir, err)
	}
	if want := []string{api, root}; !reflect.DeepEqual(got.Searched, want) {
		t.Errorf("Searched = %v, want %v", got.Searched, want)
	}
}

func TestLoadNearest_DirOutsideRootSearchesOnlyDir(t *testing.T) {
	dir := t.TempDir()
	for _, root := range []string{"", t.TempDir()} {
		got, _ := LoadNearest(dir, root)
		if want := []string{dir}; !reflect.DeepEqual(got.Searched, want) {
			t.Errorf("root=%q: Searched = %v, want %v", root, got.Searched, want)
		}
	}
}

func TestLoadNearest_RelativeRoot(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "api")
	writeFile(t, root, PublicFileName, `{"dev": {}}`)
	t.Chdir(root)

	for _, r := range []string{".", "./"} {
		got, err := LoadNearest("api", r)
		if err != nil || got.Dir != "." {
			t.Errorf("root=%q: Dir = %q, err = %v", r, got.Dir, err)
		}
		if want := []string{"api", "."}; !reflect.DeepEqual(got.Searched, want) {
			t.Errorf("root=%q: Searched = %v, want %v", r, got.Searched, want)
		}
	}
}

func TestLoadNearest_ParseErrorStopsSearch(t *testing.T) {
	root := t.TempDir()
	api := mkdirs(t, root, "api")
	writeFile(t, root, PublicFileName, `{"dev": {}}`)
	writeFile(t, api, PublicFileName, `{ broken`)

	got, err := LoadNearest(api, root)
	if err == nil || !strings.HasPrefix(err.Error(), "http-client.env.json: ") {
		t.Fatalf("err = %v", err)
	}
	if got.Dir != api {
		t.Errorf("Dir = %q, want %q (must not fall back to root)", got.Dir, api)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/env/ -run TestLoadNearest_`
Expected: FAIL (`undefined: LoadNearest`)

- [ ] **Step 3: `LoadNearest`와 `Loaded.FileName` 구현** (`internal/env/nearest.go`)

파일 존재 여부는 `os.Stat`으로 본다. 반복은 `cur == root`인 폴더를 확인한 뒤 멈추고, `filepath.Dir(cur) == cur`일 때도 멈춘다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/env/`
Expected: PASS

- [ ] **Step 5: CLAUDE.md 갱신**

Architecture의 `internal/env/` 항목 파일 목록 `(envfile.go, resolve.go, substitute.go)`에 `nearest.go`를 넣고, "`LoadNearest` searches from the `.http` file's folder up to the `-dir` root and reads the public/private pair from the nearest folder that has either" 한 문장을 더한다.

- [ ] **Step 6: 커밋**

```bash
git add internal/env/nearest.go internal/env/nearest_test.go CLAUDE.md
git commit -m "env: search env files from the request folder up to the root"
```

---

### Task 4: Request view가 가장 가까운 env를 읽고 env 줄에 출처를 표시 (P2-3, P2-4)

**Files:**
- Modify: `internal/tui/requestview.go` (`requestModel` 필드, `newRequestModel`, `envLine`, 새 함수 `dirLabel`, `canCycleEnv`)
- Modify: `internal/tui/app.go` (`NewApp`, `App.Update`의 `openRequestMsg` 분기)
- Modify: `README.md` "Environment configuration" 첫 문장, `CLAUDE.md` 마지막 문단의 env 파일 위치 문장
- Modify: `internal/tui/*_test.go`의 모든 `newRequestModel(` 호출(시그니처 이전)
- Test: `internal/tui/requestview_test.go`, `internal/tui/app_test.go`

**Interfaces:**
- Consumes: Task 3 `env.LoadNearest`, `env.Loaded`, `Loaded.FileName()`. Task 2 `env.EnvNames`, `env.Merge`. P1 `assertViewFits`.
- Produces:
  - `newRequestModel(filePath, root string, file *httpfile.File, req httpfile.Request, store *history.Store, preferredEnv string) requestModel`. 이 태스크에서는 `preferredEnv`를 받기만 하고 쓰지 않는다(Task 5에서 사용).
  - `func dirLabel(root, dir string) string`: 문자열 비교가 아니라 `filepath.Rel(root, dir)`로 판단한다(`-dir ./api/`처럼 끝에 `/`가 붙거나 정리되지 않은 root도 `Searched`·`Dir`의 Clean한 값과 맞아야 한다). 결과가 `.`이면 `"./"`, root 아래면 슬래시로 바꾼 상대 경로 + `"/"`(`"api/"`, `"api/v1/"`), `root == ""`이거나 root 밖이면(Task 3의 "밖" 규칙) `filepath.ToSlash(filepath.Clean(dir)) + "/"`.
  - `func (m requestModel) canCycleEnv() bool`: `len(m.envNames) > 1`
  - env 줄 문구(`envLine`):
    - 찾았고 env가 있음: `env: <name> (<i>/<n>) · <dirLabel(root, Dir)><FileName()>`, `canCycleEnv()`이면 ` — e cycle`을 붙인다.
    - 찾았지만 env 이름이 없음(`$shared`만 있거나 `{}`): `env: (none) · <dirLabel><FileName()>`
    - 못 찾음: `no http-client.env.json found (searched: <dirLabel들을 ", "로 연결>)` (muted)
    - 파싱 실패: `errorTextStyle`로 `err.Error()`(Task 1에서 이미 `<파일명>: <오류>` 형식)
    - 어떤 경우든 `m.viewport.Width > 0`이면 `ansi.Truncate(line, m.viewport.Width, "…")`(`github.com/charmbracelet/x/ansi`)로 한 줄을 유지한다(Global Constraints).
  - `requestModel` 필드: `root string`, `envSource env.Loaded`, `envErr error`를 더하고, 기존 `publicEnv, privateEnv`를 지운다(`recompute`는 `m.envSource.Public`/`m.envSource.Private`를 쓴다). 이후 태스크는 이 필드 대신 `envLine()`과 `canCycleEnv()`를 쓴다.

- [ ] **Step 1: 시그니처만 바꾸고 기존 호출 이전**

`newRequestModel`의 시그니처를 Interfaces의 6인자로 바꾸고 `root`는 `m.root`에 담기만 한다(env 로딩은 아직 `env.LoadFiles(filepath.Dir(filePath))` 그대로). 호출을 이렇게 옮긴다.

- `internal/tui/*_test.go`의 `newRequestModel(a, b, c, d)` → `newRequestModel(a, "", b, c, d, "")`
- `NewApp`: `newRequestModel("", "", &httpfile.File{}, httpfile.Request{}, store, "")`
- `App.Update`의 `openRequestMsg` 분기: `newRequestModel(msg.filePath, a.root, msg.file, msg.req, a.store, "")`

`root == ""`이면 Step 4 이후에도 `dir` 하나만 확인하므로 기존 테스트의 동작은 그대로다.

Run: `grep -n 'newRequestModel(' internal/tui/*.go && go test ./internal/tui/`
Expected: 정의와 모든 호출이 인자 6개이고, 테스트가 PASS(동작 변화 없음).

- [ ] **Step 2: 실패하는 테스트 작성**

`requestview_test.go`에 헬퍼와 테스트를 추가한다. P1이 같은 역할의 헬퍼를 이미 만들었다면 그것을 쓴다.

```go
func writeNestedFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func typeRunes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func plainText(s string) string { return ansiEscapePattern.ReplaceAllString(s, "") }

const threeEnvs = `{"dev": {"host": "https://dev.example.com"}, "prod": {"host": "https://prod.example.com"}, "staging": {"host": "https://staging.example.com"}}`

func openAt(t *testing.T, root, rel, preferred string) requestModel {
	t.Helper()
	req := httpfile.Request{Method: "GET", URL: "{{host}}/x"}
	return newRequestModel(filepath.Join(root, rel), root, &httpfile.File{}, req, newTestHistoryStore(t), preferred).SetSize(120, 30)
}

func TestDirLabel(t *testing.T) {
	cases := []struct{ root, dir, want string }{
		{"/r", "/r", "./"},
		{"/r/", "/r", "./"},
		{"/r", "/r/api/v1", "api/v1/"},
		{".", "api", "api/"},
		{"./", ".", "./"},
		{"", "/x", "/x/"},
		{"/r", "/other", "/other/"},
	}
	for _, c := range cases {
		if got := dirLabel(c.root, c.dir); got != c.want {
			t.Errorf("dirLabel(%q, %q) = %q, want %q", c.root, c.dir, got, c.want)
		}
	}
}

func TestRequestModel_EnvLineFromAncestorFolder(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), threeEnvs)

	m := openAt(t, root, "api/users.http", "")

	if m.resolved.URL != "https://dev.example.com/x" {
		t.Errorf("resolved.URL = %q", m.resolved.URL)
	}
	if got, want := plainText(m.envLine()), "env: dev (1/3) · ./http-client.env.json — e cycle"; got != want {
		t.Errorf("envLine = %q, want %q", got, want)
	}
}

func TestRequestModel_EnvLineShowsNearestFileRelativeToRoot(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "api", "http-client.env.json"), threeEnvs)

	m := openAt(t, root, "api/users.http", "")
	if got, want := plainText(m.envLine()), "env: dev (1/3) · api/http-client.env.json — e cycle"; got != want {
		t.Errorf("envLine = %q, want %q", got, want)
	}
}

func TestRequestModel_EnvLineSingleEnvHasNoCycleHint(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), `{"dev": {}}`)

	m := openAt(t, root, "a.http", "")
	if got, want := plainText(m.envLine()), "env: dev (1/1) · ./http-client.env.json"; got != want {
		t.Errorf("envLine = %q, want %q", got, want)
	}
	if m.canCycleEnv() {
		t.Error("canCycleEnv() = true with a single env")
	}
}

func TestRequestModel_EnvLineSharedOnly(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), `{"$shared": {"host": "https://shared.example.com"}}`)

	m := openAt(t, root, "a.http", "")
	if m.resolved.URL != "https://shared.example.com/x" {
		t.Errorf("resolved.URL = %q", m.resolved.URL)
	}
	if got, want := plainText(m.envLine()), "env: (none) · ./http-client.env.json"; got != want {
		t.Errorf("envLine = %q, want %q", got, want)
	}
}

func TestRequestModel_EnvLineListsSearchedFoldersWhenNotFound(t *testing.T) {
	root := t.TempDir()
	m := openAt(t, root, "api/users.http", "")
	if got, want := plainText(m.envLine()), "no http-client.env.json found (searched: api/, ./)"; got != want {
		t.Errorf("envLine = %q, want %q", got, want)
	}
}

func TestRequestModel_EnvLineShowsParseError(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), `{ broken`)

	m := openAt(t, root, "a.http", "")
	if got := plainText(m.envLine()); !strings.HasPrefix(got, "http-client.env.json: ") {
		t.Errorf("envLine = %q, want prefix %q", got, "http-client.env.json: ")
	}
}
```

`app_test.go`:

```go
func TestApp_OpenRequestSearchesEnvUpToAppRoot(t *testing.T) {
	a := newTestApp(t)
	writeNestedFile(t, filepath.Join(a.root, "http-client.env.json"), threeEnvs)

	model, _ := a.Update(openRequestMsg{filePath: filepath.Join(a.root, "api", "users.http"), file: &httpfile.File{}, req: httpfile.Request{Method: "GET", URL: "{{host}}/x"}})
	a = model.(App)

	if a.request.resolved.URL != "https://dev.example.com/x" {
		t.Errorf("resolved.URL = %q", a.request.resolved.URL)
	}
}

func TestApp_EnvLineFitsTerminalWithDeepEnvPath(t *testing.T) {
	a := newTestApp(t)
	deep := filepath.Join(a.root, "services", "payments", "internal", "api", "v1")
	writeNestedFile(t, filepath.Join(deep, "http-client.env.json"), `{"staging-eu-west": {}, "dev": {}, "qa": {}}`)
	model, _ := a.Update(openRequestMsg{filePath: filepath.Join(deep, "a.http"), file: &httpfile.File{}, req: httpfile.Request{Method: "GET", URL: "https://example.com"}})
	a = model.(App)

	for _, sz := range [][2]int{{60, 20}, {80, 24}, {120, 35}} {
		model, _ = a.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		assertViewFits(t, model.(App).View(), sz[0], sz[1])
	}
}
```

- [ ] **Step 3: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestDirLabel|TestRequestModel_EnvLine|TestApp_OpenRequestSearchesEnvUpToAppRoot|TestApp_EnvLineFitsTerminalWithDeepEnvPath'`
Expected: FAIL. 먼저 `undefined: dirLabel`, `m.canCycleEnv undefined` 컴파일 오류가 난다. 그 밖의 원인은 env 줄이 기존 `env: dev (1/3) — press e to cycle`/`no environment file found` 형식이고 상위 폴더의 env를 읽지 않는 것이다. `TestApp_EnvLineFitsTerminalWithDeepEnvPath`는 기존 문구가 짧아 지금은 통과할 수 있다. 새 문구가 60열보다 길어도 레이아웃을 깨지 않는지 지키는 회귀 테스트다.

- [ ] **Step 4: 구현**

- `newRequestModel`: `filePath != ""`이면 `env.LoadNearest(filepath.Dir(filePath), root)`를 부르고, 결과를 `m.envSource`, 오류를 `m.envErr`에 담는다. 오류가 없을 때만 `envNames`를 채운다. Step 1에서 남긴 `env.LoadFiles(filepath.Dir(filePath))` 호출을 대체한다.
- `envLine`, `dirLabel`, `canCycleEnv`를 Interfaces의 규칙대로 만든다.

- [ ] **Step 5: README·CLAUDE.md 문장 갱신**

- `README.md` "Environment configuration" 첫 문장의 "`.http` 파일이 있는 디렉터리에"를 "`.http` 파일이 있는 디렉터리부터 `-dir` 루트까지 위로 올라가며 가장 가까운 디렉터리의"로 고친다. 탐색 규칙 전체와 지원표는 P3-8이 쓴다.
- `CLAUDE.md` 마지막 문단의 "the two IntelliJ env JSON files sitting alongside the `.http` files"를 "the two IntelliJ env JSON files in the nearest folder at or above the `.http` file (up to the `-dir` root)"로 고친다.

- [ ] **Step 6: 통과 확인**

Run: `go test ./internal/tui/ && go vet ./... && gofmt -l .`
Expected: PASS, vet 경고 없음, gofmt 출력 없음

- [ ] **Step 7: 커밋**

```bash
git add internal/tui/ README.md CLAUDE.md
git commit -m "tui: load env files from the nearest ancestor folder and show their source"
```

---

### Task 5: 세션 동안 env 유지, prod 경고 색 (P2-5)

**Files:**
- Modify: `internal/tui/requestview.go` (`newRequestModel`, 새 메서드 `currentEnv`, `envLineStyle`, `envLine`)
- Modify: `internal/tui/app.go` (`App` 필드 `lastEnv`, `App.Update`의 `openRequestMsg` 분기와 `screenRequest` 위임)
- Modify: `internal/tui/styles.go` (`colorWarning`, `warnTextStyle`)
- Test: `internal/tui/requestview_test.go`, `internal/tui/app_test.go`

**Interfaces:**
- Consumes: Task 4 `newRequestModel` 6인자 시그니처, `openAt`, `threeEnvs`, `typeRunes`
- Produces:
  - `func (m requestModel) currentEnv() string`: 선택한 env 이름. env 목록이 비어 있으면 `""`.
  - `newRequestModel`: `preferredEnv`가 `envNames`에 있으면 그 env를, 없으면 `envNames[0]`(알파벳순 첫 env)을 고른다.
  - `App.lastEnv string`: `App.Update`가 `screenRequest`에 메시지를 넘긴 직후 `a.request.currentEnv()`가 `""`이 아니면 그 값으로 갱신한다. `openRequestMsg` 분기는 `newRequestModel(..., a.lastEnv)`로 넘긴다.
  - `colorWarning = lipgloss.Color("11")`, `warnTextStyle = lipgloss.NewStyle().Foreground(colorWarning)`. P4-5가 다시 쓴다.
  - `func (m requestModel) envLineStyle() lipgloss.Style`: `strings.Contains(strings.ToLower(m.currentEnv()), "prod")`이면 `warnTextStyle`, 아니면 `lipgloss.NewStyle()`. `envLine`의 "찾았고 env가 있음" 문구에 적용한다.

- [ ] **Step 1: 실패하는 테스트 작성**

`requestview_test.go`:

```go
func TestNewRequestModel_PreferredEnvWinsWhenPresent(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), threeEnvs)

	if got := openAt(t, root, "a.http", "staging").currentEnv(); got != "staging" {
		t.Errorf("currentEnv = %q, want staging", got)
	}
	if got := openAt(t, root, "a.http", "qa").currentEnv(); got != "dev" {
		t.Errorf("currentEnv = %q, want dev (alphabetical first)", got)
	}
}

func TestRequestModel_ProdEnvUsesWarningStyle(t *testing.T) {
	for name, warn := range map[string]bool{"prod": true, "Production": true, "preprod-eu": true, "staging": false} {
		root := t.TempDir()
		writeNestedFile(t, filepath.Join(root, "http-client.env.json"), `{"`+name+`": {}}`)
		m := openAt(t, root, "a.http", "")
		if got := m.envLineStyle().GetForeground() == colorWarning; got != warn {
			t.Errorf("%s: warning style = %v, want %v", name, got, warn)
		}
	}
}
```

`app_test.go`:

```go
func TestApp_SelectedEnvCarriesToNextRequest(t *testing.T) {
	a := newTestApp(t)
	writeNestedFile(t, filepath.Join(a.root, "http-client.env.json"), threeEnvs)
	writeNestedFile(t, filepath.Join(a.root, "qa", "http-client.env.json"), `{"dev": {}, "qa": {}}`)
	open := func(a App, path string) App {
		model, _ := a.Update(openRequestMsg{filePath: path, file: &httpfile.File{}, req: httpfile.Request{Method: "GET", URL: "https://example.com"}})
		return model.(App)
	}
	press := func(a App, s string) App {
		model, _ := a.Update(typeRunes(s))
		return model.(App)
	}

	a = press(open(a, filepath.Join(a.root, "a.http")), "e")
	if a.request.currentEnv() != "prod" || a.lastEnv != "prod" {
		t.Fatalf("currentEnv = %q, lastEnv = %q, want prod", a.request.currentEnv(), a.lastEnv)
	}
	if got := open(a, filepath.Join(a.root, "b.http")).request.currentEnv(); got != "prod" {
		t.Errorf("next request env = %q, want prod", got)
	}

	a = press(open(a, filepath.Join(t.TempDir(), "noenv.http")), "j")
	if a.lastEnv != "prod" {
		t.Errorf("lastEnv = %q after a request with no env file, want prod kept", a.lastEnv)
	}

	if got := open(a, filepath.Join(a.root, "qa", "c.http")).request.currentEnv(); got != "dev" {
		t.Errorf("env in folder without prod = %q, want dev", got)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestNewRequestModel_PreferredEnvWinsWhenPresent|TestRequestModel_ProdEnvUsesWarningStyle|TestApp_SelectedEnvCarriesToNextRequest'`
Expected: FAIL (`currentEnv`, `envLineStyle`, `colorWarning`, `lastEnv` 미정의)

- [ ] **Step 3: 구현**

Interfaces의 규칙대로 구현한다. `lastEnv` 갱신은 `App.Update` 끝의 `switch a.screen`에서 `case screenRequest:` 위임 직후에 한다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/tui/requestview.go internal/tui/app.go internal/tui/styles.go internal/tui/requestview_test.go internal/tui/app_test.go
git commit -m "tui: keep the selected env for the session and flag prod envs"
```

---

### Task 6: 해결되지 않은 변수가 있을 때 전송 차단 안내 (P2-6)

**Files:**
- Modify: `internal/tui/requestview.go` (`canSend`, `blockedMessage`, `applyEdit`, `Update`의 `keys.Send` 분기, `statusLine`, `buildContent`)
- Modify: `internal/tui/app.go` (`footerHints`의 Request view 힌트 slice)
- Modify: `internal/tui/styles.go` (`blockedHighlightStyle`)
- Test: `internal/tui/requestview_test.go`, `internal/tui/app_test.go`

**Interfaces:**
- Consumes: P1 `statusLine()`, `hint`, `renderHints`, `assertViewFits`. Task 4 헬퍼.
- Produces:
  - `func (m requestModel) canSend() bool`: `len(m.missingVars) == 0`. 전송 중 여부는 따로 본다. P3는 `missing file: …` 항목을 `missingVars`에 합쳐 이 판정을 그대로 쓴다.
  - `func (m requestModel) blockedMessage() string`: `"cannot send: unresolved " + {{name}}들을 ", "로 연결 + " — e switch env · i edit"`.
  - `requestModel.blockedFlash bool`: `!canSend()`인 상태에서 `keys.Send`를 누르면 `true`, 그 다음 아무 키 입력이 오면 `false`.
  - `statusLine` 우선순위에 두 항목을 넣는다(편집 중이 아닐 때만. 편집 중에는 P1 그대로 `editError`만 보인다). `blockedFlash`일 때의 `blockedHighlightStyle.Render(blockedMessage())`가 가장 앞이고, `!canSend()`일 때의 `errorTextStyle.Render(blockedMessage())`는 복사 알림 다음이다.
  - `applyEdit`: 성공하면 `missingVars`를 비우는 대신 `_, m.missingVars = env.ResolveRequest(m.resolved, nil)`로 편집 결과에 남은 `{{…}}`를 해결되지 않은 변수로 다시 모은다. `m.resolved`는 편집한 글자 그대로 둔다. 함수 위 주석의 "clears m.missingVars" 설명도 이에 맞게 고친다.
  - 테스트 헬퍼 `func applyTestEdit(t *testing.T, m requestModel, text string) requestModel` (`requestview_test.go`): `i` → `editor.SetValue(text)` → `ctrl+s`. Task 7이 다시 쓴다.
  - `buildContent`: 머리 부분의 `unresolved variables: …` 줄을 지운다(상태 줄과 같은 내용이 두 번 나오지 않게). `!canSend()`이고 아직 보내지 않았으면 응답 자리 문구를 `(not yet sent)`로 바꾼다.
  - footer: `!a.request.canSend()`이면 Request view 힌트 slice에서 send 힌트를 뺀다.
  - `blockedHighlightStyle = errorTextStyle.Bold(true).Reverse(true)`

- [ ] **Step 1: 실패하는 테스트 작성**

`requestview_test.go`:

```go
func TestRequestModel_UnresolvedVarsBlockSendWithGuidance(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "{{host}}/x", Headers: []httpfile.Header{{Name: "Authorization", Value: "Bearer {{token}}"}}}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "").SetSize(200, 30)
	want := "cannot send: unresolved {{host}}, {{token}} — e switch env · i edit"

	if m.canSend() {
		t.Fatal("canSend() = true with unresolved vars")
	}
	if got := plainText(m.statusLine()); !strings.Contains(got, want) {
		t.Errorf("statusLine = %q, want it to contain %q", got, want)
	}
	if content, _ := m.buildContent(); strings.Contains(content, "press enter to send") || strings.Contains(content, "unresolved variables:") {
		t.Errorf("content still invites sending or repeats the notice: %q", plainText(content))
	}

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.executing || !m.blockedFlash {
		t.Fatalf("after enter: cmd=%v executing=%v blockedFlash=%v", cmd != nil, m.executing, m.blockedFlash)
	}
	if got := plainText(m.statusLine()); !strings.Contains(got, want) {
		t.Errorf("statusLine after enter = %q", got)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.blockedFlash {
		t.Error("blockedFlash should clear on the next key")
	}
}

func TestRequestModel_CanSendAfterSwitchingToCompleteEnv(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), `{"a": {}, "b": {"host": "https://b.example.com"}}`)
	m := openAt(t, root, "x.http", "")
	if m.canSend() {
		t.Fatal("env a lacks host; canSend() should be false")
	}
	m, _ = m.Update(typeRunes("e"))
	if !m.canSend() {
		t.Error("env b defines host; canSend() should be true")
	}
}

func applyTestEdit(t *testing.T, m requestModel, text string) requestModel {
	t.Helper()
	m, _ = m.Update(typeRunes("i"))
	m.editor.SetValue(text)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.editing {
		t.Fatalf("edit not applied: %s", m.editError)
	}
	return m
}

func TestRequestModel_ApplyEditKeepsLeftoverPlaceholdersBlocked(t *testing.T) {
	req := httpfile.Request{Method: "GET", URL: "{{host}}/x"}
	m := newRequestModel("", "", &httpfile.File{}, req, newTestHistoryStore(t), "").SetSize(200, 30)

	m = applyTestEdit(t, m, "GET https://ok.example.com/x\nAuthorization: Bearer {{token}}\n")
	if m.canSend() {
		t.Fatal("canSend() = true; the edit still contains {{token}}")
	}
	if got := plainText(m.statusLine()); !strings.Contains(got, "cannot send: unresolved {{token}} — e switch env · i edit") {
		t.Errorf("statusLine = %q", got)
	}

	m = applyTestEdit(t, m, "GET https://ok.example.com/x\nAuthorization: Bearer abc\n")
	if !m.canSend() {
		t.Errorf("canSend() = false after removing every placeholder; missingVars = %v", m.missingVars)
	}
}
```

`app_test.go`:

```go
func footerLine(a App) string {
	lines := strings.Split(a.View(), "\n")
	return plainText(lines[len(lines)-1])
}

func TestApp_FooterHidesSendWhenBlocked(t *testing.T) {
	a := newTestApp(t)
	model, _ := a.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	model, _ = model.(App).Update(openRequestMsg{filePath: filepath.Join(a.root, "a.http"), file: &httpfile.File{}, req: httpfile.Request{Method: "GET", URL: "{{host}}/x"}})
	a = model.(App)

	if got := footerLine(a); strings.Contains(got, "send") {
		t.Errorf("footer = %q, want no send hint", got)
	}
	for _, sz := range [][2]int{{60, 20}, {80, 24}, {120, 35}} {
		model, _ = a.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		assertViewFits(t, model.(App).View(), sz[0], sz[1])
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestRequestModel_UnresolvedVarsBlockSendWithGuidance|TestRequestModel_CanSendAfterSwitchingToCompleteEnv|TestRequestModel_ApplyEditKeepsLeftoverPlaceholdersBlocked|TestApp_FooterHidesSendWhenBlocked'`
Expected: FAIL (`canSend`, `blockedFlash` 미정의)

- [ ] **Step 3: 구현**

Interfaces의 규칙대로 구현한다. `keys.Send` 분기는 `m.executing`이면 지금처럼 무시하고, `!m.canSend()`이면 `blockedFlash = true`만 하고 `nil` cmd를 돌려준다. `blockedFlash`를 지우는 처리는 `tea.KeyMsg` 분기 맨 앞, `m.editing` 검사보다 먼저 한다(Send 분기가 다시 켠다).

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/tui/`
Expected: PASS. 기존 `TestNewRequestModel_MissingVarsBlockSend`, `TestRequestModel_CopyTextNotYetSent`(`copyText`의 문구는 바꾸지 않는다), `TestRequestModel_ApplyEditValidUpdatesResolvedAndClearsMissingVars`(편집 내용에 `{{…}}`가 없다)도 통과한다.

- [ ] **Step 5: 커밋**

```bash
git add internal/tui/requestview.go internal/tui/app.go internal/tui/styles.go internal/tui/requestview_test.go internal/tui/app_test.go
git commit -m "tui: explain why a request with unresolved variables cannot be sent"
```

---

### Task 7: 편집을 적용한 뒤 env 전환은 두 번 눌러야 함 (P2-7)

**Files:**
- Modify: `internal/tui/requestview.go` (`applyEdit`, `Update`의 `keys.CycleEnv` 분기, `statusLine`)
- Test: `internal/tui/requestview_test.go`

**Interfaces:**
- Consumes: Task 4 `canCycleEnv`, `openAt`, `typeRunes`. Task 5 `currentEnv`. Task 6 `applyTestEdit`, `statusLine` 우선순위.
- Produces: `requestModel.edited bool`(`applyEdit` 성공 시 `true`, env를 실제로 바꾸면 `false`), `requestModel.discardArmed bool`. `edited && canCycleEnv()`인 상태에서 처음 누른 `e`는 `discardArmed = true`만 하고, 두 번째 `e`가 env를 바꾼다. 그 사이에 다른 키를 누르면 `discardArmed = false`가 된다. `discardArmed`이면 `statusLine`은 `blockedFlash`보다도 먼저 `edits will be discarded — press e again`(경고 색 `warnTextStyle`)을 보여 준다. 최종 우선순위: `discardArmed` > `blockedFlash` > 복사 알림 > `!canSend()` 안내.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
func TestRequestModel_CycleEnvAfterEditNeedsSecondPress(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), threeEnvs)
	m := applyTestEdit(t, openAt(t, root, "a.http", ""), "GET https://edited.example.com/x\n")

	m, _ = m.Update(typeRunes("e"))
	if m.currentEnv() != "dev" || m.resolved.URL != "https://edited.example.com/x" {
		t.Fatalf("first e changed state: env=%q url=%q", m.currentEnv(), m.resolved.URL)
	}
	if got := plainText(m.statusLine()); !strings.Contains(got, "edits will be discarded — press e again") {
		t.Errorf("statusLine = %q", got)
	}

	m, _ = m.Update(typeRunes("e"))
	if m.currentEnv() != "prod" || m.resolved.URL != "https://prod.example.com/x" {
		t.Errorf("second e: env=%q url=%q", m.currentEnv(), m.resolved.URL)
	}
	if got := plainText(m.statusLine()); strings.Contains(got, "edits will be discarded") {
		t.Errorf("prompt still shown after switching: %q", got)
	}
}

func TestRequestModel_OtherKeyDisarmsEditDiscard(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), threeEnvs)
	m := applyTestEdit(t, openAt(t, root, "a.http", ""), "GET https://edited.example.com/x\n")

	m, _ = m.Update(typeRunes("e"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(typeRunes("e"))
	if m.currentEnv() != "dev" {
		t.Errorf("env = %q, want dev (prompt must re-arm after another key)", m.currentEnv())
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestRequestModel_CycleEnvAfterEditNeedsSecondPress|TestRequestModel_OtherKeyDisarmsEditDiscard'`
Expected: FAIL. 첫 `e`에서 바로 `prod`로 바뀌고 편집한 URL이 사라진다.

- [ ] **Step 3: 구현**

Interfaces의 규칙대로 구현한다. `discardArmed`를 지우는 처리는 Task 6의 `blockedFlash` 해제와 같은 자리(`tea.KeyMsg` 분기 맨 앞)에서 하되, `keys.CycleEnv`에는 적용하지 않는다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/tui/`
Expected: PASS (기존 `TestRequestModel_CycleEnvRecomputesResolvedVars`는 편집이 없으므로 그대로 통과)

- [ ] **Step 5: 커밋**

```bash
git add internal/tui/requestview.go internal/tui/requestview_test.go
git commit -m "tui: confirm before an env switch discards applied edits"
```

---

### Task 8: 히스토리에 env 기록과 표시 (P2-8)

**Files:**
- Modify: `internal/history/store.go:17-32` (`Entry`)
- Modify: `internal/tui/requestview.go` (`requestModel.entryEnv`, `newRequestModelFromEntry`, `newRequestModelFromEntryNoSend`, `sendCmd`, `buildContent`)
- Modify: `internal/tui/historyview.go` (`entryItem.Title`, `renderEntryDetail`)
- Test: `internal/history/store_test.go`, `internal/tui/requestview_test.go`, `internal/tui/historyview_test.go`

**Interfaces:**
- Consumes: Task 5 `currentEnv`. P1 `execResultMsg.modelID`, `requestModel.id`.
- Produces:
  - `history.Entry.Env string` (태그 `json:"Env,omitempty"`)
  - `requestModel.entryEnv string`: 히스토리에서 만든 모델은 `entry.Env`를 담는다. `sendCmd`는 `Env`에 `m.currentEnv()`를 기록하고, 그 값이 `""`이면 `m.entryEnv`를 기록한다.
  - `buildContent`: 응답이 있고 `m.currentEnv() != ""`이며 `m.lastEntry.Env != m.currentEnv()`이면, 응답 렌더링 바로 위에 muted로 `(sent with env: <lastEntry.Env>)` 한 줄을 넣는다.
  - `entryItem.Title`: `Env != ""`이면 시각 다음에 `[<env>]`을 넣는다(`15:04:05  [dev] GET … -> 200`). 줄 순서 재배치는 P4-10이 한다.
  - `renderEntryDetail`: `Env != ""`이면 첫 줄에 muted로 `env: <env>`를 넣는다.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/history/store_test.go`:

```go
func TestStore_EnvRoundTripsAndIsOmittedWhenEmpty(t *testing.T) {
	s, path := newTestStore(t)
	if _, err := s.Append(Entry{Method: "GET", URL: "https://example.com"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), `"Env"`) {
		t.Errorf("empty Env should be omitted: %s", data)
	}
	if _, err := s.Append(Entry{Method: "GET", URL: "https://example.com", Env: "dev"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, _ := s.List(1)
	if got[0].Env != "dev" {
		t.Errorf("Env = %q, want dev", got[0].Env)
	}
}
```

`store_test.go`의 import에 `strings`를 추가한다.

`internal/tui/requestview_test.go`:

```go
func TestRequestModel_SendRecordsEnvInHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), `{"dev": {"host": "`+srv.URL+`"}}`)
	store := newTestHistoryStore(t)
	m := newRequestModel(filepath.Join(root, "a.http"), root, &httpfile.File{}, httpfile.Request{Method: "GET", URL: "{{host}}/x"}, store, "")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := cmd().(execResultMsg)
	if msg.entry.Env != "dev" {
		t.Errorf("entry.Env = %q, want dev", msg.entry.Env)
	}
	if got, _ := store.List(1); got[0].Env != "dev" {
		t.Errorf("stored Env = %q, want dev", got[0].Env)
	}
}

func TestRequestModel_RerunRecordsOriginalEnv(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	_, cmd := newRequestModelFromEntry(history.Entry{Method: "GET", URL: srv.URL, Env: "staging"}, newTestHistoryStore(t))
	if got := cmd().(execResultMsg).entry.Env; got != "staging" {
		t.Errorf("entry.Env = %q, want staging", got)
	}
}

func TestRequestModel_SentWithEnvShownAfterSwitch(t *testing.T) {
	root := t.TempDir()
	writeNestedFile(t, filepath.Join(root, "http-client.env.json"), threeEnvs)
	m := openAt(t, root, "a.http", "")
	m, _ = m.Update(execResultMsg{modelID: m.id, entry: history.Entry{Env: "dev", StatusCode: 200, Status: "200 OK"}})

	if content, _ := m.buildContent(); strings.Contains(content, "sent with env") {
		t.Errorf("note shown before switching env: %q", plainText(content))
	}
	m, _ = m.Update(typeRunes("e"))
	if content, _ := m.buildContent(); !strings.Contains(plainText(content), "(sent with env: dev)") {
		t.Errorf("content = %q, want (sent with env: dev)", plainText(content))
	}
}
```

`internal/tui/historyview_test.go`:

```go
func TestHistoryView_ShowsEnvInListAndDetail(t *testing.T) {
	e := history.Entry{Method: "GET", URL: "https://example.com", StatusCode: 200, Env: "dev"}
	if got := (entryItem{entry: e}).Title(); !strings.Contains(got, "[dev]") {
		t.Errorf("Title = %q, want [dev]", got)
	}
	if got := plainText(renderEntryDetail(e)); !strings.Contains(got, "env: dev") {
		t.Errorf("detail = %q, want env: dev", got)
	}
	e.Env = ""
	if got := (entryItem{entry: e}).Title(); strings.Contains(got, "[") {
		t.Errorf("Title = %q, want no env marker", got)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/history/ -run TestStore_EnvRoundTripsAndIsOmittedWhenEmpty; go test ./internal/tui/ -run 'TestRequestModel_SendRecordsEnvInHistory|TestRequestModel_RerunRecordsOriginalEnv|TestRequestModel_SentWithEnvShownAfterSwitch|TestHistoryView_ShowsEnvInListAndDetail'`
Expected: FAIL (`Entry.Env` 미정의)

- [ ] **Step 3: 구현**

Interfaces의 규칙대로 구현한다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/history/ internal/tui/requestview.go internal/tui/historyview.go internal/tui/requestview_test.go internal/tui/historyview_test.go
git commit -m "history: record and show the env each request was sent with"
```

---

### Task 9: 히스토리에서 연 요청의 env 줄과 footer (P2-9)

**Files:**
- Modify: `internal/tui/requestview.go` (`envLine`)
- Modify: `internal/tui/app.go` (`footerHints`의 Request view 힌트 slice)
- Test: `internal/tui/requestview_test.go`, `internal/tui/app_test.go`

**Interfaces:**
- Consumes: Task 4 `canCycleEnv`, Task 6 `footerLine`, Task 8 `entryEnv`
- Produces:
  - `requestModel.fromEntry bool`: `newRequestModelFromEntry`와 `newRequestModelFromEntryNoSend`가 `true`로 만든다.
  - `envLine`: `fromEntry`이면 `entryEnv != ""`일 때 `resolved at send time (env: <entryEnv>)`, 비어 있으면 `resolved at send time`(muted).
  - footer: `!a.request.canCycleEnv()`이면 Request view 힌트 slice에서 cycle env 힌트(현재 footer 문구 `e cycle env`)를 뺀다(히스토리·Recent에서 연 요청, env 파일 없음, env 1개). P1이 이 힌트의 desc를 바꿨다면 테스트의 `"cycle env"`도 그 문구로 맞춘다.

- [ ] **Step 1: 실패하는 테스트 작성**

`requestview_test.go`:

```go
func TestRequestModelFromEntry_EnvLineShowsSendTimeEnv(t *testing.T) {
	store := newTestHistoryStore(t)
	m := newRequestModelFromEntryNoSend(history.Entry{Method: "GET", URL: "https://example.com", Env: "dev"}, store)
	if got, want := plainText(m.envLine()), "resolved at send time (env: dev)"; got != want {
		t.Errorf("envLine = %q, want %q", got, want)
	}
	m, _ = newRequestModelFromEntry(history.Entry{Method: "GET", URL: "https://example.com"}, store)
	if got, want := plainText(m.envLine()), "resolved at send time"; got != want {
		t.Errorf("envLine = %q, want %q", got, want)
	}
}
```

`app_test.go`:

```go
func TestApp_FooterHidesCycleEnvWhenEnvCannotChange(t *testing.T) {
	a := newTestApp(t)
	writeNestedFile(t, filepath.Join(a.root, "http-client.env.json"), threeEnvs)
	writeNestedFile(t, filepath.Join(a.root, "one", "http-client.env.json"), `{"dev": {}}`)
	model, _ := a.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	a = model.(App)
	req := httpfile.Request{Method: "GET", URL: "https://example.com"}

	cases := []struct {
		name string
		msg  tea.Msg
		want bool
	}{
		{"three envs", openRequestMsg{filePath: filepath.Join(a.root, "a.http"), file: &httpfile.File{}, req: req}, true},
		{"one env", openRequestMsg{filePath: filepath.Join(a.root, "one", "a.http"), file: &httpfile.File{}, req: req}, false},
		{"recent entry", openRequestFromEntryMsg{entry: history.Entry{Method: "GET", URL: "https://example.com", Env: "dev"}}, false},
	}
	for _, tc := range cases {
		model, _ := a.Update(tc.msg)
		if got := strings.Contains(footerLine(model.(App)), "cycle env"); got != tc.want {
			t.Errorf("%s: footer has cycle env = %v, want %v", tc.name, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/tui/ -run 'TestRequestModelFromEntry_EnvLineShowsSendTimeEnv|TestApp_FooterHidesCycleEnvWhenEnvCannotChange'`
Expected: FAIL. env 줄이 `no http-client.env.json found (searched: )` 형식이고, footer에 늘 `cycle env`가 있다.

- [ ] **Step 3: 구현**

Interfaces의 규칙대로 구현한다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: PASS, 출력 없음

- [ ] **Step 5: 커밋**

```bash
git add internal/tui/requestview.go internal/tui/app.go internal/tui/requestview_test.go internal/tui/app_test.go
git commit -m "tui: show the send-time env for requests opened from history"
```

---

### Task 10: 실제 바이너리 확인과 dev 바이너리 재빌드 (P2-1~P2-9 확인)

**Files:**
- Modify (재생성): `rest-tui`

**Interfaces:** 없음. 코드 변경 없음.

- [ ] **Step 1: 바이너리 빌드**

Run: `scripts/build.sh`
Expected: `go vet ./...` 통과 후 `rest-tui`가 갱신된다. 이 바이너리로 아래를 확인하고 그대로 커밋한다.

- [ ] **Step 2: fixture와 에코 서버 준비** (저장소 밖 고정 경로)

Bash 호출 사이에 셸 변수가 남지 않을 수 있고, zsh는 `$T`처럼 공백이 든 변수를 단어로 나누지 않는다. 그래서 경로를 고정하고 tmux는 래퍼 스크립트 `$FIX/t`로 부른다. 이후 모든 블록의 첫 줄은 `FIX="${TMPDIR:-/tmp}/rest-tui-p2"; T="$FIX/t"`다.

```bash
FIX="${TMPDIR:-/tmp}/rest-tui-p2"; T="$FIX/t"; REPO=$(git rev-parse --show-toplevel)
rm -rf "$FIX"; mkdir -p "$FIX/p2/api" "$FIX/p2/broken" "$FIX/none/api" "$FIX/home"
printf '#!/bin/sh\nexec /opt/homebrew/bin/tmux -L rest-tui-p2 -f /dev/null "$@"\n' > "$T"; chmod +x "$T"
printf 'env HOME=%s/home %s/rest-tui -config %s/history.json -dir' "$FIX" "$REPO" "$FIX" > "$FIX/cmd"
cat > "$FIX/p2/http-client.env.json" <<'EOF'
{"$shared": {"host": "http://127.0.0.1:18091"},
 "dev": {"port": 18091, "debug": true, "SSLConfiguration": {"clientCertificate": "x.pem"}},
 "production": {"host": "http://127.0.0.1:18091"},
 "staging": {"port": 1}}
EOF
printf '### List\nGET {{host}}/?port={{port}}\n\n### Needs token\nGET {{host}}/secure\nAuthorization: Bearer {{token}}\n' > "$FIX/p2/api/users.http"
printf '{ broken' > "$FIX/p2/broken/http-client.env.json"
printf 'GET {{host}}/x\n' > "$FIX/p2/broken/a.http"
printf 'GET {{host}}/x\n' > "$FIX/none/api/x.http"
python3 -I -m http.server 18091 --bind 127.0.0.1 --directory "$FIX/p2" >/dev/null 2>&1 & echo $! > "$FIX/srv.pid"
sleep 1; curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18091/
```

Expected: 마지막 줄 `200`. `HOME`을 `$FIX/home`으로 바꿔 사용자의 `~/.config/rest-tui/dirs.json`을 건드리지 않고, `-config`로 사용자 히스토리도 건드리지 않는다. Browser의 파일 순서는 `api/users.http`, `broken/a.http`다.

- [ ] **Step 3: 120x35에서 env 로딩·전환·차단·기록 확인**

```bash
FIX="${TMPDIR:-/tmp}/rest-tui-p2"; T="$FIX/t"
"$T" new-session -d -s p2 -x 120 -y 35 "$(cat "$FIX/cmd") $FIX/p2"
sleep 1; "$T" send-keys -t p2 Enter Enter; sleep 0.5; "$T" capture-pane -p -t p2
```

`sleep 1`은 앱이 raw 모드로 바뀌기 전에 보낸 `Enter`가 `ctrl+j`로 읽히는 것을 막는다. 표의 각 행은 `"$T" send-keys -t p2 <키>; sleep 0.5; "$T" capture-pane -p -t p2`로 진행한다(색 확인은 `capture-pane -e -p`).

| 순서 | 키 | 화면에서 확인할 것 | 요구사항 |
|---|---|---|---|
| 1 | (위 명령) | `env: dev (1/3) · ./http-client.env.json — e cycle`, `GET http://127.0.0.1:18091/?port=18091` | P2-1, P2-2, P2-3, P2-4 |
| 2 | `Enter` | 응답 `200` | P2-1 |
| 3 | `e` | `env: production (2/3) …`, env 줄이 노란색(`capture-pane -e`로 색 시퀀스가 1번과 다른지 확인), `(sent with env: dev)`, 상태 줄 `cannot send: unresolved {{port}} — e switch env · i edit`, footer에 `send` 없음 | P2-5, P2-6, P2-8 |
| 4 | `Enter` | 같은 문구가 반전·굵게 강조됨 | P2-6 |
| 5 | `Escape` `Down` `Enter` | `Needs token`이 `env: production (2/3)`로 열림, 상태 줄 `cannot send: unresolved {{token}} — e switch env · i edit` | P2-5, P2-6 |
| 6 | `e`(→ staging) → `i` → `send-keys -t p2 -l 'X-Edited: yes'` → `C-s` | 헤더 `X-Edited: yes`가 보이고, 상태 줄은 여전히 `cannot send: unresolved {{token}} — e switch env · i edit`(편집에 남은 `{{token}}`) | P2-6 |
| 7 | `e` | `edits will be discarded — press e again`, 헤더 `X-Edited: yes` 유지, env는 `staging` 그대로 | P2-7 |
| 8 | `e` | `env: dev (1/3)`, `X-Edited` 줄 사라짐 | P2-7 |
| 9 | `h` | 목록 첫 항목에 `[dev]` | P2-8 |
| 10 | `Enter` | 상세 첫 줄 `env: dev` | P2-8 |
| 11 | `r` | env 줄 `resolved at send time (env: dev)`, footer에 `cycle env` 없음 | P2-9 |

- [ ] **Step 4: 80x24에서 같은 화면 확인**

```bash
FIX="${TMPDIR:-/tmp}/rest-tui-p2"; T="$FIX/t"
"$T" resize-window -t p2 -x 80 -y 24; sleep 0.5; "$T" capture-pane -p -t p2
"$T" send-keys -t p2 Escape Enter; sleep 0.5; "$T" capture-pane -p -t p2
```

Expected: 첫 캡처는 각 줄이 80열 안에 있고, env 줄 `resolved at send time (env: dev)`가 한 줄이며, 마지막 행이 `? help  q quit`로 끝난다. 두 번째 캡처(Browser로 돌아가 선택된 `Needs token`을 다시 엶)는 `env: dev (1/3) · ./http-client.env.json — e cycle`과 상태 줄 `cannot send: unresolved {{token}} — e switch env · i edit`이 각각 한 줄이다.

- [ ] **Step 5: 파싱 오류와 미발견 문구 확인**

```bash
FIX="${TMPDIR:-/tmp}/rest-tui-p2"; T="$FIX/t"
"$T" kill-session -t p2
"$T" new-session -d -s p2 -x 120 -y 35 "$(cat "$FIX/cmd") $FIX/p2/broken"
sleep 1; "$T" send-keys -t p2 Enter Enter; sleep 0.5; "$T" capture-pane -e -p -t p2
```
Expected: env 줄이 빨간색(`9`번 색 시퀀스) `http-client.env.json: invalid character …` (P2-4)

```bash
FIX="${TMPDIR:-/tmp}/rest-tui-p2"; T="$FIX/t"
"$T" kill-session -t p2
"$T" new-session -d -s p2 -x 120 -y 35 "$(cat "$FIX/cmd") $FIX/none"
sleep 1; "$T" send-keys -t p2 Enter Enter; sleep 0.5; "$T" capture-pane -p -t p2
```
Expected: `no http-client.env.json found (searched: api/, ./)` (P2-3, P2-4)

- [ ] **Step 6: 정리**

Run: `FIX="${TMPDIR:-/tmp}/rest-tui-p2"; "$FIX/t" kill-server; kill "$(cat "$FIX/srv.pid")"; rm -rf "$FIX"`
Expected: 오류 없이 끝남.

- [ ] **Step 7: 바이너리 커밋**

```bash
git status --short --untracked-files=no   # 추적 파일 변경은 rest-tui 하나여야 한다
git add rest-tui
git commit -m "build: rebuild committed dev binary"
```

---

## 요구사항 → 태스크 추적표

| 요구사항 | 구현 태스크 | 확인 테스트 / 절차 |
|---|---|---|
| P2-1 숫자·bool 문자열화, 객체·배열 건너뜀 | Task 1 | `TestLoadFiles_StringifiesScalarValues`, `TestLoadFiles_SkipsNullAndNonObjectEnvs`, `TestLoadFiles_EmptyFileYieldsNonNilMap`, Task 10 표 1–2 |
| P2-2 `$shared` 병합, 목록 제외 | Task 2 | `TestMerge_SharedLayersUnderEveryEnv`, `TestMerge_SharedAppliesWithNoEnvSelected`, `TestEnvNames_ExcludesShared`, `TestRequestModel_EnvLineSharedOnly` |
| P2-3 상위 폴더 탐색, 가장 가까운 폴더, public/private 짝 | Task 3, Task 4 | `TestLoadNearest_*`, `TestApp_OpenRequestSearchesEnvUpToAppRoot`, Task 10 Step 5 |
| P2-4 env 줄 문구 3종 | Task 4 | `TestDirLabel`, `TestRequestModel_EnvLine*`, `TestApp_EnvLineFitsTerminalWithDeepEnvPath`, Task 10 표 1, Step 4–5 |
| P2-5 세션 env 유지, 알파벳순 대체, prod 경고 색 | Task 5 | `TestNewRequestModel_PreferredEnvWinsWhenPresent`, `TestApp_SelectedEnvCarriesToNextRequest`, `TestRequestModel_ProdEnvUsesWarningStyle`, Task 10 표 3·5 |
| P2-6 전송 차단 안내, Enter 강조, send 힌트 숨김 | Task 6 | `TestRequestModel_UnresolvedVarsBlockSendWithGuidance`, `TestRequestModel_CanSendAfterSwitchingToCompleteEnv`, `TestRequestModel_ApplyEditKeepsLeftoverPlaceholdersBlocked`, `TestApp_FooterHidesSendWhenBlocked`, Task 10 표 3–6 |
| P2-7 편집 후 env 전환 재확인 | Task 7 | `TestRequestModel_CycleEnvAfterEditNeedsSecondPress`, `TestRequestModel_OtherKeyDisarmsEditDiscard`, Task 10 표 7–8 |
| P2-8 히스토리 env 기록, `(sent with env: …)`, 목록·상세 표시 | Task 8 | `TestStore_EnvRoundTripsAndIsOmittedWhenEmpty`, `TestRequestModel_SendRecordsEnvInHistory`, `TestRequestModel_RerunRecordsOriginalEnv`, `TestRequestModel_SentWithEnvShownAfterSwitch`, `TestHistoryView_ShowsEnvInListAndDetail`, Task 10 표 3·9–10 |
| P2-9 `resolved at send time`, `e cycle env` 숨김 | Task 9 (`canCycleEnv`는 Task 4) | `TestRequestModelFromEntry_EnvLineShowsSendTimeEnv`, `TestApp_FooterHidesCycleEnvWhenEnvCannotChange`, Task 10 표 11 |

| 단계 간 계약 (P2가 만듦) | 태스크 |
|---|---|
| `env.LoadNearest(dir, root string) (env.Loaded, error)`, `env.Loaded{Public, Private, Dir, Searched}` | Task 3 |
| `newRequestModel(filePath, root string, file *httpfile.File, req httpfile.Request, store *history.Store, preferredEnv string) requestModel` | Task 4 (시그니처), Task 5 (`preferredEnv` 사용) |
| `requestModel.currentEnv() string`, `App.lastEnv` | Task 5 |
| `requestModel.canSend() bool` | Task 6 |
| `history.Entry.Env string` (JSON 키 `Env`, `omitempty`) | Task 8 |
