# protocol

The daemon's side of the wire, as `contract/protocol.md` sets it out: its own Go module, `github.com/handline/handline/go/protocol`.
It imports `go/kit` only; the daemon at `go/` requires it with a `replace` to this folder, and `go/go.work` lists it.

| Package | What it is |
|---|---|
| `rpc` | The JSON-RPC 2.0 connection: framing, the hello handshake, the method table, errors and writes |
| `authority` | Who a caller is, on proof only, from the founder's protected store |
| `sessiontoken` | The proof that a request comes from an agent's own session |
| `devices` | Who may use the network listener: pairing codes and device tokens |
| `operations` | The long-running operations this process runs |
| `pairsock` | A client connected to a test daemon with no socket file |

Test it alone: `cd go/protocol && GOWORK=off go test ./...`, which `tools/test-go.sh ./protocol/...` runs too.
