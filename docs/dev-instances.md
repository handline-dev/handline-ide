# Team dev instances

Each team (`handline.daemon`, `handline.pc`, `handline.mobile`) can run its own
daemon from its own branch, beside the live one. An instance lives in
`~/.handline-dev/<team>/` with its own database, socket, protected store,
listener port (7451, 7452 and 7453) and launchd job. It never starts, wakes or
types into an agent session.

## Turning it on and off

Run these from a plain shell, with no `HANDLINE_HOME` set.
`hl dev on` snapshots whatever home `hl` points at, so with `HANDLINE_HOME` at
another team's dev home it copies that one instead of the live home.

    hl dev on <team> [--worktree PATH] [--listen ADDR] [--fresh]
    hl dev status
    hl dev rebuild <team>
    hl dev off <team>

- `on` builds `handlined` from the worktree (default: the team manager's) and
  starts the daemon, which serves its own network listener. The first `on` copies the live database
  read-only; later ones keep the instance's own copy unless `--fresh` is given.
  It prints `instance.json`.
- `rebuild` builds the worktree again and restarts the job onto the new binary.
- `off` stops the job (and a gateway job an older `on` left). The folder stays, so the next `on` picks up where it left off.
- `on` refuses a home that resolves to the live one, and a listener port in use.

From an agent session only the team's manager may change its instance. The
check reads `HANDLINE_AGENT`, so it is advisory, not a lock.

## Using it

- **hl:** `env -u HANDLINE_HOME HANDLINE_HOME=~/.handline-dev/<team> hl ...` talks to
  the instance. With `HANDLINE_HOME` still naming the live home, hl refuses.
- **Desktop app:** `pnpm dev <team>` runs the vite page and the shell
  (rebuilt on every save) against the instance, with an app data folder of its own
  (`<instance>/app-data`). When the instance is off it says so and stops; it
  never falls back to the live daemon. See [app-dev-loop](app-dev-loop.md).
- **Phone:** the daemon listens on `127.0.0.1` by default, which a phone cannot
  reach. Start it on the Tailscale address with
  `hl dev on handline.mobile --listen :7453`, then pair with
  `env -u HANDLINE_HOME HANDLINE_HOME=~/.handline-dev/handline.mobile hl device pair`.
  The link names the dev daemon, so the phone pairs with that instance only.
