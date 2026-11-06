# scheduler

When and whether agent sessions run: the session lifecycle, the tick, and the resource-aware start. Its own Go module,
`github.com/handline/handline/go/scheduler`. It imports `go/agentctl`, `go/accounts`, `go/data`, `go/kit`, `go/protocol` and
`go/taskboard` only; the daemon at `go/` requires it with a `replace` to this folder, and `go/go.work` lists it.

| Package | What it is |
|---|---|
| `lifecycle` | Starting, closing, waking and moving sessions, and the single-instance hold |
| `session` | What is true of an agent's session right now |
| `tick`, `dangling` | What one pass decides for each agent, and work left hanging |
| `loadgate`, `procload`, `sysres`, `leases`, `keepawake` | Whether the machine has room to start more, simulator and browser leases, staying awake |
| `schedules`, `cronspec` | Messages typed at an agent on a cron schedule |
| `settings`, `staymove`, `worktreelock`, `worktreepaths` | Lifecycle settings, worktree moves, locks and paths |

Test it alone: `cd go/scheduler && GOWORK=off go test ./...`, which `tools/test-go.sh ./scheduler/...` runs too.
