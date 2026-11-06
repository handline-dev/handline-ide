# The app dev loop

Developing the desktop app — `apps/web` (the page) and `apps/electron` (the
shell) — with edits reflected while you type, against the real daemon or the
fake. The repo is two halves sharing one history: `ocp/` is the Python CLI
and hooks, `apps/`, `packages/`, `go/` are the Handline app. Commands run from
the repo root, `pnpm install` once. The old Textual bridge TUI (`ocp bridge`,
`ocp/ui/`) is deprecated — leave it be. Never run `ocp` code with the real
home (`~/.handline`) as `HANDLINE_HOME`: opening the db applies migrations, and the
live db's schema has diverged.

## What runs where

The daemon listens on its Unix socket and, with `-listen`, on a network
listener (127.0.0.1:7443) for pages: the page reaches it by WebSocket at
`/rpc` with a device token, and the listener takes a page's socket only from
its own origin — which is why the desktop shell rewrites the Origin of its
page's ws frames, and why a page served anywhere else is refused without the
same rewrite. A pairing is kept in the page's storage per origin, so origins
do not share tokens. The shell itself also holds a daemon connection over the
Unix socket (badge, notices, worktree lookups).

## The page, hot-reloading

    HANDLINE_LISTEN_WS=127.0.0.1:7443 pnpm --filter @handline/web dev

vite serves the page on 127.0.0.1:5173 and tunnels its ws `/rpc` to the
listener with the Origin the listener takes — the same rewrite the shell does;
vite's proxy `headers` option does not apply to ws upgrades, so the config
does the tunnel by hand. Open
`http://127.0.0.1:5173/?daemon=ws://127.0.0.1:5173/rpc`, run `hl device pair`
in another terminal and paste the code into the page. Edits reach the page in
about a second.

Without the env the tunnel is off and the config is an inert plugin. Without
a real daemon at all, use the fake:
`HANDLINE_FAKE=1 pnpm --filter @handline/web dev`, then open the page bare
(no `?daemon=`) — it dials the fake on port 7790.

**Common failure:** "Cannot reach the daemon" — a page opened without
`?daemon=` dials the fake daemon, not the real one; add the parameter.

## The shell, next to the everyday app

    HANDLINE_DATA=~/.handline-dev-data \
    HANDLINE_DEV_URL=http://127.0.0.1:5173 \
    HANDLINE_LISTEN=127.0.0.1:7443 \
    pnpm --filter @handline/electron start

`HANDLINE_DATA` gives the instance its own data folder (settings, device
token, layouts), which also keeps its single-instance lock apart — without it
a second instance quits at once and focuses the running app instead.
`HANDLINE_DEV_URL` loads the vite page instead of the built `apps/web/dist`,
so the whole shell — tray, badge, notices — runs while the page hot-reloads.
`HANDLINE_LISTEN` pins the listener address: the shell otherwise picks this
machine's Tailscale address and advertises its `*.ts.net` name, which a phone's
pairing link needs (`pnpm dev <team>` leaves it unset for that). A daemon
listening on loopback only is then unreachable and the page says so, retrying.

**Common failure:** the window opens on the pairing screen — the instance
could not pair through the daemon's socket; paste a `hl device pair`
code into the page, as with a browser.

To run the page and the shell against your team's own daemon and branch
instead, use `pnpm dev <team>`: see [dev-instances](dev-instances.md).

## One dev run per checkout

`pnpm dev <team>` runs once per checkout: a second one says which pid already runs and stops. Ctrl-C or a TERM stops the vite server,
the shell's watch and its Electron together, and the shell's watch also stops once `pnpm dev <team>` itself is gone. It is not a follow-main
run, so it shows no update notice. A save under `go/` (not a `_test.go`) runs `hl dev rebuild <team>` and prints its line: a failed
build prints its error and the old daemon keeps running, a good one restarts the daemon and the window reconnects by itself. Only when the
instance is built from this checkout; otherwise the run says so once and does not watch `go/`. An agent's run stays
in the background (apps/electron/README.md), with no menu bar icon and no update dialog either.

## Pairing and devices

Every pairing is a device acting as the founder: `hl device list` shows them,
`hl device revoke <id>` cancels one. A streak of wrong codes locks the
daemon's pairing for 15 minutes ("too many wrong pairing codes") — run
`hl device pair` again for a fresh code instead of retrying.

## Notes

- On this Mac the default file watcher lags ~25s behind edits, so the vite
  config watches with polling; edits reach the page in ~1s.
- Electron main-process files (`apps/electron/src/`) are outside HMR:
  `start` rebuilds the main process, and a change there needs a relaunch.
- Phones, Tailscale and the founder's own setup are in
  [run-on-real-daemon](run-on-real-daemon.md); the test suite is in
  [development](development.md).
