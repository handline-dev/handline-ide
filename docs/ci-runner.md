# CI

`.github/workflows/check.yml` runs on GitHub's `ubuntu-latest` runners for a pull request, the merge queue and each push to
main. No job needs macOS or a machine of ours. Each job only sets up and calls `tools/check.sh --group <name>`, so local green
is CI green; `tools/check.sh --plan` says first whether the change needs the job at all.

- `apps`: the web, mobile and electron tests, typecheck and lint.
- `daemon (<shard>)`: the Go suite, one job per shard of packages in parallel. `tools/go-shard.sh` holds the lists; `other` is
  every package no named shard lists, so a new package is tested without a line there.
- `contract`: the generated contract files and Go schema being current, and the conformance fixtures
  (`tools/test-go.sh --gates`).
- `contract-fakes`: the fakes build (`-tags fakes`), beside the gates.
- `files`: what a reviewer would otherwise catch in the files the change adds or edits, and Prettier on them.
- `secrets`: the change's commits hold no secret.
- `go`: green only when every shard and both contract jobs passed; `pre-audit`: green only when `apps`, `go`, `files` and
  `secrets` passed.

Tools: Go comes from `actions/setup-go` (the version in `go/go.mod`), which restores a module and build cache keyed by the
modules' `go.sum`. The Go jobs install `tmux` with `tools/ci-tmux.sh`, for the terminal tests; `git` and `python3` are on the
image. On CI the Go suite runs under `tools/leakcheck.py`, which fails it when it leaves a process, tmux server or temp file
behind.

`pr-rules.yml` checks a pull request's text and base (`tools/pr_checks.py body`) and runs `tools/localrefs.py` on its commit
messages and added lines.

Other workflows: `nightly.yml` runs the whole Go suite with `-race` on main's head once a day; `perf.yml` measures how much the
web app draws; `mac-release.yml` builds the Mac app's releases.
