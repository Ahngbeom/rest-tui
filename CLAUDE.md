# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

`rest-tui` (module `github.com/Ahngbeom/rest-tui`) is a full-screen terminal app, built with Bubble Tea, for browsing and running IntelliJ HTTP Client `.http` scratch files without leaving the terminal. Entry point: `main.go`, flags `-dir` (directory to scan for `.http` files, default `.`), `-config` (history file path, default `~/.config/rest-tui/history.json`), and `-version` (print version and exit). Every `-dir` run is also recorded to a fixed `~/.config/rest-tui/dirs.json` (no flag override) so past directories can be browsed and switched to from the TUI. See `README.md` for install/usage docs aimed at end users.

## Commands

- Build: `go build ./...` to compile-check; run `scripts/build.sh` to rebuild and atomically replace the checked-in `rest-tui` binary with the current source (runs `go vet ./...` first, then builds to a temp file and `mv`s it into place only on success). `go build -o rest-tui .` still works directly if you don't need the vet gate or the summary output. The compiled `rest-tui` binary is committed to git (not in `.gitignore`), so a diff on it after rebuilding is expected, not a mistake. `scripts/build.sh --install` additionally copies the freshly built binary to `$HOME/.local/bin/rest-tui` (creating the directory if needed) so it's runnable from any directory; override the target with `REST_TUI_INSTALL_DIR`.
- `scripts/install.sh` is a standalone, repo-independent script (meant to be fetched via `curl -fsSL <raw-url> | bash`) that downloads a prebuilt release binary for the current OS/arch from GitHub Releases, verifies it against `checksums.txt`, and installs it to `$HOME/.local/bin`. It does not require a local clone of this repo.
- Run: `go run . -dir <path-to-http-files>`
- Test all: `go test ./...`
- Test a single test: `go test ./internal/httpfile/ -run TestParse_SingleRequestNoDelimiter` (package path + `-run <TestName>`, plain `testing` package, no test runner config)
- Format: `gofmt -l .` to check, `gofmt -w .` to fix
- Vet: `go vet ./...`

There is no Makefile or linter config in this repo — `scripts/build.sh` plus the commands above are the full local dev toolchain.

## Releases

Pushing a `v*` tag (e.g. `v0.1.0`) triggers `.github/workflows/release.yml`, which runs [goreleaser](https://goreleaser.com) (config: `.goreleaser.yaml`) to cross-compile `rest-tui` for linux/darwin/windows × amd64/arm64, publish archives + checksums to GitHub Releases, and push an updated formula to the `Ahngbeom/homebrew-tap` repo. The version string is injected into the `main.version` var via `-ldflags`, surfaced through the `-version` flag. This is the only CI in the repo — there's no test/lint workflow, only release-on-tag. `scripts/install.sh` downloads these same per-OS/arch archives and `checksums.txt` for its install flow.

## Architecture

MVU (Elm-architecture) design via Bubble Tea, with HTTP parsing/execution kept independent of the UI layer.

- `internal/httpfile/` — parses `.http` scratch files into `httpfile.File{Vars, Requests}` / `httpfile.Request{Method, URL, Headers, Body}` (`parser.go`, `types.go`).
- `internal/env/` — resolves `{{var}}` substitution by loading IntelliJ-style `http-client.env.json` / `http-client.private.env.json` files, merged with file-scoped `@name=value` vars. Precedence: fileVars > private > public (`envfile.go`, `resolve.go`, `substitute.go`).
- `internal/executor/` — executes requests via plain `net/http` (`Execute(ctx, req, timeout)` → `executor.Response`), decoupled from the TUI so it's independently testable.
- `internal/output/` — formats response bodies (JSON indent + ANSI color via `tidwall/pretty`), UI-framework-agnostic.
- `internal/history/` — appends/lists executions as a single JSON array file (`store.go`). Self-heals from a corrupted history file by renaming it to `.corrupted-<timestamp>` and surfacing the warning via `Store.Warning()` / `App.statusMsg` rather than crashing.
- `internal/dirhistory/` — mirrors `internal/history`'s JSON-array-file + self-heal pattern, but records directories passed via `-dir` (`Entry{Path, Timestamp}`) instead of request executions. `Store.Touch(path)` normalizes with `filepath.Clean`, dedupes by path, and refreshes the timestamp so `List` returns most-recently-used directories first.
- `internal/tui/` — the Bubble Tea app:
  - `app.go` — root model. Owns a `screen` enum (`screenBrowser`, `screenRequest`, `screenHistory`, `screenDirHistory`) and one sub-model per screen. `App.Update` intercepts global keys/messages first, then delegates the `tea.Msg` to whichever sub-model is active. Also renders the header/breadcrumb, footer hints, and help overlay (`breadcrumb()`, `footerHints()`, `helpView()`), and computes the vertical space left for the active sub-model via `contentHeight()`.
  - `browser.go` — two-pane file/request picker; `browserModel.focus` (`paneFiles`/`paneRequests`) tracks which `list.Model` pane has keyboard focus (Tab/Esc to switch), styled via `paneFocusedStyle` vs `paneStyle`. Re-scanning a new root creates a fresh `browserModel` (via `newBrowserModel`) rather than mutating in place.
  - `requestview.go` — variable resolution display, send, and response rendering. Async send is done via a `tea.Cmd` closure (`sendCmd`) that performs the HTTP call + history append off the update loop and returns `execResultMsg`.
  - `historyview.go` — list + detail view of past executions; supports rerun via `newRequestModelFromEntry`, which builds the model and returns a `tea.Cmd` to immediately resend.
  - `dirhistoryview.go` — list-only view (no detail mode — a path is self-explanatory) of directories recorded in `internal/dirhistory`; selecting one emits `switchDirMsg` to re-scan the Browser against that path.
  - `keys.go` — shared keybindings. `messages.go` — custom `tea.Msg` types (`openRequestMsg`, `rerunMsg`, `backToBrowserMsg`, `openHistoryMsg`, `switchDirMsg`) used for inter-screen navigation; `App.Update` type-switches on these to change `a.screen`. `keys.Directories` ("d") jumps to `screenDirHistory` the same way `keys.ToggleLog` ("h") jumps to `screenHistory` — handled directly in `App.Update`'s top-level key switch rather than via a dedicated open message. `styles.go` — lipgloss styles.

There is no separate "collections/workspaces" concept or app config beyond the two IntelliJ env JSON files sitting alongside the `.http` files — the filesystem (directory of `.http` files being browsed) is the source of truth for requests. No environment variables are read by the app itself.
