# accounts

The agent accounts: their usage, limits and logins. Its own Go module, `github.com/handline/handline/go/accounts`. It imports
`go/agentctl`, `go/data` and `go/kit` only; the daemon at `go/` requires it with a `replace` to this folder, and `go/go.work` lists it.

| Package | What it is |
|---|---|
| `usage` | What the accounts screens read: per-account settings, status, the month cycle and pacing |
| `usagepoll` | Asks accounts where they stand and keeps their readings fresh |
| `claudecli` | Runs a Claude CLI in stream-json mode, for the usage poll |
| `subscriptions` | Two upkeep sweeps over the accounts, every tick |
| `weblogin` | An account's login for the web page, with no terminal |
| `livetest` | What the live tests share (they run only with `HANDLINE_LIVE=1`) |

Test it alone: `cd go/accounts && GOWORK=off go test ./...`, which `tools/test-go.sh ./accounts/...` runs too.
