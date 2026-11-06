# ADR: judgement functions live in `internal/verdict`

Decided by the architect, 2026-11-03 (roadmap P4-1). Status: settled.

## Context

"Is the agent busy", "is it stuck" and "may a session start on this account" each had several
definitions that disagreed. The callers do not ask the same question, but they must not each
re-read the session by hand.

## Decision

New package `internal/verdict`, layer 0: no I/O, no internal imports. Inputs are plain values; the
caller turns its own types (inbox session, turn state, terminal blocker) into them. `policy` keeps
retention constants; a judgement is a different thing.

```go
func AgentState(in Input) State          // the one reading of a session
func Admit(a Account) Admission          // may a session start on this account
func ParseStuckKind(s string) StuckKind  // the stored stall kind; StuckNone if unknown
```

- `State`: Unknown, Starting, Working, IdleWithBackground, Idle, WaitingPermission, Dialog,
  ShellLeft, Closed. A known turn decides before the session file does.
- A permission prompt is not one answer, so there is one state and named folds beside it:
  `Busy` (screen; WaitingPermission is not busy), `SafeToMove` (only Idle), and for restarts
  only Working must not be interrupted. No caller folds by hand.
- `Admit` returns `Admission{Best, NeverRun, Fits, Reasons}`. Being removed, set aside or another
  vendor's refuses outright. Otherwise Best needs an admitting reading with room; NeverRun
  (no reading yet) and Fits (nothing known is against it) are the fallbacks. `Reasons` lists
  everything against the account, so the screen shows what the assignment used.
- `StuckKind` (trust, dialog, send_dialog, start, wake) is stored beside the stalled row. Nothing
  parses it back out of free text. `NeedsPerson` says whether a person must answer.

## Not a verdict

Process liveness is one `lock.Alive(pid)` where EPERM means alive (the process exists).

## Consequences

- Judgement is tested without a database. Callers differ only in which fold they call.
- A new rule is added in `verdict`, once, with its test.
