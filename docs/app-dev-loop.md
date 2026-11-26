# The app dev loop

Developing the desktop app, `apps/web` (the page) and `apps/electron` (the shell), with edits shown as you type, against a real
daemon or the fake one. Commands run from the repo root after `pnpm install`.

To run the page and the shell against your team's own daemon and branch, use `pnpm dev <team>`: see
[dev-instances](dev-instances.md). The rest of this page is the parts it is made of.

## What runs where

The daemon listens on its Unix socket and, with `-listen`, on a network listener (127.0.0.1:7443) for pages. A page reaches it
by WebSocket at `/rpc` with a device token, and the listener takes a page's socket only from its own origin, so the shell
rewrites the Origin of its page's socket and vite's tunnel does the same. A pairing is kept in the page's storage per origin.
The shell also holds its own connection over the Unix socket (badge, notices).

## The page, hot-reloading

    HANDLINE_LISTEN_WS=127.0.0.1:7443 pnpm --filter @handline/web dev

vite serves the page on 127.0.0.1:5173 and tunnels `/rpc` to the listener. Open `http://127.0.0.1:5173/`, run
`hl device pair` and paste the code into the page. Edits reach the page in about a second (the config watches by polling).

Without a real daemon, use the fake: `HANDLINE_FAKE=1 pnpm --filter @handline/web dev`, then open
`http://127.0.0.1:5173/?daemon=ws://127.0.0.1:7790`. Its seeds are in `packages/fake-daemon/README.md`.

## The shell, next to the everyday app

    HANDLINE_ATTACH=1 \
    HANDLINE_DATA=~/.handline-dev-data \
    HANDLINE_DEV_URL=http://127.0.0.1:5173 \
    HANDLINE_LISTEN=127.0.0.1:7443 \
    pnpm --filter @handline/electron start

`HANDLINE_ATTACH=1` makes it use the daemon of its home and never start, stop or replace it; without it, a shell built from
another commit than the running daemon replaces that daemon. `HANDLINE_DATA` gives the instance its own data folder (settings, device token, layouts) and its own single-instance lock;
without it a second instance quits at once and focuses the running app. `HANDLINE_DEV_URL` loads the vite page instead of the
built `apps/web/dist`, so the tray, badge and notices run while the page hot-reloads. `HANDLINE_LISTEN` pins the listener
address the window dials.

If the window opens on the pairing screen, the instance could not pair through the daemon's socket: paste a `hl device pair`
code into the page, as in a browser.

Main-process files (`apps/electron/src/`) are outside hot reload: `start` rebuilds them, and a change there needs a relaunch.

## Pairing and devices

Every pairing is a device acting as the founder; the app's Devices list shows them and revokes one. Too many
wrong codes from one address lock pairing for it for a while ("too many wrong pairing codes"): run `hl device pair` again for
a fresh code instead of retrying.

Phones and Tailscale are in [run-on-real-daemon](run-on-real-daemon.md); the test suite is in [development](development.md).
