#!/usr/bin/env bash
# Build the Go hl, which bin/hl runs, with the git sha stamped in as tools/build-go.sh stamps the daemon: tools/build-hl.sh [output]
top="$(cd "$(dirname "$0")/.." && pwd)" || exit 1
sha="$(git -C "$top" rev-parse --short HEAD)" || exit 1
cd "$top/go" || exit 1
. "$top/tools/go-env.sh"
out="${1:-$top/go/hl}"
CGO_ENABLED=0 go build -trimpath -ldflags "-X github.com/handline/handline/go/kit/version.Sha=$sha" -o "$out" ./cmd/hl || exit 1
