#!/usr/bin/env bash
# Sourced by the Go scripts.
# A session's sandbox cannot write the default build cache; keep one that survives between runs.
cache="$(go env GOCACHE)"
if ! mkdir -p "$cache" 2>/dev/null || ! [ -w "$cache" ]; then
    export GOCACHE="${TMPDIR:-/tmp}/handline-gocache"
fi
# Without -trimpath each worktree's package paths are in the compiled output, so every worktree builds its own copy of the module's
# packages (measured: a second worktree adds 86 MB per build, 1 MB with it). Panics then show module paths, not worktree paths.
case " ${GOFLAGS-} " in *" -trimpath "*) ;; *) export GOFLAGS="${GOFLAGS:+$GOFLAGS }-trimpath" ;; esac
# At most once an hour, keep the cache under its size cap (20 GB, HANDLINE_GOCACHE_MAX_GB), longest-unused files first. In the
# background: the walk takes minutes. Never `go clean -cache` here: the files vanish under the builds running in other worktrees.
cache="$(go env GOCACHE)"
if [ -z "$(find "$cache/handline-pruned" -mmin -60 2>/dev/null)" ] && touch "$cache/handline-pruned" 2>/dev/null; then
    (python3 "$(dirname "${BASH_SOURCE[0]}")/gocache_prune.py" --dir "$cache" >/dev/null 2>&1 &)
fi
