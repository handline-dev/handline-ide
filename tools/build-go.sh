#!/usr/bin/env bash
# Build the Go daemon, one binary with the git sha stamped in and no cgo: tools/build-go.sh [output [go build flags]], e.g. -tags fakes
top="$(cd "$(dirname "$0")/.." && pwd)" || exit 1
sha="$(git -C "$top" rev-parse --short HEAD)" || exit 1
cd "$top/go" || exit 1
. "$top/tools/go-env.sh"
out="${1:-$top/go/handlined}"
[ $# -gt 0 ] && shift
CGO_ENABLED=0 go build -trimpath -ldflags "-X github.com/handline/handline/go/kit/version.Sha=$sha" "$@" \
    -o "$out" ./cmd/handlined
