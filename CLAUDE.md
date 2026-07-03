# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

`rest-tui` (module `github.com/bahn/rest-tui`) is a full-screen terminal app, built with Bubble Tea, for browsing and running IntelliJ HTTP Client `.http` scratch files without leaving the terminal. Entry point: `main.go`, flags `-dir` (directory to scan for `.http` files, default `.`) and `-config` (history file path, default `~/.config/rest-tui/history.json`).

## Commands

- Build: `go build ./...` to compile-check; run `scripts/build.sh` to rebuild and atomically replace the checked-in `rest-tui` binary with the current source (runs `go vet ./...` first, then builds to a temp file and `mv`s it into place only on success). `go build -o rest-tui .` still works directly if you don't need the vet gate or the summary output.
- Run: `go run . -dir <path-to-http-files>`
- Test all: `go test ./...`
- Test a single test: `go test ./internal/httpfile/ -run TestParse_SingleRequestNoDelimiter` (package path + `-run <TestName>`, plain `testing` package, no test runner config)
- Format: `gofmt -l .` to check, `gofmt -w .` to fix
- Vet: `go vet ./...`

There is no CI config, Makefile, or linter config in this repo — `scripts/build.sh` plus the commands above are the full toolchain.

## Architecture

MVU (Elm-architecture) design via Bubble Tea, with HTTP parsing/execution kept independent of the UI layer.

- `internal/httpfile/` — parses `.http` scratch files into `httpfile.File{Vars, Requests}` / `httpfile.Request{Method, URL, Headers, Body}` (`parser.go`, `types.go`).
- `internal/env/` — resolves `{{var}}` substitution by loading IntelliJ-style `http-client.env.json` / `http-client.private.env.json` files, merged with file-scoped `@name=value` vars. Precedence: fileVars > private > public (`envfile.go`, `resolve.go`, `substitute.go`).
- `internal/executor/` — executes requests via plain `net/http` (`Execute(ctx, req, timeout)` → `executor.Response`), decoupled from the TUI so it's independently testable.
- `internal/output/` — formats response bodies (JSON indent + ANSI color via `tidwall/pretty`), UI-framework-agnostic.
- `internal/history/` — appends/lists executions as a single JSON array file (`store.go`). Self-heals from a corrupted history file by renaming it to `.corrupted-<timestamp>` and surfacing the warning via `Store.Warning()` / `App.statusMsg` rather than crashing.
- `internal/tui/` — the Bubble Tea app:
  - `app.go` — root model. Owns a `screen` enum (`screenBrowser`, `screenRequest`, `screenHistory`) and one sub-model per screen. `App.Update` intercepts global keys/messages first, then delegates the `tea.Msg` to whichever sub-model is active.
  - `browser.go` — two-pane file/request picker; `browserModel.focus` (`paneFiles`/`paneRequests`) tracks which `list.Model` pane has keyboard focus (Tab/Esc to switch), styled via `paneFocusedStyle` vs `paneStyle`.
  - `requestview.go` — variable resolution display, send, and response rendering. Async send is done via a `tea.Cmd` closure (`sendCmd`) that performs the HTTP call + history append off the update loop and returns `execResultMsg`.
  - `historyview.go` — list + detail view of past executions; supports rerun via `newRequestModelFromEntry`, which builds the model and returns a `tea.Cmd` to immediately resend.
  - `keys.go` — shared keybindings. `messages.go` — custom `tea.Msg` types (`openRequestMsg`, `rerunMsg`, `backToBrowserMsg`, `openHistoryMsg`) used for inter-screen navigation; `App.Update` type-switches on these to change `a.screen`. `styles.go` — lipgloss styles.

There is no separate "collections/workspaces" concept or app config beyond the two IntelliJ env JSON files sitting alongside the `.http` files — the filesystem (directory of `.http` files being browsed) is the source of truth for requests. No environment variables are read by the app itself.
