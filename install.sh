#!/bin/sh
# hc-crew installer and updater.
#
#   curl -fsSL https://raw.githubusercontent.com/shadowbane/hc-crew/main/install.sh | sh
#   wget -qO-  https://raw.githubusercontent.com/shadowbane/hc-crew/main/install.sh | sh
#
# Run it again (or `hc-crew update`) to update. Environment:
#   HC_CREW_VERSION  a tag such as v0.1.0 (default: latest release)
#   HC_CREW_BIN_DIR  where to put the binary (default: ~/.local/bin)
set -eu

REPO="shadowbane/hc-crew"
BIN_DIR="${HC_CREW_BIN_DIR:-$HOME/.local/bin}"
VERSION="${HC_CREW_VERSION:-latest}"

say() { printf '%s\n' "$*"; }
die() { printf 'hc-crew install: %s\n' "$*" >&2; exit 1; }

if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -qO "$2" "$1"; }
else
  die "needs curl or wget"
fi

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  darwin|linux) ;;
  *) die "unsupported OS: $os" ;;
esac
arch=$(uname -m)
case "$arch" in
  arm64|aarch64) arch=arm64 ;;
  x86_64|amd64) arch=amd64 ;;
  *) die "unsupported CPU: $arch" ;;
esac

asset="hc-crew_${os}_${arch}.tar.gz"
if [ "$VERSION" = latest ]; then
  base="https://github.com/$REPO/releases/latest/download"
else
  base="https://github.com/$REPO/releases/download/$VERSION"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

say "downloading $asset ($VERSION)"
fetch "$base/$asset" "$tmp/$asset" || die "download failed: $base/$asset"
fetch "$base/checksums.txt" "$tmp/checksums.txt" || die "download failed: $base/checksums.txt"

want=$(grep " $asset\$" "$tmp/checksums.txt" | cut -d' ' -f1)
[ -n "$want" ] || die "no checksum for $asset"
if command -v shasum >/dev/null 2>&1; then
  got=$(shasum -a 256 "$tmp/$asset" | cut -d' ' -f1)
else
  got=$(sha256sum "$tmp/$asset" | cut -d' ' -f1)
fi
[ "$want" = "$got" ] || die "checksum mismatch for $asset"

tar -xzf "$tmp/$asset" -C "$tmp"
mkdir -p "$BIN_DIR"
install -m 0755 "$tmp/hc-crew" "$BIN_DIR/hc-crew"
# Downloaded with curl, so no quarantine flag is expected; clear it anyway.
xattr -d com.apple.quarantine "$BIN_DIR/hc-crew" 2>/dev/null || true

say "installed $("$BIN_DIR/hc-crew" version) to $BIN_DIR/hc-crew"

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) say "note: $BIN_DIR is not on your PATH. Add it, e.g.: echo 'export PATH=\"$BIN_DIR:\$PATH\"' >> ~/.zshrc" ;;
esac
command -v hc >/dev/null 2>&1 || say "note: hc (herdr-core) is not installed; hc-crew needs it."
command -v claude >/dev/null 2>&1 || say "note: claude (Claude Code) is not installed; hc-crew needs it."
