#!/usr/bin/env bash
# Every check CI runs, as named groups; CI's jobs only set up and call `tools/check.sh --group <name>`, so local green is CI green.
# Usage: tools/check.sh [--group apps|files|go|contract|fakes|secrets|app-smoke] [--shard <go-shard.sh name>] [--plan] [<base>]  (base: main)
# No group runs all of them on the change against <base>; Go groups run for a change outside docs/ and design/. go tests only the modules
# tools/go-modules.py selects (a shard of another module says so and passes); without --shard, the packages the change reaches in them
# (CHECK_FULL=1: all). --plan prints kind=<none|docs|go> and apps=<no|app-data|turbo>, runs nothing.
# app-smoke (local only, a Mac): tools/app-smoke.mjs, run unasked for a change to the app's main, owner or follow. A failing step does not stop the rest.
cd "$(dirname "$0")/.." || exit 1
args=("$@")
group="" shard="" plan=""
while [ $# -gt 0 ]; do
    case "$1" in
        --group) group="${2:-}"; shift 2 ;;
        --shard) shard="${2:-}"; shift 2 ;;
        --plan) plan=1; shift ;;
        *) break ;;
    esac
done
base="${1:-main}"
case "$group" in ""|apps|files|go|contract|fakes|secrets|app-smoke) ;; *) echo "tools/check.sh: no group '$group'" >&2; exit 2 ;; esac
needs_node() { [ -d node_modules ] || { echo "tools/check.sh: no node_modules for the $1 checks: run  pnpm install --frozen-lockfile" >&2; return 1; }; }
# A step's output only when it fails.
quiet() { local out; out="$("$@" 2>&1)" || { printf '%s\n' "$out" >&2; return 1; }; }

# What the change touches: from where HEAD left <base>, not what main did since. No base (a first push) is no change.
case "$base" in ""|0000000000000000000000000000000000000000) base="" changed="" ;; *) changed="$(git diff --name-only --no-renames "$base...HEAD")" || exit 1 ;; esac
kind=none
while IFS= read -r path; do
    [ -n "$path" ] || continue
    case "$path" in
        contract/*|skill/*|AGENTS.md) kind=go; break ;;
        docs/*|design/*|*.md) [ "$kind" = none ] && kind=docs ;;
        *) kind=go ;;
    esac
done <<< "$changed"
apps=no
case "$kind" in go) grep -qE '^(apps|web|packages)/|^(package\.json|pnpm-lock\.yaml|pnpm-workspace\.yaml|turbo\.json|contract/package\.json|contract/protocol\.ts|contract/gen/.*\.ts)$' <<< "$changed" && apps=turbo || apps=app-data ;; esac
# A push to main runs the Go groups always and the apps only when they changed: that warms the caches a pull request reads. Its Go
# tests still cover only the modules the push reaches, against the commit before it.
if [ "${GITHUB_EVENT_NAME:-}" = push ]; then kind=go; [ "$apps" = turbo ] || apps=no; fi
[ -z "${CHECK_FULL:-}" ] || kind=go # the whole Go suite, even with nothing changed (the nightly run)
if [ -n "$plan" ]; then printf 'kind=%s\napps=%s\n' "$kind" "$apps"; exit 0; fi

# A local run needs disk (a Go build is gigabytes) and, with `heavy_max` set, a heavy slot: the machine's runs queue instead of all
# filling the disk and the cores at once. CHECK_MIN_FREE_GB (default 5) is the free space under which it does not start.
if [ "$kind" = go ] && [ -z "${GITHUB_ACTIONS:-}" ]; then
    free_gb="$(df -Pk "${TMPDIR:-/tmp}" | awk 'NR==2 {printf "%d", $4 / 1048576}')"
    if [ "${free_gb:-0}" -lt "${CHECK_MIN_FREE_GB:-5}" ]; then
        echo "tools/check.sh: only ${free_gb} GB free (floor ${CHECK_MIN_FREE_GB:-5} GB): not starting. Free space (tools/gocache_prune.py --cap-gb 10, old worktrees: tools/prune_worktrees.py) and run again." >&2
        exit 1
    fi
    if [ -z "${CHECK_HEAVY_HELD:-}" ] && command -v hl >/dev/null && hl use 2>&1 | grep -q heavy; then
        export CHECK_HEAVY_HELD=1
        exec hl use heavy -- "$0" "${args[@]}"
    fi
fi

want() { [ -z "$group" ] || [ "$group" = "$1" ]; }
# A Go group has nothing to run for a change that is not Go.
golike() { [ "$kind" = go ] && { want go || want contract || want fakes; }; }
gotest() {
    if [ -n "${GITHUB_ACTIONS:-}" ]; then tools/leakcheck.py run --dir "$RUNNER_TEMP/hlc" tools/test-go.sh "$@"
    else tools/test-go.sh "$@"; fi
}

group_apps() {
    [ "$apps" = no ] || needs_node app || return 1
    if [ "$apps" = turbo ]; then
        local r=0
        pnpm turbo test typecheck lint --continue || r=1
        tools/dupes.sh || r=1
        return "$r"
    elif [ "$apps" = app-data ]; then pnpm --dir packages/app-data test
    else echo "no app change: the app checks are not run" >&2; fi
}

group_files() {
    needs_node file || return 1
    local r=0 pretty
    quiet python3 -m unittest tools/test_pr_checks.py tools/test_check_hl_commands.py tools/test_go_modules.py tools/test_app.py tools/test_leakcheck.py tools/test_gocache_prune.py tools/test_check_guard.py || r=1
    python3 tools/check_hl_commands.py || r=1
    node tools/ui-copy-check.mjs || r=1
    [ -n "$base" ] || return "$r"
    python3 tools/pr_checks.py files "$base" || r=1
    python3 tools/localrefs.py authors "$base..HEAD" || r=1  # a push to main has no pr-rules run; a clone without the hooks still gets caught here
    pretty="$(git diff --name-only --no-renames --diff-filter=AM "$base...HEAD" |
        grep -E '^((packages|apps|web)/.*\.(ts|tsx|js|mjs|json|css)|[^/]*\.json)$')"
    [ -z "$pretty" ] || xargs pnpm exec prettier --check <<< "$pretty" || r=1
    return "$r"
}

# The Go modules the change needs tested (tools/go-modules.py says which and why), one directory under go/ a line; every module with
# no base or CHECK_FULL=1.
go_modules() {
    if [ -z "$base" ]; then tools/go-modules.py --all "there is no base to compare with"
    elif [ -n "${CHECK_FULL:-}" ]; then tools/go-modules.py --all "CHECK_FULL=1"
    else printf '%s\n' "$changed" | tools/go-modules.py -; fi
}

group_go() {
    local pkgs mods m
    mods="$(go_modules)" || return 1
    if [ -n "$shard" ]; then
        m=.
        [ ! -f "go/$shard/go.mod" ] || m="$shard" # a nested module's shard is named after it; the daemon's are not
        grep -qxF "$m" <<< "$mods" || { echo "shard $shard: its module is not selected, nothing to test" >&2; return 0; }
        pkgs="$(tools/go-shard.sh "$shard")" && [ -n "$pkgs" ] || { echo "no packages for shard $shard" >&2; return 1; }
    else
        pkgs="$(tools/go-affected.py $changed)" || pkgs=ALL
        [ -z "${CHECK_FULL:-}" ] || pkgs=ALL
        if [ "$pkgs" = ALL ]; then
            pkgs=""
            for m in $mods; do if [ "$m" = . ]; then pkgs="$pkgs ./..."; else pkgs="$pkgs ./$m/..."; fi; done
            [ -n "$pkgs" ] || { echo "no Go module is selected: nothing to test" >&2; return 0; }
        fi
    fi
    local r=0
    [ -z "$pkgs" ] || gotest $pkgs || r=1
    # agentctl picks its terminal host per OS (term/hosts): it must keep building for Windows, which has no tmux.
    if grep -qxF agentctl <<< "$mods" && { [ -z "$shard" ] || [ "$shard" = agentctl ]; }; then
        GOOS=windows go -C go/agentctl build ./... || r=1
    fi
    return "$r"
}

group_contract() {
    local r=0
    python3 contract/generate.py --check || r=1
    python3 contract/test_generate_app.py || r=1
    python3 tools/go_schema.py --check || r=1
    gotest --gates || r=1
    return "$r"
}

group_fakes() { gotest --fakes; }

# The app on a scratch home through a follow restart, a quit and a kill -9. Not a CI job: it needs a Mac.
group_smoke() { [ "$(uname -s)" = Darwin ] || { echo "tools/check.sh: app-smoke needs a Mac" >&2; return 1; }; node tools/app-smoke.mjs; }

# Only what the change adds: <base>..HEAD; a first push has no base, so the head commit alone.
group_secrets() {
    local version=8.30.1 sha256=551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb  # gitleaks_<version>_checksums.txt, linux_x64
    local bin range tmp f
    bin="$(command -v gitleaks)" || {
        [ "$(uname -sm)" = "Linux x86_64" ] || { echo "tools/check.sh: gitleaks is not installed: brew install gitleaks" >&2; return 1; }
        # The release binary, pinned and checked: gitleaks-action needs a paid licence for an organisation's repository.
        tmp="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"; f="gitleaks_${version}_linux_x64.tar.gz"
        curl -sSfL -o "$tmp/$f" "https://github.com/gitleaks/gitleaks/releases/download/v$version/$f" &&
            echo "$sha256  $tmp/$f" | sha256sum -c - && tar -xzf "$tmp/$f" -C "$tmp" gitleaks || return 1
        bin="$tmp/gitleaks"
    }
    if [ -n "$base" ]; then range="$base..HEAD"; else range="-1 HEAD"; fi
    "$bin" git --no-banner --redact --log-opts="$range" .
}

if golike; then
    if [ -z "${GITHUB_ACTIONS:-}" ]; then
        run="$(tools/leakcheck.py begin "$$")" || exit 1
        export TMPDIR="$run/tmp/" TMUX_TMPDIR="$run"
        # Stopped by a signal, the run still says what it left and takes its directory with it, once the step it was in has ended.
        stopped() { trap - INT TERM HUP; tools/leakcheck.py end --interrupted "$run"; exit "$1"; }
        trap 'stopped 130' INT; trap 'stopped 143' TERM; trap 'stopped 129' HUP
    fi
fi
rc=0
if [ "$kind" = none ]; then echo "nothing changed against $base: no tests to run" >&2; fi
want secrets && { group_secrets || rc=1; }
if [ "$group" = app-smoke ] || { [ -z "$group" ] && [ -z "${GITHUB_ACTIONS:-}" ] && [ "$(uname -s)" = Darwin ] &&
    grep -qE '^apps/electron/src/(main|owner|own|follow|follow-app|leftovers|relaunch|window-place)\.ts$|^apps/electron/(launch|build)\.ts$|^tools/app-smoke\.mjs$' <<< "$changed"; }; then
    group_smoke || rc=1
fi
want files && { group_files || rc=1; }
[ "$kind" = go ] && want apps && { group_apps || rc=1; }
if [ "$kind" = docs ] && [ -z "$group" ]; then echo "documentation only: the suite is not run" >&2; fi
[ "$kind" = go ] && want contract && { group_contract || rc=1; }
[ "$kind" = go ] && want go && { group_go || rc=1; }
[ "$kind" = go ] && want fakes && { group_fakes || rc=1; }
[ -n "${run:-}" ] && { tools/leakcheck.py end "$run" || rc=1; }
exit "$rc"
