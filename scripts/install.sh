#!/usr/bin/env bash
set -euo pipefail

# Standalone installer, meant to be fetched and run directly:
#   curl -fsSL https://raw.githubusercontent.com/Ahngbeom/rest-tui/main/scripts/install.sh | bash
# Does not assume a local clone of the repo exists.

REPO="Ahngbeom/rest-tui"
INSTALL_DIR="${REST_TUI_INSTALL_DIR:-$HOME/.local/bin}"
BIN_NAME="rest-tui"

err() {
  printf '[install.sh] ERROR: %s\n' "$1" >&2
  exit 1
}

info() {
  printf '[install.sh] %s\n' "$1"
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || err "required command '$1' not found"
}

need_cmd curl
need_cmd tar

# --- OS detection ---
os_raw="$(uname -s)"
os="$(printf '%s' "$os_raw" | tr '[:upper:]' '[:lower:]')"
case "$os" in
  linux | darwin) ;;
  *)
    err "unsupported OS: $os_raw (Windows is not supported by this script — use 'go install github.com/${REPO}@latest' or download a .zip from https://github.com/${REPO}/releases)"
    ;;
esac

# --- Architecture detection ---
arch_raw="$(uname -m)"
case "$arch_raw" in
  x86_64 | amd64) arch="amd64" ;;
  aarch64 | arm64) arch="arm64" ;;
  *) err "unsupported architecture: $arch_raw" ;;
esac

# --- Version resolution ---
raw_version="${REST_TUI_VERSION:-}"
if [[ -z "$raw_version" ]]; then
  info "resolving latest release..."
  api_response="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest")" ||
    err "failed to fetch latest release metadata from GitHub API — check network connectivity or set REST_TUI_VERSION explicitly"
  tag_line="$(printf '%s' "$api_response" | grep -m1 '"tag_name"' || true)"
  raw_version="$(printf '%s' "$tag_line" | sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]*)".*/\1/')"
  [[ -n "$raw_version" ]] || err "could not determine latest version from GitHub API response"
fi

# Accept both "v0.1.0" and "0.1.0" forms; normalize to both a bare version
# (used in the archive filename) and a v-prefixed tag (used in the URL path).
version_num="${raw_version#v}"
tag="v${version_num}"

# --- Build download URLs (only linux/darwin reach here, so archive is always tar.gz) ---
asset="rest-tui_${version_num}_${os}_${arch}.tar.gz"
base_url="https://github.com/${REPO}/releases/download/${tag}"
asset_url="${base_url}/${asset}"
checksums_url="${base_url}/checksums.txt"

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

info "downloading ${asset}..."
curl -fsSL -o "${work_dir}/${asset}" "$asset_url" ||
  err "failed to download ${asset_url} (does version ${tag} exist for ${os}/${arch}?)"
curl -fsSL -o "${work_dir}/checksums.txt" "$checksums_url" ||
  err "failed to download checksums.txt from ${checksums_url}"

# --- Checksum verification ---
cd "$work_dir"
if ! grep " ${asset}\$" checksums.txt >checksum-selected.txt; then
  err "no checksum entry found for ${asset} in checksums.txt"
fi

if command -v sha256sum >/dev/null 2>&1; then
  sha256sum -c checksum-selected.txt >/dev/null ||
    err "checksum verification failed for ${asset} — download may be corrupted or tampered"
elif command -v shasum >/dev/null 2>&1; then
  shasum -a 256 -c checksum-selected.txt >/dev/null ||
    err "checksum verification failed for ${asset} — download may be corrupted or tampered"
else
  err "neither sha256sum nor shasum found — cannot verify checksum"
fi

# --- Extract & install ---
tar -xzf "$asset" -C "$work_dir" || err "failed to extract ${asset}"
[[ -f "${work_dir}/${BIN_NAME}" ]] ||
  err "extracted archive did not contain a '${BIN_NAME}' binary — archive layout may have changed"

mkdir -p "$INSTALL_DIR"
chmod +x "${work_dir}/${BIN_NAME}"
mv -f "${work_dir}/${BIN_NAME}" "${INSTALL_DIR}/${BIN_NAME}"

info "rest-tui ${version_num} installed to ${INSTALL_DIR}/${BIN_NAME}"

# --- PATH check ---
case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *)
    printf '[install.sh] WARNING: %s is not on your PATH.\n' "$INSTALL_DIR" >&2
    printf '[install.sh] Add this line to your shell rc file (~/.bashrc, ~/.zshrc, etc.) and restart your shell:\n' >&2
    printf '[install.sh]   export PATH="%s:$PATH"\n' "$INSTALL_DIR" >&2
    ;;
esac
