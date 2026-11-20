#!/bin/sh
# Installs the lazyfwd binary from a GitHub release.
#   curl -fsSL https://github.com/glattziehen/lazyfwd/releases/latest/download/install.sh | sh
# LAZYFWD_VERSION picks a release tag (default: latest); LAZYFWD_INSTALL_DIR the target (default:
# ~/.local/bin); LAZYFWD_DOWNLOAD_URL a mirror of the release assets.
set -eu

repo=glattziehen/lazyfwd
version=${LAZYFWD_VERSION:-latest}
dir=${LAZYFWD_INSTALL_DIR:-$HOME/.local/bin}

fail() {
  echo "lazyfwd install: $*" >&2
  exit 1
}

case $(uname -s) in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  MINGW* | MSYS* | CYGWIN*)
    fail "on Windows, run in PowerShell: irm https://github.com/$repo/releases/latest/download/install.ps1 | iex"
    ;;
  *) fail "no binary for $(uname -s); install with go install instead" ;;
esac

case $(uname -m) in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) fail "no binary for $(uname -m); install with go install instead" ;;
esac

# A shell under Rosetta reports x86_64 on Apple silicon.
if [ "$os" = darwin ] && [ "$(sysctl -n sysctl.proc_translated 2> /dev/null || true)" = 1 ]; then
  arch=arm64
fi

if [ -n "${LAZYFWD_DOWNLOAD_URL:-}" ]; then
  base=$LAZYFWD_DOWNLOAD_URL
elif [ "$version" = latest ]; then
  base=https://github.com/$repo/releases/latest/download
else
  base=https://github.com/$repo/releases/download/$version
fi

if command -v curl > /dev/null; then
  fetch() { curl -fsSL -o "$2" "$1"; }
elif command -v wget > /dev/null; then
  fetch() { wget -qO "$2" "$1"; }
else
  fail "needs curl or wget"
fi

if command -v sha256sum > /dev/null; then
  sha256() { sha256sum "$1"; }
elif command -v shasum > /dev/null; then
  sha256() { shasum -a 256 "$1"; }
else
  fail "needs sha256sum or shasum"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# Archive names carry the version, so the checksums tell which archive the release has.
fetch "$base/checksums.txt" "$tmp/checksums.txt" || fail "download failed: $base/checksums.txt"
asset=$(awk -v re="^lazyfwd_[0-9][^_]*_${os}_${arch}[.]tar[.]gz\$" '$2 ~ re { print $2 }' "$tmp/checksums.txt")
[ -n "$asset" ] || fail "checksums.txt lists no archive for ${os}_${arch}"
expected=$(awk -v f="$asset" '$2 == f { print $1 }' "$tmp/checksums.txt")

echo "downloading $base/$asset" >&2
fetch "$base/$asset" "$tmp/$asset" || fail "download failed: $base/$asset"
actual=$(sha256 "$tmp/$asset" | awk '{ print $1 }')
[ "$expected" = "$actual" ] || fail "checksum mismatch for $asset"

tar -xzf "$tmp/$asset" -C "$tmp" lazyfwd
mkdir -p "$dir"
# Replace rather than overwrite, so a running lazyfwd keeps its inode and macOS its signature cache.
mv -f "$tmp/lazyfwd" "$dir/lazyfwd"
chmod 755 "$dir/lazyfwd"

echo "installed $("$dir/lazyfwd" --version) to $dir/lazyfwd" >&2
case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "add $dir to your PATH to run lazyfwd" >&2 ;;
esac
