#!/bin/sh
# local-swarm-mcp -- Linux and macOS installer
#
# Usage (one-liner):
#   curl -fsSL https://raw.githubusercontent.com/jhonsferg/local-swarm-mcp/main/install/install.sh | sh
#
# Customise with environment variables before piping:
#   LSM_INSTALL_DIR=$HOME/.local/bin LSM_VERSION=v0.3.0 curl -fsSL ... | sh

set -eu

REPO="${LSM_REPO:-jhonsferg/local-swarm-mcp}"
INSTALL_DIR="${LSM_INSTALL_DIR:-$HOME/.local/bin}"
LSM_VERSION="${LSM_VERSION:-latest}"

# Override base URLs for local/offline testing:
#   LSM_TEST_API_BASE=http://localhost:8765
#   LSM_TEST_DL_BASE=http://localhost:8765
API_BASE="${LSM_TEST_API_BASE:-https://api.github.com}"
DL_BASE="${LSM_TEST_DL_BASE:-https://github.com}"

# -- Terminal helpers ----------------------------------------------------------
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
    C_CYAN='\033[0;36m'
    C_GREEN='\033[0;32m'
    C_RED='\033[0;31m'
    C_BOLD='\033[1m'
    C_RESET='\033[0m'
else
    C_CYAN='' C_GREEN='' C_RED='' C_BOLD='' C_RESET=''
fi

step() { printf "  ${C_CYAN}->${C_RESET} %s\n" "$1"; }
ok()   { printf "  ${C_GREEN}v ${C_RESET} %s\n" "$1"; }
die()  { printf "  ${C_RED}x ${C_RESET} %s\n" "$1" >&2; exit 1; }

printf "\n  ${C_BOLD}${C_CYAN}local-swarm-mcp${C_RESET}${C_BOLD} installer${C_RESET}\n\n"

# -- 1. Check required tools ---------------------------------------------------
need() {
    command -v "$1" > /dev/null 2>&1 || die "'$1' is required but not installed."
}
need curl
need tar

# -- 2. Detect OS and architecture ----------------------------------------------
OS="$(uname -s 2>/dev/null || echo unknown)"
case "$OS" in
    Linux)  PLATFORM="linux"  ;;
    Darwin) PLATFORM="darwin" ;;
    *)      die "Unsupported OS: $OS  (only Linux and macOS are supported)" ;;
esac

MACHINE="$(uname -m 2>/dev/null || echo unknown)"
case "$MACHINE" in
    x86_64 | amd64)           ARCH="amd64" ;;
    aarch64 | arm64 | armv8*) ARCH="arm64" ;;
    *)                        die "Unsupported architecture: $MACHINE (local-swarm-mcp ships amd64 and arm64 only)" ;;
esac

step "Detected platform: $PLATFORM-$ARCH"

# -- 3. Resolve version ---------------------------------------------------------
if [ "$LSM_VERSION" = "latest" ]; then
    step "Fetching latest release from $API_BASE..."
    LSM_VERSION="$(
        curl -fsSL "$API_BASE/repos/$REPO/releases/latest" \
        | grep '"tag_name"' \
        | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/'
    )"
    [ -n "$LSM_VERSION" ] || die "Failed to resolve the latest version. Check your network."
fi

step "Installing local-swarm-mcp $LSM_VERSION"

# -- 4. Download archive and checksums -------------------------------------------
ARCHIVE="local-swarm-mcp_${PLATFORM}_${ARCH}.tar.gz"
BASE_URL="$DL_BASE/$REPO/releases/download/$LSM_VERSION"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/local-swarm-mcp-install.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

step "Downloading $ARCHIVE..."
if ! curl -fSL --progress-bar "$BASE_URL/$ARCHIVE" -o "$TMP_DIR/$ARCHIVE"; then
    die "Download failed.\n  URL: $BASE_URL/$ARCHIVE\n  Check that release $LSM_VERSION exists."
fi

step "Downloading checksums.txt..."
if ! curl -fsSL "$BASE_URL/checksums.txt" -o "$TMP_DIR/checksums.txt"; then
    die "Failed to download checksums.txt for verification."
fi

# -- 5. Verify checksum -----------------------------------------------------------
step "Verifying checksum..."
EXPECTED="$(grep " $ARCHIVE\$" "$TMP_DIR/checksums.txt" | awk '{print $1}')"
[ -n "$EXPECTED" ] || die "No checksum entry found for $ARCHIVE in checksums.txt."

if command -v sha256sum > /dev/null 2>&1; then
    ACTUAL="$(sha256sum "$TMP_DIR/$ARCHIVE" | awk '{print $1}')"
elif command -v shasum > /dev/null 2>&1; then
    ACTUAL="$(shasum -a 256 "$TMP_DIR/$ARCHIVE" | awk '{print $1}')"
else
    die "Neither sha256sum nor shasum is available to verify the download."
fi

[ "$EXPECTED" = "$ACTUAL" ] || die "Checksum mismatch for $ARCHIVE.\n  Expected: $EXPECTED\n  Actual:   $ACTUAL"
ok "Checksum verified"

# -- 6. Extract and install binary -------------------------------------------------
mkdir -p "$INSTALL_DIR"
step "Extracting..."
if ! tar xzf "$TMP_DIR/$ARCHIVE" -C "$TMP_DIR" local-swarm-mcp; then
    die "Extraction failed. The archive may be corrupted."
fi
mv "$TMP_DIR/local-swarm-mcp" "$INSTALL_DIR/local-swarm-mcp"
chmod +x "$INSTALL_DIR/local-swarm-mcp"
ok "Installed to $INSTALL_DIR/local-swarm-mcp"

# -- 7. Summary ---------------------------------------------------------------------
INSTALLED_VERSION="$("$INSTALL_DIR/local-swarm-mcp" -version 2>&1 || true)"
ok "Binary check: $INSTALLED_VERSION"

printf "\n  ${C_GREEN}${C_BOLD}local-swarm-mcp $LSM_VERSION installed!${C_RESET}\n\n"
printf "  Next steps:\n\n"
printf "  1. Make sure %s is on your PATH.\n" "$INSTALL_DIR"
printf "  2. Start the daemon and add backends via the dashboard or -register-host -\n"
printf "     no config file needed. See the README's \"Configuring backends\" section:\n"
printf "       ${C_CYAN}https://github.com/%s${C_RESET}\n" "$REPO"
printf "  3. Register local-swarm-mcp with your MCP client - see:\n"
printf "       ${C_CYAN}https://github.com/%s#registering-with-an-mcp-client${C_RESET}\n\n" "$REPO"
