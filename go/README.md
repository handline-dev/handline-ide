# The Go daemon (`handlined`)

One binary, built from `go/`, the daemon's module.
It requires seven modules of its own beside it, each tested alone: `go/agentctl` (drives the agent CLIs), `go/scheduler` (session
lifecycle and the resource-aware start), `go/taskboard` (the task board's core), `go/accounts` (usage, limits and logins), `go/data`
(the store and team config), `go/protocol` (the wire, authority and pairing) and `go/kit` (shared utilities). `go/go.work` lists all eight. Run it with `-home DIR`; on the default home it refuses to start unless `-allow-real-home` says so.
`tools/test-go.sh` runs `go test` (with `-race` when `HANDLINE_GO_RACE=1`, as the auditor's run sets it), a cgo-free build and the contract fixtures against the built binary.

## The `hl` command (`go/cmd/hl`)

`tools/build-hl.sh` builds `go/hl`; `bin/hl` then runs it, and it is the `hl` command.

## Network listener

`-listen ADDR:PORT` opens a WebSocket (`/rpc`) for paired devices, plain on Tailscale or loopback without `-cert` and `-key` (Tailscale mode), TLS with them; nothing listens without it. `ADDR` must be an IP of this
machine's loopback, LAN or Tailscale network, never a wildcard, a public address or a host name. A connection no device token opened gets
one refusal for every call (`gate: unpaired`) and nothing else. `device.pair` (founder) makes a one-time code; a device trades it
with `device.redeem` for a token, which is kept as a hash in the `devices` table and sent in `hello`.

## Streaming topics

- **The devices topic** is live, in memory: `device.redeem` and `device.revoke` send `device.paired` and `device.revoked` to the
  founder connections subscribed to `devices`, with cursor 0; a client reads `devices.list` again after a reconnect. A list naming
  `devices` must name nothing else (`bad_request`, field `topics`: devices and transcript are live from memory, the rest come from the log),
  and a device naming `devices` alone is refused (`gate: role`).
- **The other topics** are rows of `stream_log`, whose id is the cursor, and `subscribe` and `unsubscribe` serve them. Rows come from this
  daemon's `Hub.Append`. One poll of
  `MAX(id)` every 0.25 s (or at once after an Append) hands the new rows to the subscribers in cursor order; there are no change detectors
  here, so a topic's rows still come from Python until its detector moves. A resume replays up to 5000 events and then sends `gap`
  (`retention`, which is also a cursor older than `stream:pruned`); a subscriber more than 1000 live events behind gets `gap` (`slow_client`)
  and is hung up. An agent session is sent what its role may read (`stream.AgentMay`). Retention is 7 days or 50000 rows, pruned hourly.
  The detectors that turn database changes into rows are `internal/watcher`: one per topic, each moved here from the Python watcher, and started
  only with `-terminal-tick`. Moved so far: tasks, agents (with the stall notification), notifications, inbox, chat, proposals (whose change check also looks at the proposal files).

## Web UI and TLS

`-web DIR` serves the built web UI on the listener's own origin (`http://` in Tailscale mode, `https://` with a certificate), so a browser needs no pinning. Files only: no
listing, nothing outside `DIR` (including through a symlink), no dotfiles, `index.html` never cached, `GET` and `HEAD` only, a `token` in the
query refused. The page is the app and its pairing screen, not data; every call it makes goes through the device gate, with the token sent
in the first `hello` message. A browser's WebSocket is accepted only from the page this listener served.

### Tailscale mode (the default): no certificate

A listener given no `-cert` and `-key` serves plain HTTP and WebSocket, because a tailnet is already encrypted end to end by WireGuard.
There is nothing to install and a browser shows no warning.

- `-listen :PORT` binds the address of the machine's Tailscale interface (`utun*` on macOS, `tailscale*` elsewhere; an address in 100.64.0.0/10 on any other interface does not count, since some networks hand that range out on the LAN) only. If there is none, the daemon refuses to start; it
  never falls back to `0.0.0.0` or to another interface. `-listen 100.x.y.z:PORT` names the address yourself, and it must be that interface's.
- Plain HTTP is served only on a Tailscale interface address or on loopback (for the proxy below). Any other address, a LAN address for example, is refused
  at start (`-cert` and `-key` are needed there). There is no way to serve plain HTTP on the LAN.
- The pairing code carries the address and a one-time secret, and no fingerprint (there is no certificate). Pairing secrets and tokens are
  never logged. A caller no device token has opened still gets no data.
- A browser's WebSocket is accepted only from the page the listener served (`http://HOST:PORT`).

**Secure context (installable web app, service worker).** A browser treats plain `http://` on a tailnet address as not secure, so a web
app cannot be installed from it. For that, run once, with the listener on loopback (`-listen 127.0.0.1:7443`):

    tailscale serve --bg --https=443 http://127.0.0.1:7443

Tailscale then serves `https://HOST.TAILNET.ts.net/` with its own certificate and proxies to the daemon; use `-advertise HOST.TAILNET.ts.net:443`.
I could not verify this command's syntax offline; check it against the current Tailscale documentation (`tailscale serve --help`) before relying on it.
Skip it if you do not want the app installed.

### TLS (any other interface, or native clients that pin)

With `-cert` and `-key` the listener serves TLS 1.3 on any LAN, Tailscale or loopback address, never plain. The certificate is read from those
files and read again whenever either file changes, so a renewal needs no restart. Nothing else is read or created, and neither file is logged.
Both files are given together or not at all.

- **Self-signed, pinned.** Any self-signed certificate and key (for example from
  `openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 3650 ...`). The pairing code carries its sha256 fingerprint and
  a native client refuses any other certificate. A browser warns once.
- **CA-trusted.** `tailscale cert HOST.TAILNET.ts.net` writes `HOST.TAILNET.ts.net.crt` and `.key`; give those to `-cert`/`-key` and
  `-advertise HOST.TAILNET.ts.net:PORT`. Browsers accept it without a warning.

`device.pair` returns `{code, expires_at, fingerprint, uri}`; `uri` is `handline://pair?host=ADDRESS&fp=FINGERPRINT&code=CODE`, the address
being `-advertise` or the listening address; `fp` is absent in Tailscale mode.
