# data

The daemon's records and the team's files: its own Go module, `github.com/handline/handline/go/data`. It imports `go/kit`,
`go/protocol` and `go/agentctl` only; the daemon at `go/` requires it with a `replace` to this folder, and `go/go.work` lists it.

| Package | What it is |
|---|---|
| `db` | The statements on the daemon's tables |
| `states`, `rowstate` | The states an agent is in, what wakes each, and the words agent_state.state holds |
| `stream` | The event stream clients subscribe to |
| `team` | The agents' and accounts' files, routing, standing consent |
| `agenthost` | What the daemon tells agentctl about a Handline home |

Test it alone: `cd go/data && GOWORK=off go test ./...`, which `tools/test-go.sh ./data/...` runs too.
