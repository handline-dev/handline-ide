#!/usr/bin/env bash
# The Go daemon's required check, in two parts. `tools/test-go.sh [go test args]` runs only go test (all of ./... with no argument).
# `tools/test-go.sh --gates` runs what is not a package's test: go.mod tidy, dupl, a cgo-free build, the conformance fixtures.
# `tools/test-go.sh --fakes` runs the fakes build (-tags fakes): its packages' tests, then the fixtures with the tick on.
# tools/test.sh and tools/check.sh run all three; CI runs the packages in shards (tools/go-shard.sh) and each of the others in a job.
# -race only when HANDLINE_GO_RACE=1, as the auditor sets it for a sha: a race build doubles the build cache, and every worktree runs this.
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../go" || exit 1
if ! command -v go >/dev/null; then
    echo "tools/test-go.sh: no go on PATH; go test ./... is a required check" >&2
    exit 1
fi
. "$here/go-env.sh"
# The live tests spend real tokens: a shell that exported HANDLINE_LIVE still does not run them here.
unset HANDLINE_LIVE
# A scratch home for anything a test starts that reads one: never the live home of the agent running this. Its path is fixed
# per checkout, since go test keys its cache on the environment a test reads, and a fresh path each run would miss it.
scratch_home="${TMPDIR:-/tmp}/handline-go-home.$(printf %s "$here" | cksum | cut -d' ' -f1)"
rm -rf "$scratch_home" && mkdir -p "$scratch_home" || exit 1
trap 'rm -rf "$scratch_home"' EXIT
export HANDLINE_HOME="$scratch_home" HANDLINE_HOME="$scratch_home"
# The user home the tests and the daemons they start see: nothing they write lands in the real ~/.config or ~/.claude.
mkdir -p "$scratch_home/user" && export HANDLINE_TEST_ROOT="$scratch_home/user"
real_protected="$HOME/.config/handline/protected"
protected_before="$(ls -A "$real_protected" 2>/dev/null)"
protected_unchanged() {
    [ "$(ls -A "$real_protected" 2>/dev/null)" = "$protected_before" ] && return 0
    echo "FAIL  the Go tests wrote to the real $real_protected" >&2
    return 1
}
gates= fakes=
# The modules beside the daemon's, each tested and built on its own (go/go.work lists them all).
nested="agentctl kit protocol data accounts taskboard scheduler"
[ "${1:-}" = --gates ] && { gates=1; shift; }
[ "${1:-}" = --fakes ] && { gates=1 fakes=1; shift; }
race=()
[ "${HANDLINE_GO_RACE:-}" = 1 ] && race=(-race)
if [ -z "$gates" ]; then
    # A pattern under a nested module runs in that module, any other in the daemon's, and a flag goes to all; no pattern is all of each.
    main=() flags=() all=1
    for a in "$@"; do
        case "$a" in
        . | ./*) all= ;;
        *) flags+=("$a") ;;
        esac
    done
    for a in "$@"; do
        case "$a" in . | ./*) ;; *) continue ;; esac
        mine=1
        for m in $nested; do case "$a" in "./$m" | "./$m/"*) mine= ;; esac; done
        [ -n "$mine" ] && main+=("$a")
    done
    [ -n "$all" ] && main=(./...)
    if [ ${#main[@]} -gt 0 ]; then
        echo "tools/test-go.sh: go test ${race[*]} ${flags[*]} ${main[*]}" >&2
        go test "${race[@]}" "${flags[@]}" "${main[@]}" || exit 1
    fi
    for m in $nested; do
        pats=()
        [ -n "$all" ] && pats=(./...)
        for a in "$@"; do case "$a" in "./$m") pats+=(.) ;; "./$m/"*) pats+=(".${a#./$m}") ;; esac; done
        [ ${#pats[@]} -gt 0 ] || continue
        echo "tools/test-go.sh: $m: go test ${race[*]} ${flags[*]} ${pats[*]}" >&2
        (cd "$m" && GOWORK=off go test "${race[@]}" "${flags[@]}" "${pats[@]}") || exit 1
    done
    protected_unchanged || exit 1
    exit 0
fi
if [ -z "$fakes" ]; then
    # go.mod and go.sum tidy, and no new copy of Go code came in.
    "$here/check-go-tidy.sh" || exit 1
    for m in $nested; do "$here/check-go-tidy.sh" "$here/../go/$m" || exit 1; done
    "$here/check-go-dupl.sh" || exit 1
    CGO_ENABLED=0 go build -o /dev/null ./cmd/handlined || exit 1
    # Each nested module builds alone, with no workspace to lend it another module's packages; so does the daemon, through its replaces.
    for m in $nested; do (cd "$m" && GOWORK=off CGO_ENABLED=0 go build ./... && GOWORK=off go vet ./...) || exit 1; done
    GOWORK=off CGO_ENABLED=0 go build ./... || exit 1
else
    go test "${race[@]}" -tags fakes ./internal/layers || exit 1
    (cd data && GOWORK=off go test "${race[@]}" -tags fakes ./agenthost) || exit 1
    (cd agentctl && GOWORK=off go test "${race[@]}" -tags fakes ./term ./agent/...) || exit 1
fi

# The conformance fixtures against the built binary. Until the harness is on main there is nothing to run.
harness="$here/../contract/conformance.py"
if [ ! -f "$harness" ]; then
    echo "tools/test-go.sh: SKIPPED the conformance fixtures: contract/conformance.py is not on this branch yet" >&2
    exit 0
fi
work="$(mktemp -d "${TMPDIR:-/tmp}/handlined.XXXXXX")" || exit 1
trap 'rm -rf "$work" "$scratch_home"' EXIT
# The lists are kept one name per line in byte order, so two branches deleting neighbouring names merge instead of conflicting.
for list in "$here"/../go/conformance-{missing,needvar,skip,fakes-skip}.txt; do
    grep -v '^#' "$list" | LC_ALL=C sort -c 2>/dev/null || {
        echo "FAIL  $list is not sorted: run  grep -v '^#' it | LC_ALL=C sort -u  and keep the # header lines on top" >&2
        exit 1
    }
done
if [ -n "$fakes" ]; then "$here/build-go.sh" "$work/handlined" -tags fakes || exit 1; else "$here/build-go.sh" "$work/handlined" || exit 1; fi
# No socket file is bound (a sandbox may forbid it): tools/conformance_fds.py hands the daemon socketpairs, one per
# fixture, and mints the founder key in a scratch home; the agent role's socketpairs come from a process recorded as an agent's
# session there. A method the Go daemon does not serve yet is named in go/conformance-missing.txt: delete a line as its method
# lands. A fixture that needs a --var this run cannot seed yet is named in go/conformance-needvar.txt the same way, and one
# left failing on a known gap in go/conformance-skip.txt.
# The daemon runs with -claude and a fake CLI that answers --version.
FAKE_CLAUDE_VERSION=2.1.296
export FAKE_CLAUDE_VERSION
conformance() {
    python3 "$here/conformance_fds.py" "$work/handlined" "$@" --allow-missing-file "$here/../go/conformance-missing.txt" \
        --allow-needvar-file "$here/../go/conformance-needvar.txt" --skip-file "$here/../go/conformance-skip.txt" \
        "$here"/../contract/fixtures/*.json
}
if [ -n "$fakes" ]; then
    # The fakes run has its own skip list: what the tick recomputes under a seeded fixture (go/conformance-fakes-skip.txt).
    echo "tools/test-go.sh: conformance fixtures, the fakes build with the tick on" >&2
    python3 "$here/conformance_fds.py" "$work/handlined" --claude "$here/fake_claude.py" --fakes \
        --allow-missing-file "$here/../go/conformance-missing.txt" --allow-needvar-file "$here/../go/conformance-needvar.txt" \
        --skip-file "$here/../go/conformance-fakes-skip.txt" "$here"/../contract/fixtures/*.json || exit 1
    protected_unchanged
    exit
fi
echo "tools/test-go.sh: conformance fixtures, -claude" >&2
conformance --claude "$here/fake_claude.py" || exit 1
protected_unchanged
