<!-- Generated from contract/spec/ by contract/generate.py; do not edit. -->
# screen methods (protocol 1)

A method marked write takes a required `request_id`. Framing, roles, errors and gates are in [protocol.md](../protocol.md); field types in `protocol.ts`.

| Method | Role | Write | Params | Result | Errors | Gates | App |
|---|---|---|---|---|---|---|---|
| `screen.attach` | member |  | `agent`, `source` | `ScreenSnapshot` | `not_found`, `bad_request`, `refused` | state | not cached |
| `screen.detach` | member |  | `agent`, `source` | `object` | `bad_request` |  | not cached |
| `screen.take` | owner | yes | `agent`, `source`, `force?`, `request_id` | `Accepted & {holder}` | `refused`, `bad_request` | role, input_holder, state | decided: take over |
| `screen.give_back` | owner | yes | `agent`, `source`, `request_id` | `Accepted & {holder}` | `refused`, `bad_request` | role, input_holder | decided: return control |
| `screen.input` | owner | yes | `agent`, `source`, `event`, `request_id` | `Accepted` | `refused`, `bad_request` | role, input_holder, state | uncached: key |
| `screen.act` | agent |  | `source`, `kind`, `box?` | `Accepted` | `refused`, `bad_request` | role, input_holder | not cached |
| `screen.status` | agent |  | `source` | `{holder, watchers, machine}` | `refused`, `bad_request` | role | not cached |

### `screen.attach`

Watch one of an agent's screen sources: replies with its current frame, then pushes screen_frame, screen_action, screen_holder and screen_ended notices. A source runs on the agent's own machine (a runner's agent's through the master, as its terminal is) and sends only while at least one connection watches it: the first attach starts it, the last detach or hang-up stops it. The reply waits up to 3 s for the source's first frame; `frame` is null when none came. A source sends a frame only when what it shows changes, and the next only once the daemon has passed the last one on, so a slow watcher slows the source and nothing queues. A connection has one attach per agent and source: attaching again answers afresh. An agent with no such source open is not_found (what: source). A source that could not be started is refused, gate state.

### `screen.detach`

Stop watching a source. Nothing to end is not an error. The holder's last watch ending gives the source back (screen.give_back).

### `screen.take`

Become the one who drives the source; while you hold it the agent's own use of it is refused. Only a connection that watches the source takes it (else refused, gate state). Taking one you hold already is accepted. The hold is kept in the daemon's memory: a daemon that starts again holds nothing. It ends with screen.give_back, or when its holder no longer watches the source, or when the source goes away.

### `screen.give_back`

Return the source to the agent. From the holder only; with no holder it is accepted and changes nothing. Before the agent may use the source again the daemon empties each password field the holder typed into and did not submit, since the agent can read the page. An agent whose use was refused while it was held is told one line: `<holder> gave the page back`.

### `screen.input`

A mouse, wheel, key or text event into the source, from its holder only. Refused, gate input_holder, unless the caller holds the source (screen.take), and gate state unless it watches it. Coordinates are in the units of the frame's width and height (the page's CSS pixels for a browser, the screen's points for a device), from its top left. Its reply is never kept for a retry.

### `screen.act`

An agent's own call before it uses its source (hl browser runs it before each verb, hl sim before each verb that acts): refused while a person holds it, otherwise shown to watchers. The caller is the agent itself. While a person or device holds the source every kind is refused, gate input_holder, with `<holder> has the page` (a device: `has the device`). Otherwise the source counts as open (close: no longer open) and as in use until 20 s after the agent's last call, which AgentView.screens carries. What the agent types is never sent, only the kind and the box.

### `screen.status`

An agent's own read of its source (hl browser list): who holds it, who watches it, and the machine it runs on. The caller is the agent itself. It acts on nothing: it is answered while a person holds the source, sends watchers nothing, and neither opens the source nor counts as its use.

