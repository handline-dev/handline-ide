# taskboard

The task board's core: its writes, the telling layer that sends and delivers, handoffs and what a task carries. Its own Go module,
`github.com/handline/handline/go/taskboard`. It imports `go/agentctl`, `go/data`, `go/kit` and `go/protocol` only; the daemon at
`go/` requires it with a `replace` to this folder, and `go/go.work` lists it. The board's reads and admin writes stay in the daemon.

| Package | What it is |
|---|---|
| `taskwrites` | The task board's write methods |
| `supervisor` | The telling layer: routing, sends, delivery, landing and the task queue |
| `handoff` | A session's record of itself: the fold digest, the handoff record and the agent's own note |
| `taskresult`, `attachments` | A task end's commit and screenshots; the one way an image or file enters the daemon |
| `requestref`, `watch` | A request named by id or ref; what a gate task can watch |
| `msgtest` | For tests: a printed message name read back as its id |

Test it alone: `cd go/taskboard && GOWORK=off go test ./...`, which `tools/test-go.sh ./taskboard/...` runs too.
