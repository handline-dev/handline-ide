#!/usr/bin/env bash
# Build a release's handlined and hl for one architecture, for CI and local packaging: tools/build-darwin.sh <version> <arm64|amd64> [outdir]
# Both are stripped of the symbol table and DWARF (-s -w; a panic still names its functions) and stamped <version>+<sha> (-dirty when the tree
# has changes), which `handlined -version` prints. outdir defaults to dist/darwin.
version="$1"
arch="$2"
if [ -z "$version" ] || [ $# -lt 2 ] || [ $# -gt 3 ] || [[ "$version" =~ [^A-Za-z0-9._-] ]] || { [ "$arch" != arm64 ] && [ "$arch" != amd64 ]; }; then
    echo "usage: tools/build-darwin.sh <version> <arm64|amd64> [outdir]  (version: letters, digits, . _ -)" >&2
    exit 2
fi
top="$(cd "$(dirname "$0")/.." && pwd)" || exit 1
out="${3:-$top/dist/darwin}"
sha="$(git -C "$top" rev-parse --short HEAD)" || exit 1
[ -z "$(git -C "$top" status --porcelain)" ] || sha="$sha-dirty"
stamp="$version+$sha"
cd "$top/go" || exit 1
. "$top/tools/go-env.sh"
mkdir -p "$out" || exit 1
for cmd in handlined hl; do
    GOOS=darwin GOARCH="$arch" CGO_ENABLED=0 go build -trimpath \
        -ldflags "-s -w -X github.com/handline/handline/go/kit/version.Sha=$stamp" -o "$out/$cmd" "./cmd/$cmd" || exit 1
done
# The binary says its version only where it can run: on its own architecture.
host="$(uname -m)"; [ "$host" = x86_64 ] && host=amd64
if [ "$host" = "$arch" ]; then
    [ "$("$out/handlined" -version)" = "$stamp" ] || { echo "tools/build-darwin.sh: $out/handlined does not say $stamp" >&2; exit 1; }
fi
echo "$out: handlined, hl $stamp ($arch)"
