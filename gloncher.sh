#!/usr/bin/env bash
# gloncher.sh — detect the OS/arch and exec the matching gloncher binary.
#
# Binaries are looked for next to this script, then in ./dist, and are named
# gloncher-<os>-<arch>[.exe] to match the release matrix in the Makefile.
# Change one, change the other.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

case "$(uname -s)" in
  Darwin)                     os="darwin"  ;;
  Linux)                      os="linux"   ;;
  CYGWIN* | MINGW* | MSYS*)   os="windows" ;;
  *) echo "gloncher.sh: unsupported OS $(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
  x86_64 | amd64)   arch="amd64" ;;
  arm64 | aarch64)  arch="arm64" ;;
  *) echo "gloncher.sh: unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac

name="gloncher-${os}-${arch}"
[[ "$os" == "windows" ]] && name="${name}.exe"

plain="gloncher"
[[ "$os" == "windows" ]] && plain="gloncher.exe"

# Release bundles carry per-platform names; a plain `make build` in a checkout
# produces ./gloncher. Accept either, so the script works in both.
for candidate in \
  "$script_dir/$name" \
  "$script_dir/dist/$name" \
  "$script_dir/bin/$name" \
  "$script_dir/$plain" \
  "$script_dir/dist/$plain"
do
  if [[ -x "$candidate" ]]; then
    exec "$candidate" "$@"
  fi
done

# Nothing prebuilt. In a source checkout with Go available, just build it —
# beats telling someone to run a command we could have run ourselves.
if [[ -f "$script_dir/go.mod" ]] && command -v go >/dev/null 2>&1; then
  echo "gloncher.sh: no binary yet, building ${name}..." >&2
  if (cd "$script_dir" && go build -o "$name" .); then
    exec "$script_dir/$name" "$@"
  fi
  echo "gloncher.sh: build failed" >&2
  exit 1
fi

echo "gloncher.sh: no binary for ${os}/${arch} (looked for $name)" >&2
echo "build one with: make build   — or: make release for all platforms" >&2
exit 1
