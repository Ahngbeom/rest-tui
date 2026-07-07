#!/usr/bin/env bash
set -euo pipefail

install=false
for arg in "$@"; do
  case "$arg" in
    --install) install=true ;;
    *)
      echo "scripts/build.sh: unknown argument: $arg" >&2
      exit 1
      ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"
[[ -f go.mod ]] || { echo "scripts/build.sh: go.mod not found at $REPO_ROOT" >&2; exit 1; }

rm -f rest-tui.new

echo "[build.sh] go vet ./..."
go vet ./...

echo "[build.sh] building..."
go build -trimpath -o rest-tui.new .
mv rest-tui.new rest-tui

commit="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
[[ -n "$(git status --porcelain 2>/dev/null)" ]] && commit="${commit} (dirty)"
size="$(wc -c < rest-tui | tr -d ' ')"
platform="$(go env GOOS)/$(go env GOARCH)"

echo "[build.sh] rest-tui updated"
echo "  source commit: ${commit}"
echo "  platform:      ${platform}"
echo "  binary:        ${REPO_ROOT}/rest-tui (${size} bytes)"
echo "  built at:      $(date '+%Y-%m-%dT%H:%M:%S%z')"

if $install; then
  install_dir="${REST_TUI_INSTALL_DIR:-$HOME/.local/bin}"
  mkdir -p "$install_dir"
  cp "$REPO_ROOT/rest-tui" "$install_dir/rest-tui"
  chmod +x "$install_dir/rest-tui"
  echo "[build.sh] installed: ${install_dir}/rest-tui"

  case ":${PATH}:" in
    *":${install_dir}:"*) ;;
    *)
      echo "[build.sh] WARNING: ${install_dir} is not on your PATH." >&2
      echo "[build.sh] Add this line to your shell rc file (~/.bashrc, ~/.zshrc, etc.) and restart your shell:" >&2
      printf '[build.sh]   export PATH="%s:$PATH"\n' "$install_dir" >&2
      ;;
  esac
fi
