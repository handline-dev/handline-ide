# Configuration

All state is under `~/.handline` (or `$HANDLINE_HOME`).

```
~/.handline/
  config.yaml  db.sqlite  locks/
  logs/                       daemon logs
  shared/.claude/             settings, CLAUDE.md, skills, agents, commands, plugins
  shared/mcp.json             optional MCP servers for every launch
  accounts/<key>/.claude/     one Claude config directory per account (keyed c<n>); credentials stay here
  accounts/<key>/status.json
  agents/<key>/               inbox/  memory/  memory-use.json
```

## Workspaces and agents

Workspaces and agents are rows in `db.sqlite`, written when you create them in the app. A workspace
has one folder; each of its agents works in a desk of it (desk-<n>, taken at start and given back at
close; its branch stays its own). Change an agent's settings in the
app: `vendor`, `model`, `effort`, `weight` (`light` or `heavy`), `chrome`, `mcp_config`, and the
founder-only `mode` (`default`, `auto`, `acceptEdits` or `plan`; bypass is not supported) and sandbox.
Which account an agent is on is recorded by the daemon and changed in the app's agent settings.

An agent is named `workspace.agent` and a workspace's lead by the workspace alone. An agent may ask you, anyone in its own workspace and the lead of another. To add, retire or rename an
agent, or add a workspace with its agents, an agent uses `hl propose`:

```bash
hl propose team shop --member sea --member shore --repo ~/code/shop --why "split the storefront work"
```

`hl propose` files a proposal for you to decide in the app's Proposals tab: an approve applies it at
once and tells the agent that asked. `hl propose list` shows proposals and what became of them.

## Role, standing context and team rules

An agent's role (ROLE.md) and standing context (base.md), and its workspace's rules, live in the daemon's
database, one version per change with who wrote it, who OK'd it and when. Nothing edits them on disk: an
agent proposes a whole new text with `hl propose context <agent> --role-file F`, `--base-file F` or
`--rules-file F` (its workspace's rules), and the founder decides it. A running agent restarts with the
new text at its next idle moment, and the chat's Restarted line says what changed and by whom.

Keep each small, because it is sent with every session:

| Part              | Aim   | What goes in it                                                     |
| ----------------- | ----- | ------------------------------------------------------------------- |
| role              | 1 KB  | What this agent owns and what it does not. Start with one sentence. |
| standing context  | 2 KB  | Facts every session needs: paths, commands, conventions.            |
| workspace rules   | 2 KB  | How the team works together.                                        |

Anything longer belongs in the repo's own docs, where the agent can read it when it
needs it.

## Accounts

The app's Accounts page shows each account's usage windows, its agents and whether it takes work. There
you wake an account a refused turn stalled (a login there ends the stall too).

An agent's account is a preference. A session goes there unless the account is at its wall, near
it, filling too fast, or stalled: a turn on it ended on an error that says the account itself cannot
be used (a refused login, an org that disallows it, a hold or billing problem). See
[how it works](how-it-works.md) for the rules.

## Agent memory

Each agent's Claude sessions keep their auto-memory in `agents/<key>/memory/` (set per launch with
`autoMemoryDirectory`), not in a folder shared by every agent of the repo. Other agents' memory is
hidden from it unless the founder grants a read. A hook notes each memory file the agent reads or
edits in `agents/<key>/memory-use.json`.

## config.yaml settings

Set these in the app's Settings page, or with `settings.set`; every key it takes, with its kind, is
`settingKinds` in `go/internal/adminwrites/settings.go`. A time is in seconds. The ones marked "per
agent" can also be set on one agent's row (the app's agent settings), which then wins over
`config.yaml` for that agent. The most used:

| Key | Default | What it does |
| --- | --- | --- |
| `park_timeout_s` | 600 | Per agent. Seconds an agent may sit idle before its session is closed (parked). 0 never closes it. |
| `handoff_note` | on | Whether closing an agent first asks it to write a handoff note. |
| `handoff_prompt` | built in | Per agent. The text of that ask; blank or under 20 characters falls back to the built-in. |
| `ignore_cache_expiry` | off | Per agent. Resume the old session even after the prompt cache has expired, instead of starting fresh. |
| `bg_notice_s` | the park timeout | Per agent. How long an agent with background work may show nothing before it is told to finish or stop it. |
| `bg_grace_s` | 300 | Per agent. After that notice, how long before the work is stopped if the agent shows nothing more. |
| `bg_check_s` | 600 | Per agent. How often an agent with background work is asked which of it to stop. |
| `notice_digest_s` | 300 | Per agent. Notices to an agent are held this long after the first, then typed once as a digest. |
| `input_hold_idle_s` | 600 | A person who holds an agent's terminal loses the hold after this long without typing. |
| `attention_wait_s` | 3600 | A request shows attention waiting once a task of it has waited on cap, wall or owner this long. |
| `attention_quiet_s` | 7200 | A request shows attention quiet once nothing in its open tree has moved for this long. |
| `attention_sent_back` | 2 | A request shows attention bounced once a task of it was sent back (reopened) this many times. |
| `blocker_alarm` | off | A queued task other open tasks wait on tells its holder after `blocker_alarm_s` (3600) and its opener after `blocker_alarm_opener_s` (14400), once each. |
| `usage_move_before_pct` | 90 | Share of the five-hour window at which an idle agent with work waiting moves to an account with room. 0 moves only at the wall. |
| `account_split` | auto | `manual`: the daemon never places or moves an agent; it waits at its account's wall. |
| `pace_target_pct` | 98 | Where an account's weekly pace line ends: the share of the week (50 to 100) it may have spent when its week resets. |
| `pace_burst_pct` | 10 | How many points ahead of that line still counts as on it (0 to 50). The pace line is shown; it holds no start. |
| `host_active_cap`, `load_per_core_pct`, `ram_free_min_pct` | 0 (off) | Machine limits on a new session: agents mid-turn, 1-minute load per core, free memory. |
| `disk_free_min_gb` | 0 | Free disk under which the founder is warned; 0 warns at 10 GB. |
| `review_flow` | off | A finished task that serves the founder is Ready for their look (see Review below). |
| `disk_keep_free_gb`, `agents_memory_max_gb` | 0 (off) | Under this much free disk the free desks are removed, oldest first; over this much agent memory the longest-idle sessions close. |
| `port_range` | unset | Ports `hl use port` picks from, as `4000-4999` (1024 to 65535), given to the command as `$HL_PORT`; asks wait when all are held. Unset, any free port and none waits. |
| `<kind>_max_hold_min` | 30 for a shared run, 0 otherwise | Minutes a lease of the kind may be held while the kind has a limit. At 80% the holder is told; past it, the lease is taken back once someone asks. `hl use --extend` adds a period. Set in `config.yaml` only. |
| `network_addresses` | unset | Hosts the network listener serves on the `-listen` port: LAN and `.local` over TLS (a certificate the daemon makes), Tailscale plain. Unset serves `-listen` alone. |
| `keep_awake` | off | `on` keeps the Mac awake while the daemon runs, `working` only while an agent is mid-turn. |
| `reap_orphans` | dry | Processes of ours left behind by a dead session: `dry` only logs what it would stop, `on` stops them, `off` does nothing. |
| `dev_mode` | off | Developer switches in the app. |
| `push_prompt_asked`, `push_task_done`, `push_stall`, `push_account_out` | on | Whether each of these events sends a notification to the founder's phone. |

The push keys for sending (`push_apns_*`, `push_fcm_credentials`) hold the credentials for
notifications; they are not in the Settings page.

## Review

With `review_flow` on, a finished task that serves the founder is Ready for their look. How often an agent's finished
tasks come to the founder is set per agent and kind of work (`agent.review_set`): every (the default), 1 in 2, 1 in 3,
1 in 5, or never. The kind is read from the files a task's commits change: contract (the contract, migrations, a store's
schema), screen (`apps/`, `packages/`), docs (`docs/`, `*.md`, or only deletions) and daemon (the rest). Contract work
always stays at every. After 10 first-look OKs in a row at a level the founder is offered the next level down; only the
founder lowers a level, and a Redo puts every kind of that task back to every.

## Shared settings

Whatever you put in `shared/.claude/` (`settings.json`, `CLAUDE.md`, `skills`, `agents`,
`commands`, `plugins`) is linked into every account's `.claude/`. The daemon links a new item on
its next tick and before a launch. To keep one account different, put a real file at that path in the
account's own directory; the daemon leaves it alone.

`shared/mcp.json` is optional. If it exists, every launch passes it to Claude with
`--mcp-config`. It is not generated from `~/.claude.json`.
