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

Today: `hl use` and `go/scheduler/leases` lease `simulator` and `browser`, limits `simulator_max` and `browser_max` (0 = none), one FIFO per kind, 30 minutes to wait, state in memory. `heavy_slots` is only a proposal in `docs/story2-load-research.md`. This adds kinds, a time limit, fairness across workspaces and survival of a restart.

### What is leased

- Counted pools: `simulator`, `browser`, and new `heavy` (a slot for a test or build run; limit from config, later from the live reading in the load research).
- `port`: a range set in config (`port_range`, e.g. 4000-4999); the lease carries the number, given to the command as `$HL_PORT`. The port is picked free at grant time and may still be taken by another process before the command binds it, so the command retries with a new ask on a bind failure.
- Disk and memory stay guards, not leases.

### Who holds, and the scope

The holder is an agent key, with its workspace recorded. Pools belong to the machine: two workspaces, and every project, share one simulator. The founder's setting is `simulator_max` 1; the shipped default stays 0 (none). Limits are owner settings.

One ask is for one kind. A command that needs two takes them in the fixed order `heavy`, `simulator`, `browser`, `port` (`hl use a b -- cmd` asks in that order and gives all back together); asking out of order is refused. That removes the hold-and-wait cycle.

### Waiting is a blocker, not a held agent

Today `hl use` makes the agent's turn sit in the queue for up to 30 minutes. Instead, when no lease is free:

1. The daemon queues the ask and puts a **lease-wait blocker** on the agent's current task (`waiting on simulator`), in the same write. By the one-doing rule (T5255) the task goes back to queued and the agent moves to its next task.
2. `hl use` exits at once with a fixed code (75) and the text "no simulator is free; this task now waits for one and comes back by itself; take your next task". Nothing ran.
3. When a lease frees, the first ask in the queue is **reserved** for that task (the lease is held by the task, not by a process) and the blocker is cleared, so the task returns to its agent by priority, not ahead of a task already running. The agent runs `hl use` again: it takes the reserved lease at once and goes on where the task left off (the task's notes and the turn's own memory carry the context).
4. A reservation not claimed within `lease_claim` (10 min, as the wake reservation) lapses to the next ask, and the task is blocked again at the back of the queue. The same applies when the task is closed, handed off or its agent retired.

The queue is per kind. Order: the workspace that has held the kind least in the last hour first (hold-seconds per workspace for that hour are kept by the manager; lost at restart, which only resets fairness), then first come first served. A reserved lease counts as held for the limit and for the display, so with one simulator the screen shows "held by A, reserved for B, waiting C, D". An agent that holds a lease taken with `--hold` and then asks for another kind is not blocked mid-task: it gets the refusal and finishes or releases first.

The in-process wait stays only for a shell that is not an agent's, which never leases.

### Given back

Today's ends stay: the command exits, `--release`, turn end (holds), session end or park, every process behind it gone. New:

- **`max_hold`** per kind, config (simulator 60 min, browser 30, heavy 30, port none), counted from the grant, for commands and holds alike, so a wedged `go test` cannot keep its slot.
- At 80 % the holder is told how long is left. `hl use --extend` adds one period, up to twice, and is refused while anyone waits.
- At `max_hold` the lease is revoked when someone waits, else it only shows "over time" and goes the moment someone asks. A waiting ask has no time ceiling: it is a blocker, and it shows in the queue until granted or its task ends.
- Revoking ends the lease and tells the holder. For a command the daemon signals the process group `hl use` started (TERM, then KILL after 10 s). For a hold there is no group to kill: the holder is told to stop and the lease is ended, never a process picked by name. `procOf` stays a display heuristic.

### Restart

Leases are stored in the base (kind, agent, workspace, granted-at, `hl use` pid, port). At start, a command's lease is adopted if its pid is alive and dropped if not. Holds are kept until the holder's next turn ends or `max_hold`, so the pool does not show a running simulator as free.

### Seen

`hl team` and the app list each pool: held (who, workspace, since, "over time"), waiting (position). Revocation and refusal are events on the holder's turn.

### Builder tasks

1. Store and restart adoption; `Lease.Workspace`; per-workspace fair grant (go/scheduler/leases).
2. Lease-wait blocker: `lease.take` for an agent with no free lease queues the ask, blocks its current task (db task-state rule counts it as an open blocker), returns code 75; grant reserves for the task, clears the blocker; `lease_claim` lapse (needs 1; touches db/taskstate and hl use).
3. `max_hold`, warning, `--extend`, revocation (needs 2: same files).
4. `heavy` kind and its guard patterns (needs 1).
5. `port` kind, `port_range`, `$HL_PORT` (needs 1).
6. Multi-kind `hl use a b` in the fixed order (needs 2).
7. Contract, `hl team` and the status-bar popover data (held, reserved, waiting with order, minutes held) and settings keys (needs 2, 3).

New limits merge off by default (AGENTS.md): `max_hold` and `heavy` start at 0 = none.

## Workspace rules

Today (main, read from `go/scheduler/lifecycle/launch.go` and `rulesnote.go`): `workspaces/<key>/base.md` reaches each agent of the workspace as the "# Rules for team X" part of the session's system prompt at launch, and a running session is told the changed lines as a note when the text's hash differs from the launch's (`RulesNote`), so there is no wait for a restart. Limit 2 KB. It is text only: nothing in it is kept by the daemon. What the daemon does keep is machine-wide or per agent, never per workspace: an account's agent cap, the load limits, lease limits, the agent's `mode` and sandbox (founder only), the global disallowed-tools list, the access grants. A workspace has a folder, added folders and a lead, and no rule.

### Two kinds of rule

- **Text rules** (`base.md`): how we work, in words an agent reads. No check. Stays as is.
- **Kept rules**: rows `workspace_settings(workspace, key, value)`, set by the owner only with `workspace.rule.set` and read with `workspace.rules`. A value on the workspace beats the machine default (the row of workspace 0), which beats the built-in. Each key names the one place that keeps it.

| Key | Meaning | Kept at |
|---|---|---|
| `accounts` | accounts the workspace's agents may run on (empty = any) | `Admit` / `FindRoom` filter; a pin outside it is refused |
| `agent_cap` | most agents of the workspace mid-turn at once (0 = none) | the start gate beside the account cap |
| `lease_max.<kind>` | most leases of a kind the workspace holds at once | the lease grant (fair share, see Resource leases) |
| `tools_deny` | tools its agents may not use, added to the global list | launch argv and the PreToolUse guard |
| `mode_max` | strongest mode an agent of it may be given (`plan` < `default` < `acceptEdits` < `auto`) | agent create and settings write |
| `review_required` | a task with commits needs a review comment from another agent before `hl task done` | the task.done write |

New keys merge off (0 or empty = no rule); the founder turns them on. Nothing else is a kept rule until a kept place for it exists: "how work lands" (direct to main or a PR) stays text in `base.md` until the daemon can see a landing.

### Told when it changes

The settings render into the same "team rules" part as a short block under the text ("Kept rules: agent_cap 3; accounts alpha, gamma"), so the existing hash and `RulesNote` tell a running agent what changed, with the changed lines only. A key that acts only at the next session start (`tools_deny`, `mode_max`) says so in that line. A refusal by a kept rule names the key, its value and who set it.

### Tests

One table test per key: value on the workspace, on the machine only, on neither (precedence); the place that keeps it refuses or allows with the exact text; a change produces the note once; a second workspace is unaffected. Writes by a non-owner are refused.

### Builder tasks

1. `workspace_settings` table, owner write rpc and contract, precedence resolve, rendering into the rules block (go/data, contract). First.
2. `accounts` filter and `agent_cap` gate (needs 1; touches usage/pick and lifecycle launch).
3. `lease_max.<kind>` (needs 1 and the leases manager task).
4. `tools_deny` and `mode_max` (needs 1; launch argv and agent create).
5. `review_required` at task.done (needs 1).
6. The rules column for T8955: key, value, source (workspace or machine), the `base.md` size (needs 1).

## Later phases

- **2 - personal mode and the first run.** The screens are designed and shown to the founder before they are built: first run, new workspace, the sidebar by workspace, settings. The first-run fixes already agreed are part of it: installing the tools on a bare Mac, the account and folder steps, the lead proposing the team, the settings cut, resource leases, the one device tab, signing. Signing needs the founder's Apple developer account.
- **3 - between workspaces.** Delivery to a workspace's lead, its hand-on or send-back, and both sides seeing the task.
- **4 - team mode.** Invite links, handles, roles, what a member sees, turns and the bell per person, agents running only on their owner's accounts, and each `actor.OwnerKey` becoming the agent's owner or the task's person. A legal check of the plan terms comes first.

## Not now

- Several Macs in one cluster.
- An agent addressing another workspace's agent directly.
- Per-agent folder grants (`go/scheduler/access`). Added folders on the workspace replace them.
