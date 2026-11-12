<!-- Generated from contract/spec/ by contract/generate.py; do not edit. -->
# team methods (protocol 1)

A method marked write takes a required `request_id`. Framing, roles, errors and gates are in [protocol.md](../protocol.md); field types in `protocol.ts`.

| Method | Role | Write | Params | Result | Errors | Gates | App |
|---|---|---|---|---|---|---|---|
| `proposal.propose` | agent | yes | `kind`, `args`, `why?`, `request_id` | `Accepted & {proposal}` | `refused`, `bad_request` |  | not from the app |
| `proposal.withdraw` | agent | yes | `proposal`, `request_id` | `Accepted` | `refused`, `not_found` | proposal_settled | decided: withdraw |
| `trust.propose` | agent | yes | `root`, `why?`, `request_id` | `Accepted & {proposal}` | `refused` |  | not from the app |
| `trust.list` | agent |  | `proposals?` | `{roots, proposals, declared}` |  |  | `trust`, snapshot |
| `trust.remove` | owner | yes | `root`, `request_id` | `Accepted` | `not_found` |  | authored: remove folder |
| `people.list` | agent |  | none | `{people}` | `bad_request` |  | `people`, fed by people |
| `workspaces.list` | agent |  | none | `{workspaces}` | `bad_request` |  | `workspaces`, fed by workspaces |
| `workspace.create` | owner | yes | `name`, `folder`, `account?`, `request_id` | `OperationStarted` | `refused`, `not_found`, `bad_request` | state, not_git | decided: create workspace |
| `workspace.set` | owner | yes | `workspace`, `name`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` | state | decided: rename workspace |
| `workspace.delete` | owner | yes | `workspace`, `worktree?`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` | state, dirty, worktree | decided: delete workspace |
| `workspace.rules` | agent |  | `workspace?`, `agent?` | `{rules, text_bytes, limit_bytes}` | `not_found`, `bad_request` |  | not cached |
| `workspace.rule.set` | owner | yes | `workspace?`, `key`, `value`, `request_id` | `Accepted` | `not_found`, `bad_request` |  | decided: set workspace rule |
| `team.get` | agent |  | none | `{agents, people, workspaces}` |  |  | `team`, snapshot |
| `proposal.list` | agent |  | `state?` | `{proposals} & ReadCursor` | `bad_request` |  | `proposals`, fed by proposals |
| `proposal.decide` | member | yes | `proposal`, `decision`, `note?`, `subject?`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` | proposal_changed, proposal_settled | decided: decide |
| `proposal.apply_now` | member | yes | `proposal`, `request_id` | `Accepted` | `refused`, `not_found` | proposal_settled | decided: apply now |
| `proposal.stop` | member | yes | `proposal`, `request_id` | `Accepted` | `refused`, `not_found` |  | decided: stop |
| `schedules.list` | agent |  | none | `{schedules, errors}` |  |  | `schedules`, snapshot |
| `schedule.add` | owner | yes | `target`, `cron`, `message`, `request_id` | `Accepted & {schedule}` | `refused`, `bad_request`, `not_found` |  | authored: add schedule |
| `schedule.edit` | owner | yes | `schedule`, `target?`, `cron?`, `message?`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | authored: edit schedule |
| `schedule.remove` | owner | yes | `schedule`, `request_id` | `Accepted` | `refused`, `not_found` |  | authored: remove schedule |

### `proposal.propose`

Propose a roster change; the founder decides (hl propose add, retire, rename, team, settings, context, schedule, run (a shell command the founder's approve runs in a terminal window; at most 5 open per agent, the sixth refused), and diff, which is kind bundle: args.changes lists several changes decided once and applied together or not at all).

### `proposal.withdraw`

Take back an undecided proposal: its author's own, or any for the founder; another agent is refused (gate role).

### `trust.propose`

Propose declaring a folder Handline may answer Claude's trust prompt in (hl trust add); the founder's own call declares it. Called by an agent it raises a decision for the founder, `why` required, and answers the proposal's id; called by the founder it declares the root directly, `why` optional (default `added by the founder`), and answers no proposal. A root already inside a declared one, a missing why or a decision that could not be raised is refused with the store's words. A paired device is refused, gate role. `root` must be an absolute path (bad_request, field `root`): the daemon would read a relative one, or `~`, against its own folder and home, so the client resolves it against its own.

### `trust.list`

Declared trust roots, and with proposals the proposals (hl trust list, hl trust proposals).

### `trust.remove`

Drop a declared root (hl trust remove); needs no approval. The founder's: the declared roots live in the protected store, which an agent session cannot write, so an agent calling `hl trust remove` is refused today and by this method (gate role). A root that is not declared is not_found. `root` must be an absolute path, as for trust.propose (bad_request, field `root`).

### `people.list`

Every person, so a client writes out a person key by name.

### `workspaces.list`

Every workspace, so a client writes out a workspace key by name and groups agents by it.

### `workspace.create`

Make a workspace on the top of the git checkout folder is in, with its lead (reporting to the owner, its rules seeded), and start the lead: one operation, answered as agent.create's. Refused before any operation starts: a folder in no checkout (gate not_git), a missing one (not_found, what folder), a name a workspace has (gate state). Owner only; an agent proposes a team instead. The operation has kind workspace.create and its agent is the workspace's name (its lead); its steps and result are agent.create's. A folder another workspace's shares, or a name the rules refuse, fails it with the reason. A start that fails leaves the workspace and its lead registered, so its name is in use: start the lead with session.start (hl wake), or remove it with workspace.delete.

### `workspace.set`

Rename a workspace: its name is the one thing this changes. Keys never change; the dotted form of each agent's name (workspace.agent) follows the new name, and the old one no longer resolves. Refused (gate state) while one of its agents has a session running, while an open proposal names one by its dotted name, or for a name the rules refuse or a workspace or person's handle has. The same name is no change. Owner only.

### `workspace.delete`

Delete a workspace: each of its agents is retired as agent.retire retires one, its lead last, and the workspace goes with the lead. The project folder is never touched. Nothing is done when any agent has a session running or an agent of another workspace under it (gate state), a worktree holds work with no worktree answer (gate dirty, every agent's in worktrees), or a worktree cannot go (gate worktree). While it runs, agent.create and the session methods in the workspace are refused (gate state), and it is refused while one of them is under way. Git or the disk failing part way puts back every retire before it, but not a worktree already removed; the call is refused (gate state) naming the step. Owner only.

### `workspace.rules`

The kept rules in force for a workspace, its own beating the machine default's, or with no workspace the machine default's alone. A rule set to 0, empty or false is listed (it still beats the level below) with on false. A rule never set on either is not listed: it is off.

### `workspace.rule.set`

Set or clear a kept rule on a workspace, or with no workspace the machine default. A workspace's value beats the machine default, which beats the built-in: no rule. Agents of the workspace (every agent, for the machine default) are told the rules in force under their team's rules text. An unknown key, a value that is not the key's kind, an account no login has or a lease kind there is none of is bad_request. Owner only.

### `team.get`

The caller's team: the agents of its workspace (all of them for a person outside every workspace folder, as the app is), the people, and the other workspaces. Each name is the word the caller uses for that key.

### `proposal.list`

proposals with their diff: what waits for the founder, and what settled recently. Without `state`: every proposal that waits (open, applying, and apply_failed until it is decided again) and every settled one (approved, rejected, withdrawn) from the last 7 days, at most 50 of those. Order: the waiting ones first, oldest `created` first; then the settled ones, newest `settled` first. A client counts the dot from `open` and `apply_failed`, never from `applying`. With `state`, only that state, in the same order, and no 7-day or 50 bound. A proposal whose requested_by is human (the founder's own change) is listed as any other, so a held one shows while it waits. The proposal file's states map as: proposed = open; approved = apply_failed (it holds the founder's approval and the change is not applied: `error` is set when an apply ran and failed, null when it never ran, e.g. a crash between the two); applied = approved; rejected = rejected; withdrawn = withdrawn; enacting and failed are never listed.

### `proposal.decide`

Approve or reject a proposal; approving an apply_failed one applies it again. The call answers only with the outcome. Approving a change to an agent with a session running holds it (the proposal turns applying) and the call waits until those turns end and the change is applied, then answers what was done, or the refusal; other calls on the connection are answered meanwhile. Past 10 minutes the held change is dropped (withdrawn), nothing is changed and the call is refused (gate state). Refused with gate proposal_changed or proposal_settled, carrying the proposal as it is; an applying proposal is refused proposal_settled.

### `proposal.apply_now`

Apply a waiting proposal now: for an applying proposal, interrupt the turn of each agent in `waiting_on` (as session.interrupt does), then close the session, apply the change and restart the renamed agent at the next tick, without waiting for the turn to end. The call answers once the change is applied, with what was done, or the refusal; past 10 minutes the held change is dropped (withdrawn) and the call is refused (gate state). Refused with gate proposal_settled, carrying the proposal as it is, unless the proposal is applying. Works for every kind a running session can hold: rename, retire, context, link, settings.

### `proposal.stop`

The founder stops the running command of an approved run proposal: its window is closed and the run ends stopped; the proposing agent is told. Refused (gate role) for anyone else, and (gate state) when the proposal is not kind run, its command is not running, or its window is no longer the command's.

### `schedules.list`

Schedules. An agent whose `schedules` value is bad is listed in `errors` (target and the error text), not shown as an agent with no schedules.

### `schedule.add`

Add a schedule.

### `schedule.edit`

Edit a schedule.

### `schedule.remove`

Remove a schedule.

