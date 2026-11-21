<!-- Generated from contract/spec/ by contract/generate.py; do not edit. -->
# devices methods (protocol 1)

A method marked write takes a required `request_id`. Framing, roles, errors and gates are in [protocol.md](../protocol.md); field types in `protocol.ts`.

| Method | Role | Write | Params | Result | Errors | Gates | App |
|---|---|---|---|---|---|---|---|
| `devices.list` | owner |  | none | `{devices, push_platforms}` |  |  | `devices`, fed by devices |
| `device.pair` | owner | yes | `name?`, `ttl_s?`, `address?`, `request_id` | `Accepted & {code, expires_at, fingerprint, uri, web_link}` | `refused` |  | decided: pair |
| `device.redeem` | anonymous | yes | `code`, `name`, `kind?`, `node?`, `request_id` | `Accepted & {token, device}` | `refused` | pairing_code, attempt_limit, already_joined | not from the app |
| `device.revoke` | owner | yes | `device`, `request_id` | `Accepted` | `refused`, `not_found` |  | decided: revoke |
| `device.set` | owner | yes | `device`, `lending`, `request_id` | `Accepted` | `refused`, `not_found` | person, kind | decided: set |
| `device.push_token` | owner | yes | `platform`, `token`, `sandbox?`, `request_id` | `Accepted` | `refused`, `not_found` | device | not from the app |
| `node.status` | agent |  | none | `{master, nodes}` |  |  | `nodes`, fed by nodes |
| `node.forward` | owner |  | `agent`, `method`, `params?`, `client?`, `cwd?` | `object` | `refused` | node, not_on_node, method | not cached |

### `devices.list`

Paired devices with last seen and whether each is connected.

### `device.pair`

Make a one-time pairing code. Founder only. The code is single use and expires: `ttl_s` seconds (default 300, at most 600). The reply carries the listener certificate fingerprint and the address in `uri`, which is what the QR holds; a client pins that fingerprint and refuses any other certificate. It is not replayed by request_id: a retry with the same request_id runs again and makes another code.

### `device.redeem`

Trade a pairing code for a device token. The only method an unpaired network caller may call. A code that is wrong, expired or already used is refused, gate `pairing_code`, without saying which; too many wrong tries are refused, gate `attempt_limit`. A code redeems once: the token is in this one reply and never again, and the daemon keeps only its hash. `name` is how the device is listed. It is not replayed by request_id either: a retry runs again, and a code that was redeemed stays used, so the retry is refused. The wrong-code count for a redeem over the local socket is kept under the host '' (the empty string), since it has no address. A mac or a server whose node id this master already has is refused, gate `already_joined`. The device is the person's who made the code (device.pair).

### `device.revoke`

Cancel a device and close its connections at once. Cancels the device's token. Its open connections and terminal attaches are closed at once, its next call is refused (gate `unpaired`), and a device.revoked event is sent on the devices topic. Revoking a device that is not paired is not_found.

### `device.set`

Set a lent machine's Run · Pause · Off. Only the device's own person sets it; anyone else is refused, gate `person`. A device that is no mac or server that joined is refused, gate `kind`. run lists the machine in node.status and lets the master start work there; pause lists it as paused and the master starts nothing new there; off takes it out of node.status. A device.changed event is sent on the devices topic, and the nodes feed changes.

### `device.push_token`

A paired device gives the push token its OS gave it. Called by the paired device itself, after pairing and whenever its OS gives it a new token; the token is kept on that device's row in place of the one before. A connection that is no paired device (the local socket) is refused, gate `device`. The token is dropped when the device is revoked. devices.list shows whether a device has one (`push`).

### `node.status`

The machines of this master, its own first. Read only. The master's own machine and the machines lent to it on run or pause (device.set).

### `node.forward`

A runner's call for one of its agents, run on the master as that agent. Only on a runner's link: the connection a joined machine's daemon keeps open to the master with its device token; any other connection is refused, gate `node`. The master runs it only for an agent whose session it opened on that runner (gate `not_on_node`), never hello, node.forward, subscribe, unsubscribe or a runner.* call (gate `method`). The forwarded call's result or error comes back unchanged.

