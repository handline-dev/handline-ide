# agentctl

A Go library that drives coding-agent CLIs (Claude Code, opencode): it launches one in a terminal, types into it, reads its screen,
installs its state hooks and turns their reports into turns. It knows nothing of tasks, teams or the daemon's database.

Its own module, `github.com/handline/handline/go/agentctl`. The daemon at `go/` requires it with a `replace` to this folder.
It imports no package of the daemon: what it must know of the program that runs it comes in as values.

- `agent.Host`: the program's home, its control socket and its command names (hook lines and safe commands use them).
- `term.Place`: the tmux server, a key for the program's set of terminals, and the variables every new terminal gets.
- `agent.UserHome` and `agent.RunDir`: set once at start. The daemon sets all of these in `go/data/agenthost`.

| Package | What it is |
|---|---|
| `agent` | The interface an agent CLI meets, and its account facts |
| `agent/claudecode`, `agent/opencode`, `agent/fake` | The CLIs (`fake` is the `-tags fakes` build's) |
| `agent/vendors` | Links every CLI, so each registers itself |
| `term` | Terminal hosts (tmux, the fakes build's in-process one), screen reading and key presses |
| `delivery`, `peerport`, `peerinbox` | How a message reaches a session, and its receipt |
| `verdict` | Pure judgements of an agent's state |
| `ansi` | Styled screen rows |
| `dialogs` | The dialogs a CLI shows, and which are routine to answer |

Test it alone: `cd go/agentctl && GOWORK=off go test ./...`, and again with `-tags fakes`. `tools/test-go.sh ./agentctl/...` runs the first, `tools/test-go.sh --fakes` the second.
