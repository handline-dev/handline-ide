# The Go daemon (`handlined`)

One binary, built from `go/`, the daemon's module. It requires seven modules of its own beside it, each tested alone: `go/agentctl`
(drives the agent CLIs), `go/scheduler` (session lifecycle and the resource-aware start), `go/taskboard` (the task board's core),
`go/accounts` (usage, limits and logins), `go/data` (the store and team config), `go/protocol` (the wire, authority and pairing) and
`go/kit` (shared utilities). `go/go.work` lists all eight. Run it with `-home DIR`; with no `-home` it refuses to start unless
`-allow-real-home` says so. `handlined -h` lists every flag.

`tools/test-go.sh` runs `go test` over every module (with `-race` when `HANDLINE_GO_RACE=1`); `--gates` runs the checks that are not a
package's test (go.mod tidy, duplication, a cgo-free build, the contract fixtures against the built binary); `--fakes` runs the
`-tags fakes` build's tests and fixtures.

## The `hl` command (`go/cmd/hl`)

`tools/build-hl.sh` builds `go/hl`; `bin/hl` runs it, and it is the `hl` command.

## Streaming topics

- **The devices topic** is live, in memory: `device.redeem` and `device.revoke` send `device.paired` and `device.revoked` to the
  founder connections subscribed to `devices`, with cursor 0; a client reads `devices.list` again after a reconnect. A list naming
  `devices` must name nothing else (`bad_request`, field `topics`), and a device naming `devices` alone is refused (`gate: role`).
- **The other topics** are rows of `stream_log` (`go/data/stream`), whose id is the cursor; `subscribe` and `unsubscribe` serve them.
  One poll of `MAX(id)` every 0.25 s (or at once after an append) hands the new rows to the subscribers in cursor order. A resume
  replays up to 5000 events, else it gets `gap` (`retention`, also for a cursor older than `stream:pruned`); a subscriber more than 1000
  live events behind gets `gap` (`slow_client`) and is hung up. An agent session is sent what its role may read (`stream.AgentMay`).
  Rows are kept 7 days or 50000 rows, pruned hourly.
- The detectors that turn database changes into rows are `internal/watcher`, one per topic. A daemon that holds a board starts them
  unless `-terminal-tick=false`.

## Network listener

`-listen ADDR:PORT` opens the network listener; nothing listens without it. It serves JSON-RPC over a WebSocket at `/rpc` for paired
devices. `ADDR` must be an IP of this machine's loopback, LAN or VPN (100.64.0.0/10) network, never a wildcard, a public address or a
host name. A connection no device token opened gets one refusal for every call (`gate: unpaired`) and nothing else. `device.pair`
(founder) makes a one-time code; a device trades it with `device.redeem` for a token, kept as a hash in the `devices` table and sent
in `hello`.

The listener is a set of doors, one per IP served, all on the same port, plus a plain loopback door for the desktop app:

- While the `network_addresses` setting is unset, the doors are the `-listen` one and Handline's pick for pairing. Once set (a setting
  the app writes from the hosts `connection.addresses` lists), it chooses the hosts served. The doors follow the setting and the machine's interfaces
  (checked every 5 s) without a restart.
- **Tailscale** addresses (an address in 100.64.0.0/10 on `utun*` on macOS or `tailscale*` elsewhere; the range on any other
  interface does not count) are served plain HTTP, since the tailnet already encrypts. With no `-cert`/`-key`, `-listen :PORT` binds
  that interface's address, and the daemon refuses to start if there is none. When the tailnet has HTTPS on, the same door also
  answers TLS with Tailscale's own certificate for the MagicDNS name (`tailscale cert`, written to `<home>/network` and fetched again a
  month before it expires), so a browser trusts it and the web app can be installed.
- **LAN** addresses are served TLS 1.3 only, with a self-signed certificate made once in `<home>/network/lan.crt` and kept, so its
  fingerprint, which a paired client pins, stays the same.
- `-cert` and `-key` (together or not at all) replace the daemon's own certificate on every TLS door; the files are read again when
  either changes, so a renewal needs no restart. Neither file is logged.
- `-advertise HOST:PORT` is the address a pairing names when it differs from `-listen` (a host name, or a proxy in front).

`device.pair` returns `{code, expires_at, fingerprint, web_link}`; `web_link` is `http(s)://HOST:PORT/#pair=CODE`, with
`&fp=FINGERPRINT` on a self-made certificate. The host is Handline's pick unless a door is named: the Tailscale name, else the
Tailscale IP, else the first Wi-Fi address. Pairing secrets and tokens are never logged.

## Web UI

`-web DIR` serves the built web UI on each door's own origin, so a browser needs no pinning: on loopback and Tailscale doors, never on a
Wi-Fi door with a self-made certificate (a browser's warning there cannot tell a fake server from this machine). Files only: no listing,
nothing outside `DIR` (a symlink included), no dotfiles, `index.html` never cached. The page is the app and its pairing screen, not
data: every call it makes goes through the device gate, with the token sent in the first `hello`. A browser's WebSocket is accepted
only from the page its door served.
