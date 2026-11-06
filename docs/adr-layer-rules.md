# ADR: layer rules for the Go packages

Decided by the architect, 2026-11-03 (roadmap P3-1, P3-6 to P3-8). Status: settled.
The table lives in `go/internal/layers/layers_test.go`; this page holds the rule and the reasons.

## Context

Packages grew imports in both directions: `reads` imported `lifecycle`, the write packages imported
`reads` only for schedule parsing, `db` leaned on `rpc`, and `cmd/hl` opened the database itself.
Every reader pulled in the whole daemon, and logic could not be tested alone.

## Decision

1. **A package imports only from its own layer or a lower one.** Within a layer Go already forbids
   cycles. `cmd/*` sits above everything.
2. The layers, lowest first: L0 leaf (no internal imports), L1 wire and infra (`rpc`, `term`,
   `claudecli`, ...), L2 `db`, L3 model (`states`, `team`, `prefs`, `usage`, ...), L4 domain writes
   and reads (`taskwrites`, `chatwrites`, `adminwrites`, `reads`, `supervisor`), L6 runners
   (`lifecycle`, `tick`, `watcher`, ...), L7 composition (`compose`, `daemon`).
3. A package not in the table goes in the lowest layer its imports allow.
4. **`cmd/hl` talks to the daemon only through RPC.** It imports `hlclient`, `rpc` and contract
   types, and never `db`, `store`, `lifecycle`, `compose` or `database/sql`.
   - Hooks write through `hook.record`. The Stop hook gets its nudge text in the result; if the
     daemon does not answer in time there is no nudge (fail open, as with a down daemon). A refusal
     that must block (PreToolUse) stays synchronous through the same call.
   - A verb's argv and chain ride as metadata on its RPC call; the daemon records the event.
   - `hl launch` still builds the launch argv, but its writes and room check sit behind a daemon
     method that returns what the CLI execs. A refused launch keeps its reasons.

## Enforcement

A Go test holds the table and fails on an upward import or on a package missing from it. It runs
in `tools/check.sh`. Another test fails if `cmd/hl` imports one of the forbidden packages.

## Consequences

- Shared pieces move down (schedule spec, settings, constants) instead of imports going up.
- A new dependency that needs an upward edge is a design question for the architect, not a table edit.
