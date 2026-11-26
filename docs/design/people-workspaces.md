# People, workspaces and agents by key

Everything that names a who is stored as a key that never changes, so a rename moves nothing, and people stay free to name things.
Only personal mode is built: one person, the owner. The person columns are there so team mode adds rows, not columns.

## Model

| Entity | Key | Fields | Unique |
|---|---|---|---|
| person | `p<n>` | display name, handle (empty in personal mode), role `owner` or `member` | handle |
| workspace | `w<n>` | name, project folder, added folders, lead agent | name |
| agent | `a<n>` | workspace, name, owner (a person), settings | name, within its workspace |
| account | `c<n>` | name, label, owner (a person), paused, month cycle | name |
| device | `d<n>` | person, name, paired at | - |

- The rows live in `db.sqlite` (`go/kit/store/migrations/0100_base.sql`), the one truth. An agent's role and standing context and a
  workspace's rules are versions in the `instructions` table, not files.
- A key is the kind letter plus the row id (`go/kit/actor`). Ids are never reused. Every column that names a who holds a key as `TEXT`.
- Code compares keys, never names. `actor.OwnerKey` is the owner where the daemon names the person with no caller in hand. `sys`
  (the daemon's own sender) and `gate` (the owner of a task that watches outside state) are reserved keys.
- First start makes the owner `p1`: display name from the Mac account's full name, no handle, no sign-up.

## Names

`names.Check(kind, word, workspace)` holds the rules, and the app and `hl` show its refusal as it is:

- Lowercase letters, digits and `-`, 1 to 32 long, starting with a letter; nothing that reads as a key (`a17`).
- Reserved: `human`, `me`, `all`, `everyone`, `sys`, `gate`.
- A workspace name or a handle may equal no agent name anywhere, and no other workspace name or handle. An agent name is unique in its
  workspace and may repeat across workspaces.
- A new workspace's first agent is its lead, named `lead`.

## Addresses

`addr.Resolve(caller, word)` turns a word into a key, in this order: `human` (the caller's owner, or the task's person), a key that
names a row, an agent of the caller's workspace, `workspace/agent`, a workspace (its lead receives it), an agent whose name no other
agent has, a handle. Anything else is refused with the names the caller may use. `hl say`, `--assign`, `hl task handoff`, `--turn` and
comment mentions all take these words. From hl, another agent's key is refused with its name to use, except in `hl launch <key>`,
which the daemon itself types; the app sends keys.

Routing follows the same lines: an agent may reach every agent of its workspace, its owner and any workspace's lead; the founder may
reach anyone. `addr.Name` writes a key out as the word the reader would use: an agent of another workspace whose name another agent
also has as `workspace/name`, the reader's owner as `human`.

## Mentions

On write, each `@word` that resolves for the writer is stored as `<@key>`; one that does not resolve stays text. On read, `<@key>` is
written out for the reader. A rename therefore changes no stored text.

## Who is calling

`rpc.Caller` is `{Kind: person|agent|device|anonymous, Key, Person, Role}`. A person's writes are stored under their key, a device's
under its person's key. Method roles are `agent`, `member` and `owner`; a paired device has its person's role. Proofs: an agent by its
process lineage, the owner by the founder key on this Mac, a device by its pairing.

## Disk

Folders under the home are named by key, so Claude's project history, keyed by working directory, survives a rename:
`agents/<a-key>/`, `workspaces/<w-key>/`, `caches/<a-key>/`. An account's folder, `accounts/<folder>/` (its Claude config dir), is a
random hex name its row records (`accounts.folder`); an account whose row records none has its key, `accounts/<c-key>/`. The folder
never moves: the Keychain keeps a login under its path. An agent works in
a desk (worktree) of its workspace's project folder. `HANDLINE_AGENT` carries its name; hl looks the key up.

## Contract

Every who field carries a key. `people.list`, `workspaces.list` and `agents.list` give the words; the app caches them and writes out
every key through them. Routes and tab ids use keys, so open tabs survive a rename.

## Workspace rules

- **Text rules**: the workspace's `rules` instruction (at most 2 KB, `team.RulesTextLimit`) reaches each of its agents in the system prompt at launch; a running session is
  told the changed lines (`lifecycle.RulesNote`).
- **Kept rules** (`go/data/team/keptrules.go`): rows of `workspace_settings(workspace, key, value)`, set by the owner with
  `workspace.rule.set`, read with `workspace.rules`. A workspace's value beats the machine default (workspace 0), which beats no rule.
  They render as a "Kept rules" line under the text, so the same note tells a running agent of a change. A refusal names the key, its
  value and who set it. `0`, empty or false is no rule.

| Key | Meaning |
|---|---|
| `accounts` | accounts the workspace's agents may run on (empty = any) |
| `agent_cap` | most agents of the workspace mid-turn at once |
| `tools_deny` | tools its agents may not use, added to the global list (at next session start) |
| `mode_max` | strongest mode an agent may be given: `plan` < `default` < `acceptEdits` < `auto` (at next session start) |
| `review_required` | a done with commits needs a turn answered by an agent that did not work the task |
| `land_by` | the one agent that may push to a repo's default branch |
| `land_check` | a command that must pass before such a push |

The land guard is advisory: the PreToolUse hook reads a Bash `git push` that updates the default branch and runs `land_check` in the
agent's folder first. It does not see a push wrapped in a script or another way to land, and lets the push through when the daemon
does not answer.

## Resource leases

- A shared run (`go/scheduler/runs`) is a name, the commands it covers (a program, a program and its first words, a script path, or a
  preset `@build-test`, `@simulator`, `@browser`), how many may run at once, and a scope: This Mac (workspace 0) or one workspace. It is
  a `workspace_settings` row under `run.<name>`. An agent adds or removes one for This Mac or its own workspace with
  `hl use add|remove|list`, but not one the founder set; the founder uses `run.set`, `run.remove`, `run.list`. The PreToolUse hook
  refuses a covered Bash command outside `hl use <name>`.
- Leases (`go/scheduler/leases`): a shared run's kind is `scope:name`; `port` is the one fixed kind, a free port given as `$HL_PORT`.
  `hl use a b -- cmd` asks in a fixed order and gives all back together. A freed lease goes to the workspace that held the kind least in
  the last hour, then first come first served.
- With no lease free, `hl use` exits 75: the ask is queued, the agent's task is blocked on it and comes back when the lease is reserved
  for it. A reservation not claimed within 10 minutes goes to the next ask.
- A lease ends when its command exits, on `--release`, at turn or session end, or at its `max_hold` (`<kind>_max_hold_min`; 30 minutes
  for a shared run, none for a port) when someone waits. `hl use --extend` adds a period, up to twice. A revoked command gets TERM, then
  KILL after 10 s. Leases are stored, so a restart keeps what is held.
