# Run Handline on the real daemon

Steps for the founder: the desktop app, a browser, an iPhone and an Android phone, all against the daemon that runs your agents.
Commands run from the repo root. `hl` is the command from [Setup](setup.md).

The daemon's plain network listener on this Mac's Tailscale address serves all of them at once: the desktop app, a browser, and the phones (a tailnet is
already encrypted, so there is no certificate to pin). Tailscale must be on for this Mac and for each phone.

Known limits of this MVP: the app cannot create a task (founder decision), and every paired device acts as you.

## 1. Start the daemon

**With the desktop app** (the usual way). It starts the daemon itself, which listens on this Mac's Tailscale address, advertises its `*.ts.net` name, and pairs itself.

    pnpm install && tools/build-go.sh
    pnpm app                  # the app runs from its own clone of main and offers a restart when main moves

Tailscale must be up when the app starts. Without it the daemon listens on loopback only and no phone can pair.

**Without the desktop app** (a browser and phones only). Find this Mac's Tailscale address `<ip>` (a `100.x.y.z`) and its name `<name>` (`<mac>.<tailnet>.ts.net`, no trailing dot):

    tailscale ip -4
    tailscale status --json | python3 -c 'import json,sys; print(json.load(sys.stdin)["Self"]["DNSName"].rstrip("."))'

The phones accept a plain link only for a `*.ts.net` name. A link with a bare `100.x` address is refused, so start the daemon with `-advertise <name>:7443`. In a terminal you keep open, with no other daemon on this home:

    pnpm install && tools/build-go.sh && pnpm --filter @handline/web build
    go/handlined -allow-real-home -claude "$(command -v claude)" -terminal-tick -listen <ip>:7443 -advertise <name>:7443 -web apps/web/dist

You should see `network listener on <ip>:7443, plain HTTP (Tailscale)`, in the daemon's terminal (with the app, in `daemon.log` in its data folder).

**Common failure:** the daemon exits with `another daemon holds this home`: one already runs (the app's). Stop it first, or use that one.

## 2. PC

### Desktop app

Step 1 opened it. It pairs itself, so you should see the agent list at once. `hl device list` shows this computer and `hl device revoke <id>` cancels it.

**Common failure:** the window shows "Pair this computer": the app could not pair itself. Run `hl device pair` and paste the `code:` value, or the whole `handline://` line, into the window.
The app stops a daemon already running on the home and starts its own, which exits with the app; agents keep running in tmux. If the window fails to connect, the reason is in `daemon.log` in the app's data folder.
If `hl device pair` says `the network listener is off`, the daemon runs without `-listen`: step 1.

### Browser

Open `http://<ip>:7443` (the daemon you started without the app serves the page; with the app, use its window).
Run `hl device pair` and paste the code into the page. The agent list appears.

**Common failure:** "too many wrong pairing codes": this address is locked for 15 minutes after repeated wrong codes. Wait, then run `hl device pair` for a fresh code and paste it whole.

## 3. iPhone

1. Tailscale on the phone, signed in to the same tailnet.
2. Build the app onto the phone. It is a dev client, not Expo Go. Plug the iPhone in; the first build needs your Apple team set in Xcode. Build Release, so the JS is inside the app:

       cd apps/mobile && npx expo run:ios --device --configuration Release

   The QR scanner needs the camera module (`expo-camera`), so an app built before QR pairing must be rebuilt; paste pairing works without it. A Debug build (the default) loads its JS from Metro on the Mac (`npx expo start --dev-client`, port 8081): it shows nothing once that terminal is closed or if the phone cannot reach the Mac.
3. Pair. With the desktop app, open Settings > Devices and choose **Connect a device**: it shows a one-time QR and a web link. In the phone app tap **Scan the code**, allow the camera, and point it at the QR. The code works once and expires in a few minutes; close the dialog when the phone is connected.
   Without the app, or if the camera cannot read it: on the Mac run `hl device pair`, send the `uri:` line to the phone (Messages, Notes) and paste the whole `handline://pair?host=<name>:7443&code=...` into the app's field, then Pair.

You should see the agent list, then an agent's page.

**Common failure:** `cannot connect to ...`: the phone cannot reach `<name>:7443` (Tailscale is off on the phone). With the daemon started without the app, the phone's browser at `http://<name>:7443` loads the page; if it does not, the network is the cause. (`<name>` is the host in the `uri:` line.)
If the app says the link has no certificate fingerprint and the address is not a Tailscale name, the daemon was started without `-advertise <name>:7443` (step 1): stop it and start it again.

`apps/mobile/README.md` documents the simulator only; `--device` builds onto a plugged-in phone. This command is not tested.

## 4. Android

1. Tailscale on the phone, signed in to the same tailnet.
2. Build the release app once (about 9 minutes) and install it on the phone with USB debugging on. `apps/mobile/android` is not in the repo, so generate it first.

       cd apps/mobile && npx expo prebuild --platform android
       cd android && ./gradlew assembleRelease -PreactNativeArchitectures=arm64-v8a
       adb install -r app/build/outputs/apk/release/app-release.apk

   An app built before QR pairing lacks the camera module (`expo-camera`): prebuild and build again. Skip `adb reverse`: the phone reaches `<name>` itself. Open the app from the launcher.
3. Pair the same way as on the iPhone: **Connect a device** in Settings > Devices, then **Scan the code** in the phone app; or `hl device pair` and paste the `uri:` line, then Pair.

You should see the agent list.

**Common failure:** the same as the iPhone's: the phone cannot reach `<name>:7443`, or the link carries a bare `100.x` address (the daemon was started without `-advertise <name>:7443`).

## Not on Tailscale: a LAN with a certificate

Without Tailscale the daemon must serve TLS and the phone pins its certificate. Such a daemon cannot serve the desktop app (it speaks plain `ws://` only); use a browser on the Mac.
Simplest: set `network_addresses` to the Wi-Fi address from `connection.addresses` (settings.set); the daemon makes its own certificate and serves it beside the Tailscale door, with no restart.
Or make a certificate once (`<lan-ip>` is this Mac's Wi-Fi address, which the phone reaches on the same network), then start the daemon with it:

    openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 3650 -subj "/CN=handline" \
      -addext "subjectAltName=IP:<lan-ip>" -keyout ~/handline.key -out ~/handline.crt
    go/handlined -allow-real-home -claude "$(command -v claude)" -terminal-tick -listen <lan-ip>:7443 -cert ~/handline.crt -key ~/handline.key -web apps/web/dist

`hl device pair` now prints a link with a fingerprint, which the phone checks. A browser at `https://<lan-ip>:7443` warns about the certificate once.
"Certificate does not match" in the app means the link came from a daemon with a different certificate: run `hl device pair` again.
