# kit

The shared leaf utilities of the daemon and `hl`: its own Go module, `github.com/handline/handline/go/kit`. It imports no other
module of this repo; the daemon at `go/` requires it with a `replace` to this folder, and `go/go.work` lists it.

| Package | What it is |
|---|---|
| `paths` | The Handline home's layout, the live-home guard, the tmux server name and the scratch root |
| `protected` | What only the founder may say, kept where no agent can write it |
| `store` | Opens the daemon's sqlite database and brings its schema up (the base and later steps, with their manifest) |
| `actor`, `addr`, `names` | The keys that name a who (`p1`, `a17`) and the owner at first start; the words people type turned into keys and back; the rules for workspace names, agent names and handles |
| `joined` | The master a home joined (`hl device join`), kept in the protected store |
| `yamlcompat`, `textsafe`, `timefmt`, `words` | the YAML subset the config files use, text with no terminal controls, times as shown, a place (3rd) and a list (a, b and c) as a sentence says them |
| `policy`, `jsontext` | How long things are kept or wait, in one place; JSON as one fixed spaced text, so a hash or comparison of it is stable |
| `storetest`, `testfiles` | A migrated database for a test, the schema built once per test binary; a tree of files for a test |
| `atomicfile`, `lock`, `wslock`, `gitrun`, `peer`, `testkill`, `version` | Atomic writes, file locks, a workspace's shared or sole hold, git runs, who is on the other end of a unix socket, tearing down what a test starts, the build's sha |

Test it alone: `cd go/kit && GOWORK=off go test ./...`, which `tools/test-go.sh ./kit/...` runs too.
