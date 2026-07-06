# rest-tui 리뷰 이슈 수정 + 공개 저장소 전환 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 코드 리뷰에서 발견된 이슈(파서 에러 격리, 히스토리 경고 유실, gofmt/trimpath)를 고치고, `Ahngbeom/rest-tui` 저장소를 안전하게 Public으로 전환한다.

**Architecture:** 변경은 기존 MVU 레이어 경계를 그대로 따른다 — `httpfile` 파서는 순수 데이터 변경(부분 실패를 `File.ParseErrors`로 노출), `internal/tui`는 그 정보를 소비해 화면에 반영, `internal/history`/`internal/tui/app.go`는 손상 경고 표시 타이밍만 수정. 공개 전환은 코드 변경 없이 빌드 스크립트 1곳 + 저장소 설정 변경으로 끝난다.

**Tech Stack:** Go 1.26, Bubble Tea/Bubbles/Lipgloss, 표준 `testing` 패키지, `gh` CLI.

## Global Constraints

- Go 버전은 `go.mod`의 `go 1.26.4`를 그대로 사용 (변경 금지).
- 기존 9개 `httpfile` 테스트, 4개 `browser` 테스트, 5개 `app` 테스트를 포함한 전체 스위트가 항상 통과해야 함 — 하위 호환성 우선, 꼭 필요한 경우가 아니면 기존 테스트 코드를 수정하지 않는다.
- `gofmt -l .`과 `go vet ./...`는 각 태스크 종료 시점에 항상 클린해야 함.
- 커밋된 `rest-tui` 바이너리는 계속 버전 관리한다(기존 프로젝트 관례, `CLAUDE.md`에 명시됨) — 이번 계획에서 바이너리 추적을 제거하지 않는다.
- 저장소 가시성 전환(Task 7)은 되돌리기 어려운 대외 노출 행위이므로, 실행 직전 사용자에게 반드시 재확인을 받은 뒤에만 진행한다.

---

## Part A — 코드 리뷰 이슈 수정

### Task 1: gofmt 위반 수정

**Files:**
- Modify: `internal/tui/browser.go` (import 순서)
- Modify: `internal/tui/requestview.go` (import 순서 + 주석 정렬)

**Interfaces:** 없음 — 포맷팅만 변경, 동작 변화 없음.

- [ ] **Step 1: 현재 위반 확인**

Run: `gofmt -l .`
Expected: `internal/tui/browser.go` 와 `internal/tui/requestview.go` 두 줄 출력.

- [ ] **Step 2: 자동 수정 적용**

Run: `gofmt -w internal/tui/browser.go internal/tui/requestview.go`

- [ ] **Step 3: 위반 사라졌는지 확인**

Run: `gofmt -l .`
Expected: 출력 없음(빈 문자열).

- [ ] **Step 4: 빌드/테스트로 회귀 없는지 확인**

Run: `go build ./... && go test ./...`
Expected: 모든 패키지 `ok`.

- [ ] **Step 5: 커밋**

```bash
git add internal/tui/browser.go internal/tui/requestview.go
git commit -m "gofmt: fix import ordering and comment alignment"
```

---

### Task 2: 로컬 개발 빌드에 `-trimpath` 적용 + 바이너리 재생성

**Files:**
- Modify: `scripts/build.sh:15`
- Modify (재생성): `rest-tui` (커밋된 바이너리)

**Interfaces:** 없음 — 빌드 플래그만 추가.

- [ ] **Step 1: 현재 바이너리에 로컬 경로가 남아있는지 확인**

Run: `strings rest-tui | grep -c "/Users/"`
Expected: `0`보다 큰 값(현재는 로컬 빌드 경로가 임베드되어 있음).

- [ ] **Step 2: 빌드 커맨드에 `-trimpath` 추가**

`scripts/build.sh:15`를 다음과 같이 변경:

```diff
-go build -o rest-tui.new .
+go build -trimpath -o rest-tui.new .
```

- [ ] **Step 3: 재빌드**

Run: `scripts/build.sh`
Expected: `go vet ./...` 통과 후 `[build.sh] rest-tui updated` 출력.

- [ ] **Step 4: 로컬 경로가 사라졌는지 확인**

Run: `strings rest-tui | grep -c "/Users/"`
Expected: `0`.

- [ ] **Step 5: 변경 사항 확인 후 커밋**

```bash
git status --short   # scripts/build.sh, rest-tui 만 변경되어야 함
git add scripts/build.sh rest-tui
git commit -m "build: strip local file paths from dev binary with -trimpath"
```

---

### Task 3: `httpfile.Parse`가 블록 단위로 에러를 격리하도록 수정

**Files:**
- Modify: `internal/httpfile/types.go`
- Modify: `internal/httpfile/parser.go`
- Test: `internal/httpfile/parser_test.go`

**Interfaces:**
- Produces: `File.ParseErrors []*ParseError` — 성공적으로 파싱되지 않은 블록들의 목록. `Parse`는 이제 블록 하나가 실패해도 나머지 유효한 블록은 `File.Requests`에 계속 채운다. `Parse`의 반환 시그니처(`(*File, error)`)는 그대로 유지하되, `error`는 "첫 번째 블록 에러"(`f.ParseErrors[0]`, 없으면 `nil`)를 담아 기존 9개 테스트(모두 `f, err := Parse(...)` 형태)와 100% 호환된다.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/httpfile/parser_test.go` 끝에 추가:

```go
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
```

- [ ] **Step 2: 실패하는지 확인**

Run: `go test ./internal/httpfile/ -run TestParse_OneBadBlockDoesNotHideOthers -v`
Expected: FAIL (`f.ParseErrors undefined` 컴파일 에러 — 필드가 아직 없음).

- [ ] **Step 3: `File`에 `ParseErrors` 필드 추가**

`internal/httpfile/types.go`의 `File` 구조체를 다음으로 교체:

```go
// File is the parsed contents of a .http file.
type File struct {
	// Vars holds file-scoped variables declared via bare `@name = value` lines.
	Vars     map[string]string
	Requests []Request
	// ParseErrors lists blocks that failed to parse. Requests still contains
	// every block that parsed successfully — one bad block does not hide
	// the rest of the file.
	ParseErrors []*ParseError
}
```

- [ ] **Step 4: `Parse`/`parseBlock`이 블록 실패 시 계속 진행하도록 수정**

`internal/httpfile/parser.go`에서 `Parse` 함수 전체를 다음으로 교체:

```go
// Parse reads an IntelliJ HTTP Client (.http) scratch file and returns its
// file-scoped variables and request blocks. Blocks are separated by lines
// starting with "###"; a file with no such line is treated as a single block.
// A block that fails to parse is skipped rather than aborting the whole
// file: its error is recorded in File.ParseErrors, and every other block
// still parses normally. The first recorded error (if any) is also
// returned as err for callers that only care whether something went wrong.
func Parse(data []byte) (*File, error) {
	lines := strings.Split(string(data), "\n")

	f := &File{Vars: map[string]string{}}

	// blockStart[i] is the first line index (0-based) of block i; blocks are
	// split on lines beginning with "###". Everything before the first "###"
	// (or the whole file, if there is none) is block 0.
	var blockStarts []int
	if len(lines) > 0 && !strings.HasPrefix(strings.TrimSpace(lines[0]), "###") {
		blockStarts = append(blockStarts, 0)
	}
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "###") {
			blockStarts = append(blockStarts, i)
		}
	}

	for bi, start := range blockStarts {
		end := len(lines)
		if bi+1 < len(blockStarts) {
			end = blockStarts[bi+1]
		}
		block := lines[start:end]
		delimiterName := ""
		bodyOffset := start
		if strings.HasPrefix(strings.TrimSpace(block[0]), "###") {
			delimiterName = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(block[0]), "###"))
			block = block[1:]
			bodyOffset++
		}

		if err := parseBlock(f, block, bodyOffset, delimiterName); err != nil {
			f.ParseErrors = append(f.ParseErrors, err)
		}
	}

	var firstErr error
	if len(f.ParseErrors) > 0 {
		firstErr = f.ParseErrors[0]
	}
	return f, firstErr
}
```

바로 아래 `parseBlock`의 시그니처와 반환문들을 `error` → `*ParseError`로 변경(다른 로직은 동일하게 유지):

```go
// parseBlock parses one ###-delimited section (with the "###" line itself
// already stripped) and, if it contains a request, appends it to f.Requests.
// lineOffset is the 0-based source line number of block[0]. On failure it
// returns the error without touching f.Requests for this block; the caller
// is expected to record it and move on to the next block.
func parseBlock(f *File, block []string, lineOffset int, delimiterName string) *ParseError {
	name := delimiterName
	var req *Request
	i := 0

	// Skip/consume leading comments, blank lines, and file-scoped @var
	// declarations until we find the method/URL line.
	for ; i < len(block); i++ {
		line := block[i]
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}
		if directive, ok := stripCommentPrefix(trimmed); ok {
			if n, ok := parseNameDirective(directive); ok {
				name = n
			}
			continue
		}
		if k, v, ok := parseVarDecl(trimmed); ok {
			f.Vars[k] = v
			continue
		}

		method, url, ok := parseRequestLine(trimmed)
		if !ok {
			return &ParseError{Line: lineOffset + i + 1, Msg: "expected HTTP method and URL, got " + quote(trimmed)}
		}
		req = &Request{Name: name, Method: method, URL: url, Line: lineOffset + i + 1}
		i++
		break
	}

	if req == nil {
		// Block had no method/URL line at all (e.g. only comments/blank
		// lines, or empty trailing block) -- nothing to add.
		return nil
	}

	// Headers: consume "Name: value" lines until a blank line or EOF.
	for ; i < len(block); i++ {
		trimmed := strings.TrimSpace(block[i])
		if trimmed == "" {
			i++
			break
		}
		name, value, ok := parseHeaderLine(trimmed)
		if !ok {
			return &ParseError{Line: lineOffset + i + 1, Msg: "expected header \"Name: value\", got " + quote(trimmed)}
		}
		req.Headers = append(req.Headers, Header{Name: name, Value: value})
	}

	// Body: remainder of the block, trimmed of surrounding blank lines.
	if i < len(block) {
		req.Body = strings.TrimRight(strings.Join(block[i:], "\n"), "\n \t")
	}

	f.Requests = append(f.Requests, *req)
	return nil
}
```

- [ ] **Step 5: 새 테스트 통과 확인**

Run: `go test ./internal/httpfile/ -run TestParse_OneBadBlockDoesNotHideOthers -v`
Expected: PASS.

- [ ] **Step 6: 기존 9개 테스트 전부 회귀 없는지 확인**

Run: `go test ./internal/httpfile/ -v`
Expected: 10개 테스트(기존 9개 + 신규 1개) 모두 PASS. 특히 `TestParse_MissingMethodLineIsError`와 `TestParse_UnknownMethodIsError`가 여전히 통과하는지 확인 — `err.(*ParseError)` 타입 단언이 그대로 유효해야 함.

- [ ] **Step 7: 커밋**

```bash
git add internal/httpfile/types.go internal/httpfile/parser.go internal/httpfile/parser_test.go
git commit -m "httpfile: isolate parse errors per block instead of failing the whole file"
```

---

### Task 4: Browser 화면이 부분 파싱 실패에도 유효한 요청을 계속 보여주도록 수정

**Files:**
- Modify: `internal/tui/browser.go`
- Test: `internal/tui/browser_test.go`

**Interfaces:**
- Consumes: `httpfile.File.ParseErrors []*httpfile.ParseError` (Task 3에서 추가됨).
- 기존 필드 `browserModel.parseErr error`의 의미를 그대로 유지한다: "이 파일에서 보여줄 수 있는 요청이 하나도 없는 치명적 상황"(파일 읽기 실패, 또는 모든 블록이 파싱 실패)일 때만 설정된다. 일부만 실패한 경우는 `parseErr == nil`이고 `parsedFile.ParseErrors`에만 기록되므로, 기존 3개 테스트(`TestBrowserModel_TabTogglesFocusOnlyAfterFileParsed`, `TestBrowserModel_SelectFile_ParseErrorKeepsFocusOnFiles`, `TestBrowserModel_EnterOnRequestEmitsOpenRequestMsg`)는 변경 없이 그대로 통과한다.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/tui/browser_test.go` 끝에 추가:

```go
func TestBrowserModel_SelectFile_PartialParseErrorsStillShowValidRequests(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "mixed.http", "### Get user\nGET https://example.com/users/1\n\n### Broken\nContent-Type: application/json\n\n### Create user\nPOST https://example.com/users\n")

	m := newBrowserModel(dir)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.focus != paneRequests {
		t.Fatalf("focus = %v, want paneRequests despite one broken block", m.focus)
	}
	if m.parseErr != nil {
		t.Fatalf("parseErr = %v, want nil when some requests parsed successfully", m.parseErr)
	}
	if m.parsedFile == nil || len(m.parsedFile.ParseErrors) != 1 {
		t.Fatalf("parsedFile.ParseErrors = %v, want 1 entry for the broken block", m.parsedFile.ParseErrors)
	}
	if len(m.requests.Items()) != 2 {
		t.Fatalf("requests.Items() = %d, want 2 valid requests", len(m.requests.Items()))
	}
}
```

- [ ] **Step 2: 실패하는지 확인**

Run: `go test ./internal/tui/ -run TestBrowserModel_SelectFile_PartialParseErrorsStillShowValidRequests -v`
Expected: FAIL — 현재 코드는 `httpfile.Parse`가 `err != nil`을 반환하면(부분 실패든 전체 실패든) `m.parseErr`를 그 에러로 채우고 요청 목록을 통째로 비우므로, `m.parseErr != nil` 단언에서 실패.

- [ ] **Step 3: `selectFile`이 부분 실패를 구분하도록 수정**

`internal/tui/browser.go`의 `selectFile` 메서드 전체를 다음으로 교체:

```go
// selectFile parses the currently highlighted file in the files pane and, on
// success, populates the requests pane and moves focus to it. A file that
// fails to parse entirely (I/O error, or every block malformed) leaves focus
// on the files pane; a file where only some blocks fail still populates the
// requests pane with whatever parsed successfully.
func (m browserModel) selectFile() browserModel {
	item, ok := m.files.SelectedItem().(fileItem)
	if !ok {
		return m
	}
	full := filepath.Join(m.root, item.rel)
	m.selectedFile = full

	data, err := os.ReadFile(full)
	if err != nil {
		m.parseErr = err
		m.parsedFile = nil
		m.requests.SetItems(nil)
		return m
	}

	f, parseErr := httpfile.Parse(data)
	m.parsedFile = f
	items := make([]list.Item, len(f.Requests))
	for i, r := range f.Requests {
		items[i] = requestItem{req: r}
	}
	m.requests.SetItems(items)

	if len(f.Requests) == 0 {
		m.parseErr = parseErr
		return m
	}
	m.parseErr = nil
	m.focus = paneRequests
	return m
}
```

- [ ] **Step 4: `View`가 부분 실패 시 경고를 요청 목록과 함께 보여주도록 수정**

`internal/tui/browser.go`의 `View` 메서드 전체를 다음으로 교체하고, 바로 아래에 헬퍼 함수를 추가:

```go
func (m browserModel) View() string {
	filesPane := paneStyle
	requestsPane := paneStyle
	if m.focus == paneFiles {
		filesPane = paneFocusedStyle
	} else {
		requestsPane = paneFocusedStyle
	}

	right := m.requests.View()
	switch {
	case m.parseErr != nil:
		right = errorTextStyle.Render("parse error:\n" + m.parseErr.Error())
	case m.parsedFile != nil && len(m.parsedFile.ParseErrors) > 0:
		right = errorTextStyle.Render("parse warning: "+joinParseErrors(m.parsedFile.ParseErrors)) + "\n\n" + right
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		filesPane.Render(m.files.View()),
		requestsPane.Render(right),
	)
}

// joinParseErrors formats the blocks that failed to parse as a single
// semicolon-separated line for the warning banner above the requests pane.
func joinParseErrors(errs []*httpfile.ParseError) string {
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}
```

- [ ] **Step 5: 새 테스트 통과 확인**

Run: `go test ./internal/tui/ -run TestBrowserModel_SelectFile_PartialParseErrorsStillShowValidRequests -v`
Expected: PASS.

- [ ] **Step 6: tui 패키지 전체 회귀 없는지 확인**

Run: `go test ./internal/tui/... -v`
Expected: 모든 테스트 PASS, 특히 `TestBrowserModel_SelectFile_ParseErrorKeepsFocusOnFiles`(전체 실패 케이스)가 그대로 통과하는지 확인.

- [ ] **Step 7: 전체 빌드/테스트/vet 확인 후 커밋**

```bash
go build ./... && go vet ./... && go test ./...
git add internal/tui/browser.go internal/tui/browser_test.go
git commit -m "tui: keep showing valid requests when only some blocks fail to parse"
```

---

### Task 5: 히스토리 손상 경고를 세션 내내 재노출

**Files:**
- Modify: `internal/tui/app.go`
- Test: `internal/tui/app_test.go`

**Interfaces:**
- Consumes: `history.Store.Warning() string` (기존 API, 변경 없음).
- 현재 `NewApp`이 생성 직후 `store.Warning()`을 확인하는 코드는 사실상 항상 빈 문자열만 본다 — 이 시점까지 `Store.load()`가 한 번도 호출되지 않았기 때문(손상 감지는 `load()` 안에서만 일어남). 이번 수정은 그 죽은 코드를 제거하고, 대신 `App.Update`가 메시지를 처리할 때마다(특히 History 화면 진입·요청 전송처럼 실제로 `load()`가 실행되는 시점 직후) `store.Warning()`을 다시 확인하도록 만든다.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/tui/app_test.go` 상단 import에 `"os"` 추가:

```go
import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Ahngbeom/rest-tui/internal/history"
	"github.com/Ahngbeom/rest-tui/internal/httpfile"
)
```

파일 끝에 테스트 추가:

```go
func TestApp_SurfacesHistoryCorruptionWarningAfterDetection(t *testing.T) {
	dir := t.TempDir()
	histPath := filepath.Join(t.TempDir(), "history.json")
	if err := os.WriteFile(histPath, []byte("{ not valid json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	store := history.NewStore(histPath)
	a := NewApp(dir, store)
	model, _ := a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a = model.(App)

	if a.statusMsg != "" {
		t.Fatalf("statusMsg = %q, want empty before anything has read the history file", a.statusMsg)
	}

	// "h" jumps to the History screen, which calls store.List -> load(),
	// discovering the corruption for the first time.
	model, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	a = model.(App)

	if a.statusMsg == "" {
		t.Fatal("expected statusMsg to surface the corruption warning once History has loaded")
	}
}
```

- [ ] **Step 2: 실패하는지 확인**

Run: `go test ./internal/tui/ -run TestApp_SurfacesHistoryCorruptionWarningAfterDetection -v`
Expected: FAIL — 현재 `NewApp`은 `load()`가 실행되기도 전에 `store.Warning()`을 한 번 확인하고 끝이라 `a.statusMsg`가 계속 빈 문자열.

- [ ] **Step 3: `App`에 경고 재확인 로직 추가**

`internal/tui/app.go`의 `NewApp` 함수에서 죽은 체크를 제거:

```go
// NewApp builds the root model. root is the directory .http files are
// discovered under; store is where executed requests are recorded.
func NewApp(root string, store *history.Store) App {
	a := App{root: root, store: store}
	a.browser = newBrowserModel(root)
	a.history = newHistoryModel(store)
	return a
}
```

같은 파일에 헬퍼 메서드 추가(`contentHeight` 아래, `Update` 위 아무 곳):

```go
// applyStoreWarning copies any newly-detected history store warning (e.g. a
// corrupted file being backed up) into the status line. Corruption is only
// discovered lazily, the first time something actually reads the history
// file, so this is checked here rather than once at startup.
func (a App) applyStoreWarning() App {
	if w := a.store.Warning(); w != "" {
		a.statusMsg = w
	}
	return a
}
```

`Update` 메서드에서 History로 전환하는 두 지점과 최종 반환부를 수정 — `case key.Matches(msg, keys.ToggleLog):` 블록을:

```go
		case key.Matches(msg, keys.ToggleLog):
			a.screen = screenHistory
			a.history = a.history.refresh()
			return a.applyStoreWarning(), nil
```

`case openHistoryMsg:` 블록을:

```go
	case openHistoryMsg:
		a.screen = screenHistory
		a.history = a.history.refresh()
		return a.applyStoreWarning(), nil
```

`Update` 함수 맨 마지막 반환문을:

```go
	var cmd tea.Cmd
	switch a.screen {
	case screenBrowser:
		a.browser, cmd = a.browser.Update(msg)
	case screenRequest:
		a.request, cmd = a.request.Update(msg)
	case screenHistory:
		a.history, cmd = a.history.Update(msg)
	}
	return a.applyStoreWarning(), cmd
```

- [ ] **Step 4: 새 테스트 통과 확인**

Run: `go test ./internal/tui/ -run TestApp_SurfacesHistoryCorruptionWarningAfterDetection -v`
Expected: PASS.

- [ ] **Step 5: tui 패키지 전체 회귀 없는지 확인**

Run: `go test ./internal/tui/... -v`
Expected: 모든 테스트(App/browser/history/request 관련) PASS.

- [ ] **Step 6: 전체 스위트 + vet 확인 후 커밋**

```bash
go build ./... && go vet ./... && go test ./...
git add internal/tui/app.go internal/tui/app_test.go
git commit -m "tui: re-check history store warning whenever it might have just been detected"
```

---

## Part B — 공개 저장소 전환

### Task 6: 전환 전 최종 재점검

Part A 커밋들이 반영된 최신 상태 기준으로, 이전 리뷰에서 했던 점검을 다시 실행해 회귀가 없는지 확인한다. 코드 변경 없음 — 전부 읽기 전용 검증.

**Files:** 없음 (검증만 수행)

- [ ] **Step 1: 포맷/정적분석/테스트**

Run: `gofmt -l . && go vet ./... && go test ./... -cover`
Expected: `gofmt -l .` 출력 없음, `go vet` 무출력, 모든 패키지 테스트 PASS (핵심 로직 패키지는 90%+ 커버리지 유지).

- [ ] **Step 2: 바이너리에 로컬 경로가 없는지 재확인**

Run: `strings rest-tui | grep -c "/Users/"`
Expected: `0` (Task 2에서 이미 적용됨).

- [ ] **Step 3: 커밋 이력에 시크릿/자격증명이 없는지 재확인**

Run: `git grep -niE 'api[_-]?key|secret|password|token|BEGIN (RSA|OPENSSH|PRIVATE)' -- . ':(exclude)*.md'`
Expected: 테스트 픽스처의 가짜 값(`internal/env/*_test.go`, `internal/httpfile/parser_test.go`)과 `.github/workflows/release.yml`/`.goreleaser.yaml`의 `${{ secrets.* }}` 참조만 나와야 함 — 실제 값이 하드코딩된 줄이 있으면 여기서 멈추고 먼저 처리한다.

- [ ] **Step 4: 작업 트리 정리 상태 확인**

Run: `git status --short`
Expected: 출력 없음(Part A 커밋이 전부 반영된 클린 상태).

---

### Task 7: GitHub 저장소 가시성을 Public으로 전환

**중요:** 이 태스크의 Step 2는 되돌리기 어려운 대외 공개 행위다. Task 6 점검이 모두 통과했다는 사실을 사용자에게 보고하고, **명시적인 최종 승인을 받은 뒤에만** Step 2를 실행한다. 이 계획이 subagent-driven-development나 executing-plans로 실행되더라도, Step 2는 자동으로 건너뛰지 말고 반드시 사용자 확인 지점에서 멈춘다.

**Files:** 없음 (저장소 설정 변경, 코드 변경 없음)

- [ ] **Step 1: 전환 전 현재 상태 기록**

Run: `gh repo view Ahngbeom/rest-tui --json visibility,isPrivate,url`
Expected: `"visibility":"PRIVATE","isPrivate":true` (현재 상태 확인용 스냅샷).

- [ ] **Step 2: (사용자 최종 승인 후) Public으로 전환**

Run: `gh repo edit Ahngbeom/rest-tui --visibility public --accept-visibility-change-consequences`
Expected: 명령이 에러 없이 종료.

- [ ] **Step 3: 전환 결과 확인**

Run: `gh repo view Ahngbeom/rest-tui --json visibility,isPrivate,url`
Expected: `"visibility":"PUBLIC","isPrivate":false`.

- [ ] **Step 4: 릴리스 파이프라인이 여전히 정상 참조되는지 확인**

Run: `gh secret list --repo Ahngbeom/rest-tui`
Expected: `HOMEBREW_TAP_TOKEN`이 목록에 존재(`GITHUB_TOKEN`은 GitHub가 자동 주입하므로 목록에 없는 것이 정상). Public 전환 자체는 Actions 시크릿이나 `.goreleaser.yaml`/`release.yml` 동작에 영향을 주지 않는다 — 이 워크플로우는 태그 push에만 반응하고 `pull_request`에는 반응하지 않으므로, Public 전환 후 흔한 위험인 "포크 PR이 시크릿에 접근" 문제도 해당 없음.

- [ ] **Step 5: 릴리스 페이지/README 노출 확인**

Run: `gh repo view Ahngbeom/rest-tui --web` 대신(브라우저 여는 대신) `curl -sI https://github.com/Ahngbeom/rest-tui | head -1`
Expected: `HTTP/2 200` — 로그인 없이도 저장소 홈이 열림(Public 전환이 실제로 적용됐다는 최종 확인).

---

## 실행 시 참고

- Task 1~5(Part A)는 순서대로 진행해도 되고, Task 3→4는 서로 의존하므로 그 순서는 지켜야 한다. Task 1, 2, 5는 독립적이라 순서를 바꿔도 무방하다.
- Task 6, 7(Part B)은 Part A가 전부 커밋된 뒤에 시작한다 — Task 6의 재점검이 Part A 결과물을 전제로 하기 때문이다.
- Task 7 Step 2는 이 계획을 실행하는 에이전트가 임의로 자동 실행해서는 안 된다.
