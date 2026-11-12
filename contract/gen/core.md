<!-- Generated from contract/spec/ by contract/generate.py; do not edit. -->
# core methods (protocol 1)

A method marked write takes a required `request_id`. Framing, roles, errors and gates are in [protocol.md](../protocol.md); field types in `protocol.ts`.

| Method | Role | Write | Params | Result | Errors | Gates | App |
|---|---|---|---|---|---|---|---|
| `hello` | anonymous |  | `client`, `protocol`, `token?`, `key?`, `client_build?`, `cwd?`, `note?` | `{protocol, min_client, daemon, version, exe, schema, role, caller, capabilities, transport, methods}` | `refused`, `bad_request` |  | not cached |
| `whoami` | agent |  | none | `{caller, role, methods}` |  |  | not cached |
| `subscribe` | agent |  | `topics`, `since?`, `resume?`, `agent?` | `{cursor}` | `bad_request`, `refused` | role | not cached |
| `unsubscribe` | agent |  | `topics` | `object` | `bad_request` |  | not cached |
| `daemon.status` | agent |  | none | `DaemonStatus & ReadCursor` |  |  | `status`, fed by status |
| `daemon.pause` | owner | yes | `until?`, `request_id` | `Accepted` | `refused`, `bad_request` |  | decided: pause new work |
| `daemon.resume` | owner | yes | `request_id` | `Accepted` | `refused` |  | decided: resume new work |
| `lease.take` | agent |  | `agent`, `kind`, `hold?` | `{lease, waited_s, port}` | `refused`, `bad_request` | lease_wait, lease_order, lease_ceiling | not cached |
| `lease.release` | agent |  | `agent`, `lease?`, `kind?` | `{released}` | `refused`, `bad_request` |  | not cached |
| `lease.extend` | agent |  | `agent`, `kind?` | `{extended}` | `refused`, `bad_request` |  | not cached |
| `lease.get` | agent |  | none | `{kinds}` |  |  | `leases`, snapshot |
| `load.get` | agent |  | `minutes?`, `agent?` | `{at, every_s, machine, total, agents, unowned, processes, sampling, series} \| null` | `bad_request` |  | `load`, snapshot |
| `events.feed` | agent |  | `since?`, `topics?`, `limit?` | `{events, cursor, gap}` | `bad_request` |  | not cached |
| `operation.get` | member |  | `operation` | `Operation & ReadCursor` | `not_found` |  | `operation` by operation, fed by operations |
| `debug.terminals` | member |  | none | `{host, now, panes}` | `refused` | role, unsupported, state | `debug-terminals`, snapshot |
| `prefs.get` | member (only member, owner) |  | none | `Prefs & ReadCursor` | `refused` |  | `prefs`, fed by prefs |
| `prefs.set` | member (only member, owner) | yes | `changes`, `request_id` | `Accepted` | `bad_request`, `refused` |  | authored: save view preferences |
| `hook.record` | agent (only agent) | yes | `agent`, `kind`, `at`, `data`, `request_id` | `Accepted & {nudge}` | `refused` | not_self | not from the app |
| `subagent.start` | agent (only agent) | yes | `agent`, `session_id?`, `description?`, `subagent_type?`, `run_in_background?` | `{deny}` | `refused`, `bad_request` | not_self | not from the app |
| `memory.status` | agent |  | `agent?` | `{agents}` |  |  | `memory`, snapshot |
| `connection.get` | owner |  | none | `ConnectionInfo` |  |  | `connection`, snapshot |
| `connection.addresses` | owner |  | none | `ConnectionAddresses` |  |  | `addresses`, snapshot |
| `settings.get` | owner |  | `agent?` | `Settings` | `bad_request`, `not_found` |  | `settings`, snapshot |
| `settings.set` | owner | yes | `changes`, `agent?`, `request_id` | `Accepted` | `refused`, `bad_request` |  | authored: save settings |
| `for_you.list` | member |  | none | `ForYouList & ReadCursor` |  |  | `for-you`, fed by for_you |

### `hello`

First call on every connection: versions and who the caller is. A client whose protocol is below min_client gets `refused` (gate `protocol`) with hint `Open the Handline app.`. A newer client gets a normal reply and decides itself whether to run. Any other call before hello is bad_request. Role by proof: a matching `key` gives founder; with no key, a peer pid that verifies against a recorded session gives agent; a device token (network listener) gives member; anything else is anonymous and every method but hello is refused until a later hello proves more. On the network listener a hello without a valid `token` is refused, gate `unpaired`, with no data; with one it is member.

### `whoami`

The viewer, the device, the role and the methods allowed.

### `subscribe`

Turn the connection into a stream of `event` notices; replays after `since` first, unless `resume` is latest. topics is a non-empty list of known topics, else -32602 bad_request naming `topics`. Notices start only after this reply is sent. devices is subscribed on its own, by the founder: a topics list naming devices and any other topic is -32602 bad_request naming `topics`, because devices is live only and every other topic has a cursor. A devices subscription is live only: `since` and `agent` are ignored, the reply cursor is 0, its events carry cursor 0, and a client that reconnects reads devices.list again.

### `unsubscribe`

Stop some topics; a subscription with no topics left ends. Result {}. Naming devices follows the same rule as subscribe: on its own.

### `daemon.status`

Status bar: alive, versions, terminal host, agent counts, inbox count.

### `daemon.pause`

Pause new work: from now no new turn of an agent starts and the daemon starts none of its own work, until daemon.resume or until. A running turn goes to its end, and a turn a person starts still goes. A pause while paused replaces its until; since stays. The pause is kept across a daemon restart.

### `daemon.resume`

End a pause (daemon.pause): held work starts at the next tick. Accepted, and changes nothing, when not paused.

### `lease.take`

hl use asks for a test or build run, simulator, browser or port lease. A free one with no ask ahead in line (per kind, the workspace that held the kind least in the last hour first, then first come first served), or one reserved for the agent's current task, is answered at once. Otherwise an agent with a current task is refused with gate lease_wait: its task waits in line on a gate task and comes back when a lease is reserved for it. A call with no current task is held until one is free, refused after 30 minutes naming who holds them. With the kind's limit 0 (heavy_max, simulator_max, browser_max; port: with no port_range) it is granted at once. A port is free when granted, not reserved against other programs: a command that gets 'address in use' asks again. A command needing several kinds asks for them in the order heavy, simulator, browser, port; one that would wait while its agent holds a hold, or while the calling process runs under a later kind, is refused (lease_order). A task that waits for a kind gives up the leases reserved for it of later kinds. A command's lease (hold false) belongs to the calling process, the hl use the command runs under: it is freed by lease.release, by the agent's session end, or within 5 s once that process and every process last seen under it are gone. A hold is freed by lease.release or the agent's turn or session end; the PreToolUse guard lets the agent's own starters of its kind through while it is held. An ask whose caller hangs up leaves the queue. Leases are stored: after a restart a hold is kept until its holder's turn ends, and a command's lease while the hl use process it was granted to runs. A lease reserved for a task waits 10 minutes (lease_claim) for its agent's hl use, then goes to the next ask; its task is not blocked again until its next hl use asks again; an ask or reservation whose task ends or leaves its agent is dropped and its gate closed. Reserved leases count toward the limit; turn and session ends leave them.

### `lease.release`

Gives back the caller's lease, or with no lease every one it holds of kind (of every kind when kind is absent); the first ask waiting gets it.

### `lease.extend`

hl use --extend: adds one max_hold period to each of the caller's leases of kind (of every kind when kind is absent). Refused, changing nothing, when a lease of the named kind has no max_hold (with no kind, those are passed over), one was extended twice already, would still be past its max_hold, or has an ask of its kind waiting. A lease's max_hold (<kind>_max_hold_min in config.yaml; heavy 30, simulator 60, browser 30, port none) counts from its grant (time reserved for a task included) and only while its kind has a limit. At 80 % its agent is told; once past it, the lease is taken back as soon as an ask of its kind waits: a command's hl use gets TERM, which it passes on, and the command's processes are killed 10 s later; a hold's agent is told to stop.

### `lease.get`

Every lease kind (test or build run, simulator, browser, port): per kind its limit and max_hold, who holds each lease (workspace, since when, its processes, when it reaches its max_hold), what is reserved for which task until when, and who waits, in order.

### `load.get`

What every process on this machine uses and whose it is: per agent (CPU, memory, process count, heaviest processes), per process, and the machine's totals; with `minutes`, each agent's per-minute series. Read only; no limit is applied from it.

### `events.feed`

Resumable feed of events with their deep-link targets; the polling twin of subscribe. topics defaults to [notifications]; limit defaults to 50, at most 200. Without `since`: the newest events, oldest first. With `since`: the events after it; if `since` is older than retention the feed starts at the oldest kept event and `gap` is true. cursor is the newest cursor in the reply, or the current newest when there are no events.

### `operation.get`

A long-running operation's current state.

### `debug.terminals`

Every pane of the terminal host with what the daemon knows of it, for the hidden debug window. Lists every pane of the daemon's terminal server (tmux), whichever agent has it or none, to find panes kept open past their prompt cache and shells left behind after an agent's session exited. Reads only: the host's pane list, the process table and the daemon's own rows; of the agent's view only cache_until. A host that cannot list its panes is refused, gate unsupported; one that cannot be reached, gate state. Watching a listed pane is terminal.attach by its handle; a pane no agent has takes no input. Callers: the founder and a paired device (member); below that is refused, gate role.

### `prefs.get`

The person's view preferences: the agent list's pins, filter, layout and folds.

### `prefs.set`

Change the person's view preferences: each key given in changes.nav replaces the stored one, but `later` merges by item. A pin or an order entry naming an agent the daemon does not know, or a name given twice, is bad_request naming the field. Once stored, a prefs.changed event on the prefs topic carries the whole Prefs. agent.retire drops the agent from pins and order (and sends prefs.changed); a rename (proposals) carries it over in place.

### `hook.record`

A hook records an event; the spool replays it with the same request_id. A PostToolUse that started a background command is noted for the agent; a Stop answers with the founder's nudge when one is due.

### `subagent.start`

The PreToolUse guard asks before an Agent call: refused when one more subagent would take the agent's account to its 5-hour or weekly limit before that window resets, else the start is recorded. The account's recorded burn plus the agent's own measured cost stands for the subagent's. Unread, stale or unmeasured figures refuse nothing. A start runs until a SubagentStop of the same session or until that session ends (subagents.counts). A hook the daemon does not answer in time lets the call through, unrecorded.

### `memory.status`

Per-agent memory against its budget (hl memory status).

### `connection.get`

How this process is reached: its doors and dev mode. Read only.

### `connection.addresses`

Every host this machine can be reached on, and whether a door serves it. Read only. Read again on every call: addresses come and go with Wi-Fi. Choosing hosts is settings.set network_addresses.

### `settings.get`

Timings, terminal host, trust folders, hooks installed.

### `settings.set`

Change settings: any keys of SettingsValues, globally or as one agent's override.

### `for_you.list`

Every open item waiting on the founder, whatever it is made of today (task rows, comment asks, prompts, proposals, the machine's needs), and the count of those not deferred.

