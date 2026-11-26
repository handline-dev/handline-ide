# scheduler

When and whether agent sessions run: the session lifecycle, the tick, and the resource-aware start. Its own Go module,
`github.com/handline/handline/go/scheduler`. It imports `go/agentctl`, `go/accounts`, `go/data`, `go/kit`, `go/protocol` and
`go/taskboard` only; the daemon at `go/` requires it with a `replace` to this folder, and `go/go.work` lists it.

| Package | What it is |
|---|---|
| `lifecycle` | Starting, closing, waking and moving sessions |
| `session` | What is true of an agent's session right now |
| `tick`, `dangling` | What one pass decides for each agent, and work left hanging |
| `loadgate`, `procload`, `sysres`, `agentload`, `pause` | Whether the machine has room to start more, what each agent's processes hold, the founder's switch that holds new work |
| `leases`, `leasestore`, `runs`, `keepawake` | Shared resources (ports, a workspace's shared runs) taken in turn, staying awake |
| `schedules`, `cronspec` | Messages typed at an agent on a cron schedule |
| `settings`, `staymove`, `access` | Lifecycle settings, account moves, the paths a session may read without asking |
| `worktreepool`, `worktreesetup`, `worktreelock`, `worktreepaths` | A project's desks (the worktrees agents take), their setup script, locks and paths |

Test it alone: `cd go/scheduler && GOWORK=off go test ./...`, which `tools/test-go.sh ./scheduler/...` runs too.
