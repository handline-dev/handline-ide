# kit

The shared leaf utilities of the daemon and `hl`: its own Go module, `github.com/handline/handline/go/kit`. It imports no other
module of this repo; the daemon at `go/` requires it with a `replace` to this folder, and `go/go.work` lists it.

| Package | What it is |
|---|---|
| `paths` | The Handline home's layout, the live-home guard, the tmux server name and the scratch root |
| `protected` | What only the founder may say, kept where no agent can write it |
| `store` | Opens the daemon's sqlite database and brings its schema up (the base and later steps, with their manifest) |
| `actor`, `names` | The keys that name a who (`p1`, `a17`) and the owner at first start; the rules for workspace names, agent names and handles |
| `yamlcompat`, `textsafe`, `timefmt` | YAML as Python wrote it, text with no terminal controls, times as shown |
| `policy`, `pyjson` | How long things are kept or wait, in one place; JSON written as Python's json.dumps writes it |
| `storetest` | A migrated database for a test, the schema built once per test binary |
| `atomicfile`, `lock`, `gitrun`, `peer`, `testkill`, `version` | Atomic writes, file locks, git runs, who is on the other end of a unix socket, tearing down what a test starts, the build's sha |

Test it alone: `cd go/kit && GOWORK=off go test ./...`, which `tools/test-go.sh ./kit/...` runs too.
