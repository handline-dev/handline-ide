<!-- Generated from contract/spec/ by contract/generate.py; do not edit. -->
# devices methods (protocol 1)

A method marked write takes a required `request_id`. Framing, roles, errors and gates are in [protocol.md](../protocol.md); field types in `protocol.ts`.

| Method | Role | Write | Params | Result | Errors | Gates | App |
|---|---|---|---|---|---|---|---|
| `devices.list` | member |  | none | `{devices, push_platforms}` |  |  | `devices`, fed by devices |
| `device.pair` | member | yes | `name?`, `ttl_s?`, `address?`, `person?`, `request_id` | `Accepted & {code, expires_at, fingerprint, uri, web_link}` | `refused` |  | decided: pair |
| `device.redeem` | anonymous | yes | `code`, `name`, `kind?`, `node?`, `request_id` | `Accepted & {token, device}` | `refused` | pairing_code, attempt_limit, already_joined | not from the app |
| `device.revoke` | member | yes | `device`, `request_id` | `Accepted` | `refused`, `not_found` |  | decided: revoke |
| `device.set` | member | yes | `device`, `lending`, `request_id` | `Accepted` | `refused`, `not_found` | person, kind | decided: set |
| `device.push_token` | member | yes | `platform`, `token`, `sandbox?`, `request_id` | `Accepted` | `refused`, `not_found` | device | not from the app |
| `node.status` | agent |  | none | `{master, nodes}` |  |  | `nodes`, fed by nodes |
| `node.forward` | member |  | `agent`, `method`, `params?`, `client?`, `cwd?` | `object` | `refused` | node, not_on_node, method | not cached |
| `node.turn` | member |  | `agent`, `view` | `object` | `refused`, `bad_request` | node, not_on_node | not cached |
| `node.transcript` | member |  | `agent`, `session`, `at`, `rows` | `object` | `refused`, `bad_request` | node, not_on_node | not cached |
| `node.heartbeat` | member |  | `version`, `boot?`, `load?`, `sessions?`, `accounts?`, `has?`, `battery?` | `{stop}` | `refused`, `bad_request` | node | not cached |
| `node.tokens` | member |  | `account`, `calls` | `object` | `refused`, `bad_request` | node | not cached |
| `node.launch` | owner |  | `agent`, `params?` | `{argv, env, warnings, files, agent_dir}` | `refused` | node, not_on_node, prompt_file, launch, state | not cached |
| `node.agent_files` | owner |  | `agent`, `files?`, `deleted?` | `Accepted` | `refused`, `bad_request` | node, not_on_node, path | not cached |

### `devices.list`

Paired devices with last seen and whether each is connected. The owner sees every person's devices, a member only their own.

### `device.pair`

Make a one-time pairing code. Founder only. The code is single use and expires: `ttl_s` seconds (default 300, at most 600). The reply carries the listener certificate fingerprint and the address in `uri`, which is what the QR holds; a client pins that fingerprint and refuses any other certificate. It is not replayed by request_id: a retry with the same request_id runs again and makes another code.

### `device.redeem`

Trade a pairing code for a device token. The only method an unpaired network caller may call. A code that is wrong, expired or already used is refused, gate `pairing_code`, without saying which; too many wrong tries are refused, gate `attempt_limit`. A code redeems once: the token is in this one reply and never again, and the daemon keeps only its hash. `name` is how the device is listed. It is not replayed by request_id either: a retry runs again, and a code that was redeemed stays used, so the retry is refused. The wrong-code count for a redeem over the local socket is kept under the host '' (the empty string), since it has no address. A mac or a server whose node id this master already has is refused, gate `already_joined`. The device is the person's who made the code (device.pair).

### `device.revoke`

Cancel a device and close its connections at once. Cancels the device's token. Its open connections and terminal attaches are closed at once, its next call is refused (gate `unpaired`), and a device.revoked event is sent on the devices topic. Revoking a device that is not paired is not_found. A member revokes only their own devices (gate `person`).

### `device.set`

Set a lent machine's Run · Pause · Off. Only the device's own person sets it; anyone else is refused, gate `person`. A device that is no mac or server that joined is refused, gate `kind`. run lists the machine in node.status and lets the master start work there; pause lists it as paused and the master starts nothing new there; off takes it out of node.status. A device.changed event is sent on the devices topic, and the nodes feed changes.

### `device.push_token`

A paired device gives the push token its OS gave it. Called by the paired device itself, after pairing and whenever its OS gives it a new token; the token is kept on that device's row in place of the one before. A connection that is no paired device (the local socket) is refused, gate `device`. The token is dropped when the device is revoked. devices.list shows whether a device has one (`push`).

### `node.status`

The machines of this master, its own first. Read only. The master's own machine and the machines lent to it on run or pause (device.set).

### `node.forward`

A runner's call for one of its agents, run on the master as that agent. Only on a runner's link (gate `node`), for an agent whose session the master opened there (gate `not_on_node`); never hello, a node.* or runner.* call, subscribe or unsubscribe (gate `method`). The answer comes back with its paths under the master's home made the runner's. A runner lent by a member calls it with that member's device token.

### `node.turn`

A runner's tracker tells the master one of its agents' turns. Only on a runner's link (gate `node`), for an agent whose session is on that runner (gate `not_on_node`). Sent on each change of the turn and for every session when the link starts. The master's readers (the agent's view, an idle wait) see that agent as one of its own; when its session there stops or the link ends, its turn goes back to unknown.

### `node.transcript`

A runner sends the chat rows one line of an agent's session transcript holds. Only on a runner's link (gate `node`), for an agent whose session is on that runner (gate `not_on_node`). The master keeps the rows when chat_transcript_rows is on and drops them otherwise. Lines read while the master is away are not sent again.

### `node.heartbeat`

A runner says it is there: its version, load, sessions, accounts and what it has. Only on a runner's link (gate `node`). The runner sends one when its link comes up, then 15 s after each answer, so one is in flight at a time. The master keeps the newest in memory and the time as the node row's last_seen; a runner unheard for 60 s is offline, and after the reclaim time offline (node_reclaim_s, 30 minutes) its sessions are taken back. With the battery switch on (node_battery_switch), one that says it went on battery is set to Off from Run, and back to Run when it is plugged in.

### `node.tokens`

A runner sends the calls one of its accounts' transcripts gained. Only on a runner's link (gate `node`). The master counts the calls in its usage under that account, each charged to the agent whose session made it; a call sent again counts once (by account, session and request). The runner moves its read positions only once the master has answered. Any linked runner may send for any account name: the runner is the person's own lent machine, so this is not checked against the node.

### `node.launch`

hl launch on a runner: the launch built on the master for that machine's home, and the files it starts with. Only on a runner's link (gate `node`), for an agent whose session the master opened there (gate `not_on_node`). Paths under the master's home are answered as the runner's. The only prompt file taken is the one the master wrote for the agent (gate `prompt_file`).

### `node.agent_files`

A runner's agent's memory and handoff note, as its session there wrote them. Only on a runner's link (gate `node`), for an agent whose session the master opened there (gate `not_on_node`); only memory/** and handoff.md inside the agent's folder (gate `path`).

