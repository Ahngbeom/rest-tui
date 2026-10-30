# rest-tui

`rest-tui`는 IntelliJ HTTP Client `.http` 스크래치 파일을 터미널을 벗어나지 않고 탐색하고 실행할 수 있는 풀스크린 TUI(Terminal UI) 애플리케이션입니다. [Bubble Tea](https://github.com/charmbracelet/bubbletea)로 만들어졌습니다.

## Features

- 디렉터리를 재귀적으로 스캔해 `.http` 파일과 그 안의 요청 목록을 두 개의 패널로 탐색
- IntelliJ 스타일 `http-client.env.json` / `http-client.private.env.json` 환경 파일과 파일 스코프 `@name=value` 변수를 이용한 `{{var}}` 치환
- 요청 실행 결과(상태 코드/헤더/바디)를 JSON 들여쓰기 + ANSI 컬러로 표시
- 실행 히스토리 저장 및 조회, 과거 요청 재실행(rerun)
- `-dir`로 열었던 디렉터리 이력 저장 및 조회, 다른 디렉터리로 즉시 전환(재스캔)
- 키보드만으로 조작하는 화면 전환(파일 탐색 → 요청 → 히스토리 → 디렉터리 이력)

## Installation

### curl 스크립트 (권장)

Go 툴체인이나 Homebrew 없이 `$HOME/.local/bin`에 사전 빌드된 바이너리를 설치합니다. sudo가 필요하지 않습니다.

```sh
curl -fsSL https://raw.githubusercontent.com/Ahngbeom/rest-tui/main/scripts/install.sh | bash
```

- 지원 대상: linux/darwin × amd64/arm64 (Windows는 아래 "미리 빌드된 바이너리 다운로드" 또는 `go install` 사용)
- 특정 버전 설치: `REST_TUI_VERSION=v0.1.0 curl -fsSL ... | bash`
- 설치 후 `$HOME/.local/bin`이 PATH에 없다면 스크립트가 안내 문구를 출력합니다. 셸 설정 파일(`~/.bashrc`, `~/.zshrc` 등)에 다음을 추가하세요:
  ```sh
  export PATH="$HOME/.local/bin:$PATH"
  ```

### go install

Go 툴체인이 설치되어 있다면 가장 빠른 방법입니다.

```sh
go install github.com/Ahngbeom/rest-tui@latest
```

### Homebrew

```sh
brew install Ahngbeom/tap/rest-tui
```

### 미리 빌드된 바이너리 다운로드

Windows를 포함해 위 방법들을 사용할 수 없다면 [Releases](https://github.com/Ahngbeom/rest-tui/releases) 페이지에서 사용 중인 OS/아키텍처(linux/darwin/windows × amd64/arm64)에 맞는 바이너리를 내려받아 실행 권한만 부여하면 됩니다.

```sh
tar -xzf rest-tui_<version>_<os>_<arch>.tar.gz
chmod +x rest-tui
./rest-tui -dir <path-to-http-files>
```

### 소스에서 빌드

```sh
git clone https://github.com/Ahngbeom/rest-tui.git
cd rest-tui
scripts/build.sh            # go vet 후 빌드해 ./rest-tui 생성
scripts/build.sh --install  # 위와 동일하게 빌드한 뒤 $HOME/.local/bin 에도 설치
```

## Usage

```sh
rest-tui -dir <path-to-http-files>
```

| 플래그 | 기본값 | 설명 |
| --- | --- | --- |
| `-dir` | `.` | `.http` 파일을 탐색할 디렉터리 |
| `-config` | `~/.config/rest-tui/history.json` | 실행 히스토리를 저장할 파일 경로 |
| `-version` | - | 버전 정보를 출력하고 종료 |

## Keybindings

| 키 | 동작 |
| --- | --- |
| `q`, `Ctrl+C` | 종료 |
| `?` | 도움말 토글 |
| `Esc`, `Backspace` | 뒤로 가기 |
| `Enter` | 항목 열기 / 요청 전송 |
| `Ctrl+R` | 요청 전송 |
| `Tab` | 패널(파일 ↔ 요청) 전환 |
| `h` | 히스토리 화면으로 이동 |
| `d` | 디렉터리 이력 화면으로 이동 |
| `↑`/`k`, `↓`/`j` | 위/아래 이동 |
| `r` | 선택한 히스토리 항목 재실행 |
| `e` | 환경(env) 순환 전환 |

## Environment configuration

IntelliJ HTTP Client와 동일한 환경 파일을 두면 `{{var}}` 플레이스홀더가 자동으로 치환됩니다.

- `http-client.env.json` — 공개 환경 변수 (버전 관리 대상)
- `http-client.private.env.json` — 비공개 환경 변수 (일반적으로 `.gitignore` 대상)
- 파일 상단의 `@name=value` — 해당 `.http` 파일에만 적용되는 변수

규칙은 다음과 같습니다.

- **탐색**: `.http` 파일의 폴더부터 `-dir` 루트까지 위로 올라가며 두 파일 중 하나라도 있는 가장 가까운 폴더 하나를 사용합니다. `http-client.env.json`과 `http-client.private.env.json`은 그 폴더에서 짝으로 읽으며, 루트 위로는 올라가지 않습니다.
- **우선순위**: 파일 변수 > private[env] > public[env] > private[`$shared`] > public[`$shared`] 순입니다. `$shared`는 모든 환경에 기본값으로 깔리며 환경 목록에는 나오지 않습니다.
- **값**: 숫자와 bool은 문자열로 사용하고, 객체·배열·`null`(`SSLConfiguration` 등)은 건너뜁니다.

`{{변수}}`가 해석되지 않은 요청은 Enter로 보낼 수 없으며, 이유는 상태 줄에 표시됩니다. `e`로 고른 환경은 세션 동안 유지되어 이후 열리는 요청에도 적용됩니다(해당 환경이 없는 폴더에서는 첫 환경으로 열립니다). 별도의 "컬렉션/워크스페이스" 개념은 없으며, `-dir`로 지정한 디렉터리 자체가 요청의 원천(source of truth)입니다.

## IntelliJ HTTP Client compatibility

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

IntelliJ가 응답 저장용으로 남기는 `<> history.json` 참조 줄은 경고 없이 본문에서 뺍니다.

## History

요청을 전송하면 자동으로 히스토리에 기록됩니다. 기본 저장 위치는 `~/.config/rest-tui/history.json`이며 `-config`로 변경할 수 있습니다. 히스토리 화면(`h`)에서 과거 실행을 조회하고 `r`로 재실행할 수 있습니다.

## Directory history

`-dir`로 실행할 때마다 해당 디렉터리가 `~/.config/rest-tui/dirs.json`에 자동으로 기록됩니다(고정 경로, 별도 플래그 없음). 디렉터리 이력 화면(`d`)에서 과거에 열었던 디렉터리 목록을 최근 사용 순으로 조회하고, `enter`로 선택하면 앱을 재시작하지 않고도 해당 디렉터리를 즉시 재스캔해 전환할 수 있습니다.

## Development

빌드/테스트/포맷/vet 등 개발용 명령은 [CLAUDE.md](./CLAUDE.md)에 정리되어 있습니다. 태그(`v*`)를 푸시하면 `.github/workflows/release.yml`이 [goreleaser](https://goreleaser.com)로 크로스플랫폼 바이너리를 빌드해 GitHub Releases와 Homebrew tap(`Ahngbeom/homebrew-tap`)에 배포합니다.

## License

[MIT](./LICENSE)
