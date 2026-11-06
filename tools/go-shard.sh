#!/usr/bin/env bash
# The packages of one CI shard of the Go suite, one `go test` pattern per line: `tools/go-shard.sh <shard>`. `other` is every
# package no named shard lists, so a new package is tested without a line here. Sized by their run time, the long pole alone.
cd "$(dirname "$0")/../go" || exit 1
named() {
    case "$1" in
    compose) echo ./internal/compose ;;
    cli) printf './cmd/%s\n' hl handlined ;;
    daemon) printf './internal/%s\n' daemon sessionrpc reads ;;
    terminal) echo ./internal/termchan ;;
    agentctl) echo ./agentctl/... ;;   # its own module: the library that drives agent CLIs
    kit) echo ./kit/... ;;             # its own module: the shared leaf utilities
    protocol) echo ./protocol/... ;;   # its own module: the wire, authority and pairing
    data) echo ./data/... ;;           # its own module: the store and team config
    accounts) echo ./accounts/... ;;   # its own module: usage, limits and logins
    taskboard) echo ./taskboard/... ;; # its own module: tasks, handoffs and the supervisor
    scheduler) echo ./scheduler/... ;; # its own module: session lifecycle and the resource-aware start
    *) return 1 ;;
    esac
}
if [ "${1:-}" = other ]; then
    all="$(for s in compose cli daemon terminal; do named $s; done)"
    go list ./... | sed 's#^github.com/handline/handline/go#.#' | grep -vxF "$all"
else
    named "${1:-}" || { echo "usage: tools/go-shard.sh compose|cli|daemon|terminal|agentctl|kit|protocol|data|accounts|taskboard|scheduler|other" >&2; exit 2; }
fi
