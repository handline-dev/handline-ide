# Development

```bash
tools/test.sh     # the contract's generated files current, then the Go suite, its gates and the fakes build; no network, no tokens
tools/test.sh ./internal/reads -run X   # go test's own arguments: one package, one test
tools/test-go.sh          # only go test ./...
tools/test-go.sh --gates  # what is not a package's test: go.mod tidy, dupl, a cgo-free build, the conformance fixtures
tools/check.sh main   # what a change needs: nothing for docs; otherwise the app checks, the contract, the Go tests of the packages the change reaches (all for go.mod, migrations, contract/, tools/) and the gates
HANDLINE_GO_RACE=1 tools/test-go.sh   # the Go suite with go test -race, run on the exact sha before a Go change lands
```

`HANDLINE_LIVE=1 tools/live-e2e.sh` drives a real Claude Code through a test daemon on a scratch home and prints how
fast the event stream showed each step. It spends tokens on the account named in `HANDLINE_LIVE_ACCOUNT`
(`tools/live_account.py`), so it runs only by hand, never from `tools/test.sh`, `check.sh` or CI.

`tools/prune_worktrees.py` lists the worktrees of this repository that are done with, and why each other one stays; `--remove` removes them.

## React Compiler

The web app builds with the React Compiler, and lint holds the Rules of React it relies on (`packages/config/eslint/react.js`, the
`react-hooks` compiler rules as errors). They are rules of React, so they hold with or without the compiler.

Do:

- Read changing values from a hook's return.
- Derive during render instead of setting state in an effect.
- Keep refs for event handlers and effects.

Do not:

- Mutate props, state or a hook's return during render.
- Read or write `ref.current` in render.
- Call `Date.now()` or `Math.random()` in render (take the time from `useNow`).
- Define a component inside a component.
- Disable a `react-hooks` rule. `'use no memo'` only with a comment saying why.

The debt is the `compilerDebt` block in a package's `eslint.config.js` (`packages/app-data`, `packages/app-react`): files that still
break a rule sit at `warn` there. The list only shrinks; a file that is not in it gets the error.

Design notes are in `docs/`.

Working on the desktop app itself (the page with hot reload, a dev Electron instance next to the everyday one, pairing) is in
[app-dev-loop](app-dev-loop.md); running it on a real daemon in [run-on-real-daemon](run-on-real-daemon.md).
