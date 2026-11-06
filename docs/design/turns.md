# Turns: who acts on a task now

A task keeps its owner and carries a stack of turns on top: the top acts now, and when it finishes its turn comes off and the one below is told.

## Why

- **Waiting on the founder takes a slot.** An agent may not hand a task to `human`, so it asks with an `@human` comment and keeps the task doing. `task.time` reads that ask as waiting on the founder, but the scheduler does not: `TaskWaits` counts only blockers and children, so the slot stays taken, the time box and the session-closed sweep alarm the agent, and its queue never starts.
- **Review is a handoff with a manual return.** The reviewer must hand the task back, so builders open a separate audit task instead; most of a request's subtasks are audits and re-audits.
- Gerrit's attention set and GitHub's review requests keep the owner fixed and move only whose turn it is; a reply moves it back by itself. Turns do the same.

## Model

- `owner` (the `tasks.owner` column): the agent the task is assigned to, accountable for its cost and history.
- `turns`: a stack above it, bottom first, rows `{agent, ask, by, since, on_pass, evidence}` in a `task_turns` table, the truth; `task_turn_push` and `task_turn_pop` events are written in the same transaction as history. Empty for most tasks. At most three turns.
- **acting**: the top turn's agent, or the owner when there is none. One agent appears once in the stack and the owner.

Every rule that asks who works the task now reads acting instead of `owner`: delivery and `isCurrent`, `HoldsCurrent`/`wayFree`/`HeldByHolder` (the doing slot), `HolderOf` and the time box with who it tells, `SweepHeldTaskAfterClose` and its hand-on to `answerer`, the idle nudge, `my_turn` and `requests.pending`, task.board's "who holds it now", the assignee that task.comment.add and task.comment.recipients tell, task.draft's "holder working the task", task.wait's return to the holder's queue. `AnswersTo` (reporter, else opener) does not change.

An agent below the top acts on nothing: its slot is free and its next queued task starts. When acting is `human`, `HolderOf` is `FounderHolds`, so no agent is alarmed and the time box restarts when the turn ends, as for a task the founder owns today.

## Writes

- `task.turn.push {task, to, ask, on_pass?, evidence?}`: acting (or the founder) puts `to` on top. `on_pass: "done"` with `evidence` closes the task when the turn passes, for review of finished work. Refused: `to` already in the stack or the owner, a fourth turn, an ended task.
- `task.turn.pop {task, outcome, note?}`: by acting or the founder. `pass`, or `send_back` with a note. The agent below gets the task at the front of its queue with the note as a comment. With `on_pass: "done"` a pass closes the task through the usual done gates; if a gate refuses (open children), the turn still comes off and the refusal is the note.
- **The founder's turn** ends when acting is `human` and the founder comments on the task (a pass, the comment its note) or presses Return (a pass, no note). Reading it ends nothing. A founder comment while an agent is on top is a comment only.
- `task.handoff` and `task.edit owner` replace acting: the top turn if there is one, else the owner. An agent's handoff or edit to `human` is a push of `human`, no longer refused; `task.new` with owner `human` stays refused for agents.
- `task.release` by the top agent is a `send_back` with its reason; by the owner with turns open it is refused (pop first). `task.draft` and `task.discard` clear the stack. `task.done` by acting, the owner or the founder clears the stack and tells the agents in it. `task.reopen` goes to the owner with an empty stack.
- `blocked_by` and `task.wait` block the task whoever acts; the acting agent's slot is the one freed.
- Retiring an agent pops each turn it holds as `send_back`, note "retired".
- `@human` in a comment stays a question that does not stop the work: it opens an ask, which `task.time` reads as waiting on the founder, and the slot stays taken. To wait on the answer, push `human`.

## Reads

- `Task` gains `turns: Turn[]` and `acting: string`; `tasks.list` gains `acting?`.
- `RequestRow.attention` `bounced` counts `send_back` pops beside reopens while audit tasks remain.
- A row shows acting and the chain below: `T8503 · founder ← handline.ui`.

## Rules for agents

- Cannot go on without the founder's decision: push `human` with the decision and a recommendation in `ask`, then take the next task or stop.
- A question that does not stop the work: an `@human` comment.
- Finished work that needs review: push the reviewer with `on_pass: "done"` and the evidence. The reviewer passes or sends back; nobody opens a fix task for the same work.

## Phases

1. **The founder's turn**: the table, push and pop, acting in every rule above, handoff and edit to `human` as a push, the founder's comment and Return as the pop. It lands with a sweep of every text an agent reads about holding, handing on and asking, so no old rule is left beside the new one: the team base rules, AGENTS.md, ROLE.md files, `hl` help and refusals, the daemon's notices (time box, session closed, idle, handoff), and the contract's method text.
2. **Reviews**: builders push the audit agents and they pop, instead of audit tasks. `Task.role` and check badges fold away when no audit task is open.
3. **Clean-up**: drop "a review is a handoff" from the contract, and `Task.role` with its `team.yaml` field.

Not now: several reviewers at once (a stack has one top); an agent's question to another agent as a push, allowed but not yet asked for.
