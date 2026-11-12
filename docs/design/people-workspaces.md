# People, workspaces and agents by id

Decided by the founder on 2026-11-11. This doc is the spec for phase 1, the new base. Phases 2 to 4 are named only so phase 1 leaves room for them.

## Why

- An agent's dotted name (`shop.designer`) is its key everywhere: in about 25 tables, its folder, its worktree and its Claude project history. A rename breaks all of them. Two projects cannot both have a `designer`.
- There is one person, `human`. Every write by a person or a paired device is stored as `human`. 327 places in 73 Go files compare against it.
- Founder: start in personal mode with no sign-up. Team mode turns on when a second person is invited. Agents get addresses they cannot confuse, and people stay free to name things.

## Model

| Entity | Key | Fields | Unique |
|---|---|---|---|
| person | `p<n>` | display name, handle (empty in personal mode), role `owner` or `member` | handle, across the cluster |
| workspace | `w<n>` | name, project folder, added folders, lead agent | name, across the cluster |
| agent | `a<n>` | workspace, name, owner (a person), reports to (an agent or a person), model, effort, weight, enabled, schedules, context size | name, within its workspace |
| account | `c<n>` | label shown, owner (a person), plan, ends at, agents at once, paused | label, per owner |
| device | `d<n>` | person, name, paired at | - |

- These are tables in `db.sqlite`, the one truth. `team.yaml` and every `agent.yaml` go away. `hl hr` and the app write the rows. `ROLE.md` and `base.md` stay files, because agents read them.
- The cluster is one Handline install. It is not a row.
- The daemon's own sender (`task-board` today) is the reserved key `sys`. The owner of a task that watches outside state (`gate` today) is the reserved key `gate`.

## Keys

- A key is the kind letter plus the row id (`a17`). It never changes and is never reused.
- Every column that names a who holds a key, and stays `TEXT`: `tasks.owner`, `opened_by`, `reporter`, `discarded_by`, `requests.requester`, `target`, `task_memos.author`, `task_turns.agent`, `pushed_by`, `events.agent`, `agent_state.agent` and the rest. Account columns hold `c` keys.
- Code compares keys, never names. `db.Human` goes away: `actor.IsPerson(k)`, `actor.Kind(k)` and the caller's role replace the comparisons. Where the daemon names the person with no caller in hand (a request to them, their card, an event they cause), phase 1 writes `actor.OwnerKey`, which is `p1`.
- Words exist only at the edges: an address read from `hl` or a comment, and a name written out to an agent or the app.

## Names

- Workspace names, agent names and handles: lowercase letters, digits and `-`, 1 to 32 long, starting with a letter. No dots: the dotted team form is gone.
- Reserved: `human`, `me`, `all`, `everyone`, `sys`, `gate`.
- Collisions are refused when something is created or renamed, so any word points to one thing:
  - A workspace name may not equal any agent name in any workspace, and an agent name may not equal any workspace name.
  - A handle may not equal any workspace or agent name.
- One function, `names.Check(kind, word, workspace)`, holds these rules. The app and `hl` show its refusal as it is.
- A new workspace's first agent is its lead and is named `lead`. It can be renamed.

## Addresses

An agent uses three forms. `hl say`, `--assign`, `hl task handoff`, `hl task turn` and comment mentions all take them:

1. `designer`: an agent in the caller's own workspace.
2. `shop`: another workspace. Its lead receives it and hands it on or sends it back.
3. `human`: the caller's owner. In team mode a handle (`jisoo`) names any other person.

- One function, `addr.Resolve(caller, word)`, turns a word into a key, in this order: `human`, an agent of the caller's workspace, a workspace (its lead), a handle. Nothing else resolves. The refusal lists the names the caller may use.
- A person typing in `hl` has no workspace of their own. Agent names resolve in the workspace whose project folder holds the current directory. Elsewhere only workspace names and handles resolve. The app always sends keys.
- Agents see an agent of another workspace as `designer (shop)`. That is display only and does not resolve. A reply to it goes by `--parent`, which carries the key.
- `hl team` lists the caller's workspace agents, the people and the other workspace names. It never lists another workspace's agents.
- Routing follows the same lines. An agent may reach every agent of its workspace, its owner and any workspace. The edges and deny lists of `team.yaml` go away, with the parent, child and sibling links read from dotted names. The chain depth limit stays.

## Mentions

- On write, the daemon replaces each `@word` that resolves for the writer with `<@key>`, the way Slack stores `<@U123>`. An `@word` that does not resolve stays text.
- On read, `<@key>` is written out for the reader: the address the reader would use for an agent, a person's display name in the app, and a link.
- A comment reaches the agents it mentions, as today. `<@p…>` as the first token is a note to that person: it moves no turn.
- A rename therefore changes no stored text.

## Who is calling

- `Caller` becomes `{kind: person|agent|device|anonymous, key, person, role}`. `person` is the person a device belongs to. `role` is that person's.
- `Actor()` returns the key. A person's writes are stored under their own key, and a device's writes under its person's key.
- Method roles become `agent`, `member` and `owner` (today's `founder`). A paired device acts with its person's role. It no longer has a role of its own.
- The proofs stay as they are: an agent by its process lineage, the owner by the founder key on this Mac, a device by its pairing.

## Personal and team mode

- First start makes the owner, `p1`: role `owner`, the display name from the Mac account's full name, and no handle. No sign-up, no email, no Handline login. The only login is the Claude account Claude Code needs anyway.
- Team mode means more than one person row. It is worked out from the rows, not stored as a flag.
- In personal mode, the app shows no people, roles, invites or visibility, and agents address the person as `human`.
- Phase 1 builds only personal mode. The person columns (owner, role, a device's person) are there, so phase 4 adds rows, not columns.

## Disk

```
~/.handline/
  db.sqlite              people, workspaces, agents, accounts, devices, tasks, ...
  agents/a17/            ROLE.md, memory/, inbox/, journal/, handoffs/
  accounts/c3/           that account's Claude config dir (identity.json and the rest)
  workspaces/w2/         base.md (the workspace's team rules)
  attachments/ logs/ run/
<project>.worktrees/a17  the agent's worktree
```

- Folders are named by key, so a rename moves nothing, and Claude's project history, which is keyed by the working directory, stays attached.
- An agent works in its worktree of the workspace's project folder. The added folders reach Claude Code as `--add-dir`.
- Runtime handles carry the key too: the tmux session and `HANDLINE_AGENT`. The launch prompt tells the agent its name and its workspace.

## Contract

- Every who field in `contract/protocol.json` carries a key.
- New reads give the words: `people.list`, `workspaces.list` and `agents.list` (key, name, workspace, owner). The app keeps them cached and writes out every key through them. A rename is a `table_changes` bump on that list.
- Routes and tab ids use keys (`chat:a17`), so open tabs survive a rename.
- The architect reviews the contract diff before it lands.

## No migration: a throwaway import

The founder: no migration code in the product. A temporary importer is enough, since there are no users yet.

- The new base starts from one schema file, version 1. The 51 files in `go/kit/store/migrations` are deleted. The daemon never reads the old layout.
- `go/cmd/hl-import-v1` is a separate binary that is run by hand once. It is not linked into `handlined` or `hl`. It reads the old home (moved aside to `~/.handline-v1`) and writes the new one.
  - The first segment of a dotted name is the workspace (`shop.ui-review` is in `shop`), and the rest is the agent name (`ui-review`). The agent named just the segment becomes the lead, `lead`. The project folder is the lead's `repo`. Reports to comes from today's parent name. `human` becomes `p1`.
  - It brings agent config, agent folders, account folders (label = old name), tasks with their ids, comments, turns, links, tags, blockers, chat requests and prefs. Names in who columns become keys, and `@name` in bodies becomes `<@key>`.
  - It leaves behind events (468k rows), usage samples, token calls, transcripts, delivery state, operations and sessions. These start empty.
- The importer is deleted one week after the cutover.

## Build: phase 1

Work happens on a branch, `base-v2`. It cannot land in parts, because the team runs on main. While the branch is open, no other change lands on main under `go/` or `contract/`, except a fix for a red main. Web work goes on against today's contract until 1e.

| Step | What | Needs |
|---|---|---|
| 1a | Schema version 1. The entity tables, `actor` keys, `names.Check`, the owner at first start. Config comes from rows. `team.yaml` and `agent.yaml` readers are deleted. | - |
| 1b | Who columns hold keys. `db.Human` and the name comparisons go. The new `Caller` and the `owner` and `member` roles. | 1a |
| 1c | `addr.Resolve` behind every `hl` and daemon input. Mentions as `<@key>`. Every text to an agent is written out per reader (deliveries, refusals, `hl` output, `hl team`). | 1b |
| 1d | Disk by key: agent, account and workspace folders and worktrees. Launch reads its config from rows. | 1a |
| 1e | The contract and the app: the three lists, keys in routes and tabs, the sidebar grouped by workspace. | 1c |
| 1f | `hl-import-v1`, a dry run on a copy of this Mac's home, then the cutover: stop the daemon, move the home aside, import, start, land `base-v2` on main by fast-forward. | 1c, 1d, 1e |

- After 1f, the old home stays at `~/.handline-v1` for a week, to read from if something was missed.
- Done means the team runs on the new base and every agent reaches the others by the three forms.

## Resource leases

What is there now (`hl use`, `go/scheduler/leases`, `leaseguard.go`): two kinds, `simulator` and `browser`, each with a limit in config (`simulator_max`, `browser_max`, 0 = no limit). An agent runs `hl use <kind> -- <command>` (lease held while the command runs) or `--hold` / `--release`. Asks queue first come first served, up to 30 minutes, then are refused naming the holders. A lease ends when the command exits, on `--release`, at the turn's end (holds) or session's end, or when the `hl use` process and its children are gone. A PreToolUse guard refuses a Bash call that starts a simulator or browser outside `hl use` while the kind has a limit. Leases live in memory only. `heavy_slots` is not built: it is a proposal in `docs/story2-load-research.md` (slots for test and build runs, from idle cores).

### What is leased

A lease is a claim on something the machine has few of. Three kinds of pool:

| Kind | Pool | Limit |
|---|---|---|
| `simulator`, `browser` | counted (as today) | config, 0 = none |
| `heavy` | counted slots for tests and builds | config, or the live reading in the load research; 0 = none until measured |
| `port` | a range, e.g. 4000-4999; the lease carries the port number | the range |

`hl use port -- <command>` sets `$HL_PORT` for the command; `hl use port --hold` prints it. Nothing else is leased: disk and memory stay guards, not leases.

### Who holds, and the scope

The holder is an agent key (people-workspaces keys), with its workspace recorded. The pools belong to the machine, not to a workspace: two workspaces on one Mac share one simulator. Limits are set in the settings (owner only). Queues are fair across workspaces: the next grant goes to the workspace that has held least in the last hour, first come first served inside it, so a busy workspace cannot starve another.

### When it is given back

A lease ends at the first of: the command exits; `hl use --release`; the end of the turn (holds); the end of the session; every process behind it gone (as today); **a time limit**, new. Each kind has a `max_hold` (default simulator 60 min, browser 30, heavy 30, port none). A command's lease is not cut while its process runs unless a hold. A hold past `max_hold` is revoked.

### Taking back what is not returned

1. At 80 % of `max_hold` the holder gets a notice naming the lease and the time left. `hl use --extend` adds one period, once, and only when nobody waits.
2. At `max_hold` with a waiter in the queue the daemon revokes: it tells the holder, ends the lease, kills the processes that matched the kind for a hold (the kind's patterns in `procOf`, only the holder's own), and grants the next waiter. With no waiter nothing is revoked, and the line in `hl team` says "over time".
3. A holder whose session is gone or parked loses its leases at once.
4. At daemon start, holds are dropped (their turns ended with the daemon) and a command's lease is adopted again if its `hl use` pid is alive, else dropped.
5. A waiter that hits the 30-minute ceiling is refused as today, and the refusal names who holds and for how long.

### Seen

`hl team` and the app list each pool: held (who, since, workspace), waiting (position), over time. Revocation and refusal are events on the holder's turn.

### Builder tasks

1. Persist nothing; add `Lease.Workspace` and fairness across workspaces in `leases.Manager` (tests: two workspaces, one starved).
2. `max_hold`, warning notice, `hl use --extend`, revocation with process kill (needs 1).
3. `heavy` kind: `hl use heavy -- <cmd>`, guard patterns for test and build commands, limit from config; the live reading is a later follow-up.
4. `port` kind: range pool, `$HL_PORT`, free-port check before grant.
5. Daemon start: drop holds, adopt live command leases.
6. Contract and `hl team` / app lines for the new kinds and "over time"; settings keys.
Steps 1 and 3-5 are independent; 2 and 6 follow 1.

## Later phases

- **2 - personal mode and the first run.** The screens are designed and shown to the founder before they are built: first run, new workspace, the sidebar by workspace, settings. The first-run fixes already agreed are part of it: installing the tools on a bare Mac, the account and folder steps, the lead proposing the team, the settings cut, resource leases, the one device tab, signing. Signing needs the founder's Apple developer account.
- **3 - between workspaces.** Delivery to a workspace's lead, its hand-on or send-back, and both sides seeing the task.
- **4 - team mode.** Invite links, handles, roles, what a member sees, turns and the bell per person, agents running only on their owner's accounts, and each `actor.OwnerKey` becoming the agent's owner or the task's person. A legal check of the plan terms comes first.

## Not now

- Several Macs in one cluster.
- An agent addressing another workspace's agent directly.
- Per-agent folder grants (`go/scheduler/access`). Added folders on the workspace replace them.
