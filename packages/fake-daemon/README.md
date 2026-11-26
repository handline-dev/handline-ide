# fake-daemon

A daemon that speaks the contract with scripted agents, for UI work and tests without a real daemon.
`node src/serve.ts` serves it on a Unix socket and a WebSocket; `HANDLINE_FAKE=1 vite` in `apps/web` runs it beside the dev server.

## Pairing a phone dev build

`node src/serve.ts --phone --ws 7791 --socket /tmp/x.sock` serves the WebSocket as the paired-device listener and prints a one-time `handline://pair?...&plain=1` link.
A dev build of the phone app takes it, and a release build ignores it. iOS simulator: `xcrun simctl launch <udid> dev.handline.mobile -handlineDevPair '<link>'`
(`simctl openurl` makes iOS ask "Open in Handline?", which needs a tap). Android emulator: `adb reverse tcp:7791 tcp:7791`, then `adb shell am start -a android.intent.action.VIEW -d '<link>'`.
The pairing is kept, so later launches need no link. Run Metro without `CI=1`, or it never rebuilds the bundle. Keep the socket path short (a Unix socket path over about 100 characters fails).

## Seeds

The first-load fixtures leave out some states a UI check needs. Switch them on by name; `all` switches on every one except
those in `BY_NAME`, which differ from the rest (another cluster, a Mac without git) and come only by name. A name that does
not exist is an error. The names and what each adds are the commented entries of `SEEDS` in `src/seeds.ts` and the
`src/seeds-*.ts` files it joins.

Switch on:

- **Vite:** `HANDLINE_FAKE=1 HANDLINE_FAKE_VERBS=1 HANDLINE_FAKE_SEEDS=proposals,tasks-200 vite` (the actions call task verbs, so add `HANDLINE_FAKE_VERBS=1`).
- **serve.ts:** `node src/serve.ts --seed all`, or `HANDLINE_FAKE_SEEDS=all`.
- **A test:** `applySeeds(daemon, 'chat-actions')` from `src/seeds.ts`, before the client connects.
