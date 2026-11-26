# A master and runners on one Mac

`hl dev cluster up [--runners N]` builds `handlined` and `hl` from the checkout you are in (or `--worktree`), starts a
master on `127.0.0.1:7461` and runners on 7462 upward, each with its own home under `~/.handline-dev/cluster`, and joins
each runner with `hl device join`, set to Run. The nodes' protected stores are under `~/.handline-dev/cluster/user`,
never the real one. `status` prints the line to talk to each node; `down` stops them and keeps the homes and their
`handlined.log`. The next `up` starts afresh. Loopback and plain HTTP only.

Go tests get the same through `internal/clustertest`: `clustertest.Start(t, opts)` returns the joined nodes (`Call`,
`Hl`, `Kill`, `Stop`, `Restart`) and stops and removes them when the test ends.
