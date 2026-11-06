# Configuration

All state is under `~/.handline` (or `$HANDLINE_HOME`). It is a git repository, so `git log` there shows
what changed and when.

```
~/.handline/
  config.yaml  team.yaml  db.sqlite  locks/
  logs/                       daemon logs
  shared/.claude/             settings, CLAUDE.md, skills, agents, commands, plugins
  shared/mcp.json             optional MCP servers for every launch
  accounts/<name>/.claude/    one Claude config directory per account; credentials stay here
  accounts/<name>/status.json
  agents/<name>/              agent.yaml  ROLE.md  base.md  inbox/  journal/
```

`accounts/` and `logs/` are not tracked by git.

## team.yaml

Says who may send work to whom. A request that no rule allows is refused, and the refusal says why.

```yaml
teams:
  eng: [backend]

agents:
  backend: {}
  reviewer: {}

edges:
  - { from: '*', to: 'reviewer' }
  - { from: '*', to: 'human', kind: task }

deny: []

limits:
  max_chain_depth: 4
```

- `teams` groups agents. A team's manager is the member named like the team.
- `edges` allow one direction of contact. `from` and `to` are agent names or `*`.
- `deny` removes an allowed contact and always wins over an edge.
- `limits` stop runaway chains: how deep one request can chain, and how many requests an agent may
  send per hour.
- `human` is you. It is not an agent, but agents can send you a decision as a task.

The name gives the links, with no edge written: an agent may ask its parent (the name up to the
last dot, so `shop.sea` is under `shop`), its children and its siblings. A grandparent, a nephew and
another team are not reachable; a link between teams is an edge you write on purpose. A manager may
ask you. Siblings open and hand on tasks to each other directly, and their manager is told of each
move as a notice that asks nothing of it. To add or remove one link or agent without editing the file, use `hl propose`:

```bash
hl propose team shop --member sea=/Users/you/code/sea --member shore --why "split the storefront work"
hl propose link reviewer backend --why "backend may ask the reviewer for a second look"
```

A link's ends are agents, `*` or `human`. `hl propose unlink` removes any edge that is in `team.yaml`,
so an edge to a retired agent can go.

`hl propose` files a proposal for you to decide in the app's Proposals tab: an approve applies it at
once and tells the agent that asked. `hl propose list` shows proposals and what became of them.

## agent.yaml

Written when you create the agent in the app. Change it there, not by hand,
unless you also seal it on the Team page. The fields you will use: `vendor`, `model`, `effort`, `weight`
(`light` or `heavy`), `repo`, `chrome`, and `mcp_config`. Which account an
agent is on is not in this file. The daemon records it, and the app's agent settings change it.

Two more are the founder's to change in the app's agent settings: `mode` and the sandbox
(an agent that asks goes through `hl propose`). `mode` is Claude's permission mode: `default`, `auto` (the
default), `acceptEdits` or `plan`. Bypass mode is not supported. The sandbox is off
unless `sandbox: true` is in the file: `sandbox=on` starts the agent's next session inside
Claude's sandbox, and every session after it, until `sandbox=off`.

## ROLE.md and base.md

Both are plain Markdown, and both have a size limit because they are sent with every session.

| File      | Limit | What goes in it                                                     |
| --------- | ----- | ------------------------------------------------------------------- |
| `ROLE.md` | 1 KB  | What this agent owns and what it does not. Start with one sentence. |
| `base.md` | 2 KB  | Facts every session needs: paths, commands, conventions.            |

The app warns when one is over its limit. If you are over, the content belongs in the repo's
own docs, where the agent can read it when it needs it. Both files are loaded into every session,
so everything in them costs usage every time.

## Sealed files

`team.yaml` and each `agent.yaml` are sealed. The hash of what you last wrote is kept in a protected
store at `~/.config/handline/protected`. If a file no longer matches, `hl` refuses to act on it
and the Team page names it. This stops an agent from editing its own permissions, because every
agent can write to `~/.handline`.

If you edit one of these files yourself, read it over and press Seal on the Team page of the app.

## Accounts and limits

The app's Accounts page shows each account's usage windows, its agents and whether it takes work. There
you set how many agents may be mid-turn on it at once (default 2) and wake an account a refused turn
stalled (a login or a good turn there ends the stall too).

An agent's account is a preference. A session goes there unless the account is at its limit, at
its cap, or filling too fast, or is STALLED: a turn on it ended on an error that says the account
itself cannot be used (a refused login, an org that disallows it, a hold or billing problem), or on
an unrecognized error four times with no successful turn between. See [how it works](how-it-works.md) for the rules.

## config.yaml settings

Set these in the app's Settings page, or with `settings.set`. A time is in seconds. The ones marked "per agent" can also
be put in one agent's `agent.yaml`, which then wins over `config.yaml` for that agent.

| Key                                                                     | Default          | What it does                                                                                                                                   |
| ----------------------------------------------------------------------- | ---------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `active_cap`                                                            | 2                | Most agents mid-turn at once on an account that has no cap of its own (`active_cap_<account>`).                                                |
| `park_timeout_s`                                                        | 600              | Per agent. Seconds an agent may sit idle before its session is closed (parked). 0 never closes it.                                             |
| `handoff_note`                                                          | on               | Whether closing an agent first asks it to write a handoff note. Per agent too.                                                                 |
| `handoff_prompt`                                                        | built in         | Per agent. The text of that ask; blank or under 20 characters falls back to the built-in.                                                      |
| `ignore_cache_expiry`                                                   | off              | Per agent. Resume the old session even after the prompt cache (one hour) has expired, instead of starting fresh.                               |
| `bg_notice_s`                                                           | the park timeout | Per agent. How long an agent with background work may show nothing before it is told to finish or stop it.                                     |
| `bg_grace_s`                                                            | 300              | Per agent. After that notice, how long before the work is stopped if the agent shows nothing more.                                             |
| `bg_check_s`                                                            | 600              | Per agent. How often an agent with background work is asked which of it to stop.                                                               |
| `notice_digest_s`                                                       | 300              | Per agent. Notices to an agent are held this long after the first, then typed once as a digest.                                                |
| `input_hold_idle_s`                                                     | 600              | A person who holds an agent's terminal loses the hold after this long without typing.                                                          |
| `attention_wait_s`                                                      | 3600             | A request shows attention waiting once a task of it has waited on cap, wall or owner this long.                                                |
| `attention_quiet_s`                                                     | 7200             | A request shows attention quiet once nothing in its open tree has moved for this long.                                                         |
| `attention_sent_back`                                                   | 2                | A request shows attention bounced once a task of it was sent back (reopened) this many times.                                                  |
| `blocker_alarm`                                                         | off              | A queued task that other open tasks have waited on tells its holder at `blocker_alarm_s` and its lead at `blocker_alarm_lead_s`, once each.    |
| `blocker_alarm_s`                                                       | 3600             | How long that first waiter may wait before the holder is told.                                                                                 |
| `blocker_alarm_lead_s`                                                  | 14400            | How long that first waiter may wait before the holder's lead is told.                                                                          |
| `host_active_cap`                                                       | 0 (off)          | Most agents mid-turn on the whole machine, across accounts.                                                                                    |
| `load_per_core_pct`                                                     | 0 (off)          | No new session while the 1-minute load per core is over this percent. 0 turns it off.                                                          |
| `ram_free_min_pct`                                                      | 0 (off)          | No new session while free memory is under this percent. 0 turns it off.                                                     |
| `disk_free_min_gb`                                                      | 0 (off)          | The floor under which the founder is warned that the disk is nearly full; 0 warns at 10 GB.                                                  |
| `simulator_max`                                                         | 0 (off)          | Most simulators (iOS Simulator, Android emulator) at once; agents over it wait in `hl use simulator`.                                        |
| `browser_max`                                                           | 0 (off)          | Most headless browsers (Chrome, Playwright) at once; agents over it wait in `hl use browser`.                                                |
| `network_addresses`                                                     | unset            | Hosts from `connection.addresses` the network listener serves on the `-listen` port, re-listened live: LAN and `.local` over TLS (a certificate the daemon makes), Tailscale plain; each also serves the web page. Unset serves `-listen` alone. |
| `keep_awake`                                                            | off              | `on` keeps the Mac awake while the daemon runs, `working` only while an agent is mid-turn.                                                     |
| `reap_orphans`                                                          | dry              | Processes of ours left behind by a dead session: `dry` only logs what it would stop, `on` stops them, `off` does nothing.                      |
| `dev_mode`                                                              | off              | Developer switches in the app.                                                                                                                 |
| `push_prompt_asked`, `push_task_done`, `push_stall`, `push_account_out` | on               | Whether each of these events sends a notification to the founder's phone.                                                                      |
| `pacing`                                                                | on               | See below.                                                                                                                                     |
| `pace_target_pct`                                                       | 98               | Where the pace line ends: the share of the week (50 to 100) an account may have spent when its week resets.                                    |
| `pace_burst_pct`                                                        | 10               | How many points ahead of the line an account may run (0 to 50).                                                                                |

The push keys for sending (`push_apns_*`, `push_fcm_credentials`) hold the credentials for
notifications; they are not in the Settings page.

### Weekly pacing

Pacing is on by default and stays on. Each account's week is a budget that is spent evenly as
the week passes: its pace line rises from 0 at the weekly reset to `pace_target_pct` at the next
one. An account at or behind the line takes work as usual. One ahead of it by up to
`pace_burst_pct` points gets a smaller cap, and one further ahead takes no new turn until the line
catches up. Without it the first accounts to be used would be run to their weekly wall early in the
week while others sat unused, and then everything would stall at once. Pacing spreads the use
across accounts and across the week. Turn `pacing` off to see the figures without acting on them.

## Shared settings

Whatever you put in `shared/.claude/` (`settings.json`, `CLAUDE.md`, `skills`, `agents`,
`commands`, `plugins`) is linked into every account's `.claude/`. The daemon links a new item on
its next tick. To keep one account different, put a real file at that path in the account's own
directory. The daemon leaves it alone, and the app names it.

`shared/mcp.json` is optional. If it exists, every launch passes it to Claude with
`--mcp-config`. It is not generated from `~/.claude.json`.
