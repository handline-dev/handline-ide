# Run Handline on the real daemon

The desktop app, a browser and the phone app, all against the daemon that runs your agents. Commands run from the repo
root. `hl` is the command from [Setup](setup.md).

The daemon's network listener on this Mac's Tailscale address serves all of them at once. A tailnet is already
encrypted, so the apps speak plain `ws://` there; a browser gets Tailscale's own certificate for the Mac's name once the
tailnet has HTTPS. Tailscale must be on for this Mac and for each other device.

## 1. Start the daemon

**With the desktop app** (the usual way). It starts the daemon itself, which listens on this Mac's Tailscale address and
pairs the app.

    pnpm install
    pnpm app                  # the app runs from its own clone of main and offers a restart when main moves

Tailscale must be up when the app starts. Without it the daemon listens on loopback only and no other device can pair.
Quitting the app leaves the daemon and its agents running.

**Without the desktop app** (a browser and phones only). Find this Mac's Tailscale address `<ip>` (a `100.x.y.z`) and
its name `<name>` (`<mac>.<tailnet>.ts.net`, no trailing dot):

    tailscale ip -4
    tailscale status --json | python3 -c 'import json,sys; print(json.load(sys.stdin)["Self"]["DNSName"].rstrip("."))'

A pairing link must name the `*.ts.net` name, never a bare `100.x` address, so start the daemon with
`-advertise <name>:7443`. In a terminal you keep open, with no other daemon on this home:

    pnpm install && tools/build-go.sh && pnpm --filter @handline/web build
    go/handlined -allow-real-home -claude "$(command -v claude)" -listen <ip>:7443 -advertise <name>:7443 -web apps/web/dist

You should see a `network listener on <ip>:7443, ...` line in the daemon's terminal (with the app, in `daemon.log` in
its data folder).

**Common failure:** the daemon exits with `another daemon holds this home`: one already runs (the app's). Stop it
first, or use that one.

## 2. Pair a device

In the desktop app, Settings > Devices > **Connect a device** shows a one-time link, with a QR and a code. Open the link
on the other device (a browser, or the phone app's scanner). Without the app, `hl device pair` prints the same `code:`
and `link:`. A code works once and expires in a few minutes. Listing and revoking devices are in Settings > Devices.

- **Browser:** open the link, or `http://<name>:7443` and paste the code. The agent list appears.
- **Phone:** the phone app, with Tailscale on and signed in to the same tailnet, scans the QR or takes the link.

**Common failures:**

- "too many wrong pairing codes": this address is locked for a while after repeated wrong codes. Wait, then get a fresh
  code and paste it whole.
- `hl device pair` says `the network listener is off`: the daemon runs without `-listen` (step 1).
- The phone cannot connect: it cannot reach `<name>:7443` (Tailscale is off on the phone), or the link carries a bare
  `100.x` address (the daemon was started without `-advertise`). A browser on the phone at `http://<name>:7443` tells
  which: if the page does not load, the network is the cause.

## Not on Tailscale: a Wi-Fi address with a certificate

Off the tailnet the daemon serves TLS on its own certificate, and the apps pin it; a browser gets no link there, since its
warning for a self-made certificate cannot tell a fake server from this Mac.

Simplest: set `network_addresses` to the Wi-Fi address from `connection.addresses` (`settings.set`); the daemon makes its
own certificate and serves it beside the Tailscale door, with no restart. Or make a certificate once (`<lan-ip>` is this
Mac's Wi-Fi address) and start the daemon with it:

    openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 3650 -subj "/CN=handline" \
      -addext "subjectAltName=IP:<lan-ip>" -keyout ~/handline.key -out ~/handline.crt
    go/handlined -allow-real-home -claude "$(command -v claude)" -listen <lan-ip>:7443 -cert ~/handline.crt -key ~/handline.key -web apps/web/dist

The pairing link then carries the certificate's fingerprint (`&fp=`), which the app checks; a link from a daemon with a
different certificate is refused, so pair again.
