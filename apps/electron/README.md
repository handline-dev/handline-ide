# @handline/electron

The desktop shell: one window on the built `apps/web`, one instance, a dock badge from the inbox count, native notifications.

    pnpm dev <team>

The main process serves `apps/web/dist` on a loopback port and holds its own daemon connection (the Unix socket) for the badge and the notices. The web app is unchanged and reaches the daemon by its WebSocket.

| Env (`HANDLINE_`) | Default                                                        |
| ----------------- | -------------------------------------------------------------- |
| `SOCKET`          | `<home>/daemon.sock`, home is `HANDLINE_HOME` or `~/.handline` |
| `WS`              | the fake daemon (`ws://127.0.0.1:7777`)                        |
| `WEB_DIST`        | `apps/web/dist`                                                |

## The daemon outlives the app

The app keeps this Mac's daemon running and the daemon outlives it: quitting Handline (Cmd-Q or the tray's _Quit Handline_, which asks
first while the daemon runs, with _Don't show again_) closes the app only, and the daemon and the agents keep working. At launch the app
adopts a daemon that answers with its own build, restarts one of another build once (the app was updated), and replaces one that holds the
home's lock but does not answer. While the app runs, a daemon that exits on its own is started again once; a second exit within a minute
stays stopped, with the reason in the tray. The tray, top to bottom: the daemon's status (with how many agents work), _Open Handline_,
_Needs you_, _Open at login_, _Restart daemon_, _Stop daemon…_ (asks first), _Quit Handline_; Option adds the address, the build and the
commit. _Open at login_ (on by default) opens the app at login in the tray only, with no window and no dock icon until it is opened.

Instead of this Mac's daemon the app can use another device's, paired from the pairing screen (the `handline://` link `hl device pair`
prints, or an address and a code); Settings > Connection goes back to this Mac's. That pairing and the settings are kept in
`settings.json` in the app's data folder, mode 0600. `HANDLINE_ATTACH=1` makes a dev run attach to the daemon of its home and never start
or stop one (`hl dev`, `tools/try-prototype.sh`).

## On the real daemon

From the repo root:

    pnpm install && pnpm app

It starts the app: it puts a tray icon up at once, opens the window (which shows the daemon starting), and adopts or starts the daemon as described above (`go/handlined` of the checkout, with its network listener; `HANDLINE_HANDLINED` names another binary and `HANDLINE_CLAUDE` the claude the daemon runs; build it with `go build -o handlined ./cmd/handlined` in `go/`). Closing the window leaves it running in the tray. It pairs itself: the app is the founder's own process, so it reads the founder key from the protected store, asks the daemon's socket for a code and trades it for a device token, in the main process only. The key is not logged, stored or sent to the page; the token is kept in the app's data folder (`device.json`, mode 0600) and put in the page's storage by the preload. A token the daemon no longer lists is replaced at the next launch. There is no pairing step and no `hl device pair`; the pairing screen shows only if the app could not pair. The daemon listens on `127.0.0.1:7443` (`HANDLINE_LISTEN=host:port` moves it); the page is served on `127.0.0.1:17443` (`HANDLINE_WEB_PORT`) because the token is kept per page origin, and the shell sets the page's `Origin` to the listener's own for that one socket. The daemon's output goes to `daemon.log` in the app's data folder. The badge and notices still use the daemon's Unix socket.

A device the daemon no longer knows (revoked, or a token from an older pairing) is dropped at the next launch and the window asks for a code again.

Not in: a proposal notice (the contract has no event for it), a notice's click takes the page to the route of the event's target through `window.handline.onNavigate` (the card routes arrive with U9 and U10), a Windows badge (count only on Linux, none on Windows).

The page's side is `window.handline` (docs/electron-bridge.md): `screen` (which agent is on screen, so no notice for it), `unsent` (whether a restart would lose something) and `onNavigate`. `src/preload.ts` is bundled to `dist/preload.cjs`; every call is checked for its sender and its arguments in `src/bridge.ts`. Until the page has called `unsent` once, the shell probes the page itself.

## Packaging

    pnpm install && pnpm package [version]

Makes `apps/electron/release/Handline-<version>-universal.dmg` and `-mac.zip`: the web build, the main process, the universal `handlined` and `hl` from `tools/build-darwin.sh`, then electron-builder (`electron-builder.config.mjs`). The app carries the web build and both binaries under `Contents/Resources` (`src/bundle.ts`); an installed app serves that page and runs its own `Contents/Resources/bin/handlined`, as a checkout run does; the daemon keeps running when the app quits. After an update swaps the bundle, the relaunched app finds the daemon of the old build and starts the new one once. At launch it unloads and removes the launchd job an older Handline installed (`dev.handline.daemon`, `src/old-service.ts`), so nothing starts a second daemon under it. Settings > Connection offers to link `hl` from inside the bundle into `~/.local/bin`; when the app moves, the row shows it unlinked and links it again.

Without a Developer ID the app is signed ad hoc (`ad-hoc-sign.mjs`): it runs on the Mac that built it, and anywhere else the first open needs right-click > Open. To sign with a Developer ID, set `HANDLINE_SIGN_IDENTITY` to its name before `pnpm package` (hardened runtime and `assets/entitlements.mac.plist` then apply). Notarization is off (`notarize: false`) until there is an Apple account.

## Following main

    pnpm install && pnpm app

On a Mac `pnpm app` runs a copy of Electron.app named Handline (`~/.local/share/handline/Handline-<electron version>.app`, `HANDLINE_BUNDLE_DIR` moves them; one per Electron version, made on the first run), so the menu bar and the dock say Handline; if the copy cannot be made they run plain Electron and warn. The executable inside keeps the name Electron, because Electron takes a renamed one for a packaged app. "Restart now" starts the app again through the same launcher, so a new Electron version gets its own copy.

Run it from the checkout of main, once. It makes the app's own clone of this repo, at GitHub's main (this repo's `origin/main`, fetched first), in `~/.local/share/handline/app-main` (`HANDLINE_FOLLOW_DIR` moves it), installs, builds the page, the daemon and `hl` and starts the app from there, so no working tree is touched. Link `~/.local/bin/hl` to that clone's `bin/hl`, so the `hl` on the path is the build of the running daemon (`hl version` names both). Start it the same way every time; it brings the clone up to main first.

The app follows GitHub's main: it watches `origin/main` of the repo it was cloned from (any fetch there moves it; the repo's own `main` branch is not used, PRs merge on GitHub) and fetches it when the window comes to the front, at most once a minute. When main moves, a notice in the page names the new commit: **Restart now** brings the clone up to that main, rebuilds only what changed (the page; the app itself and a reinstall when `apps/electron`, the client packages or the lockfile changed; the daemon and `hl` every time) and relaunches on the same page, with the device token kept; **Later** asks again at the next move. The tray shows the commit the app is on and offers _Restart to update_ or _Check for update_.

Until the person chooses addresses (`network_addresses`, set from Connect a device), the daemon listens on the Mac's Tailscale address when Tailscale is up (`HANDLINE_LISTEN` overrides) and advertises its `*.ts.net` name; without it, loopback only. Once addresses are chosen the app starts the daemon on loopback and the daemon opens a door on each chosen address, so the window keeps a plain loopback door. The app always passes `-web`: the daemon serves the page on every address it is reachable on, so a browser on the same network can open an invite. The window reaches the daemon at the same address, which is chosen when the app starts, and the shell tells the page that one address is its own machine's, so plain ws to it is allowed (no other tailnet address is). If the daemon does not answer there and the app started it, the app restarts it on loopback (agents keep running in their terminals); a daemon the app did not start is left alone and the tray says it does not answer.

An agent's run stays in the background: it has no dock or menu bar icon, no update dialog, never takes focus, keeps its window hidden, shows no native notices and does not bring the everyday app forward. An agent is known by `HANDLINE_AGENT` in its shell, and `pnpm dev <team>` passes the choice on. `HANDLINE_BACKGROUND=1` asks for it and `HANDLINE_BACKGROUND=0` turns it off. The hidden page keeps drawing, so screenshots go through the DevTools port (`Page.captureScreenshot`), never a screen capture.

`HANDLINE_DEBUG_PORT=9333` opens the DevTools port on localhost for run checks. Any local process can then read the page, its device token included, so use it for checks only, never for the app you use every day.

## Trying a branch next to the installed app

    tools/try-prototype.sh [branch]

Runs a branch (default `story/prototype-apply`) as a second app on the live daemon with the same data. It keeps its own clone (`~/.local/share/handline/try-prototype`), its own app data (`~/.local/share/handline/try-prototype-data`: settings, device token, instance lock) and its own page port (`HANDLINE_WEB_PORT`, default 17445), so the installed app is not touched and quitting this one leaves the daemon running. Run it again to bring the clone to the branch's head.
