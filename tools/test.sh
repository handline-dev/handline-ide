#!/usr/bin/env bash
# The tests, from the repo root: the generated contract files being current, then the Go suite and its gates (tools/test-go.sh, which takes
# go test's own arguments, so  tools/test.sh ./internal/reads -run X  runs one package).
cd "$(dirname "$0")/.." || exit 1
if [ $# -eq 0 ]; then
    python3 contract/generate.py --check || exit 1
    tools/test-go.sh && tools/test-go.sh --gates && exec tools/test-go.sh --fakes
fi
exec tools/test-go.sh "$@"
