# Turns: who acts on a task now

A task keeps its owner and carries a stack of turns on top: the top acts now, and when its turn comes off the one below is told.
Waiting on the founder or on a reviewer therefore frees the owner's slot, and a reply moves the task back by itself.

## Model

- `owner` (`tasks.owner`): the agent the task is assigned to, accountable for its cost and history.
- turns: a stack above it, bottom first, rows `{agent, ask, pushed_by, since, on_pass, evidence, memo}` in `task_turns`, the truth;
  `task_turn_push`, `task_turn_pop` and `task_turn_ask` events are its history, written in the same transaction. At most three
  (`db.MaxTurns`). One agent appears once in the stack and the owner.
- **acting** (`db.ActingSQL`): the top turn's agent, or the owner when there is none. Every rule that asks who works the task now
  (delivery, the doing slot, the time box, the idle nudge, `requests.pending`, who a comment tells) reads acting, not the owner.
- An agent below the top acts on nothing: its slot is free and its next queued task starts. When the founder is on top, no agent is
  alarmed and the time box restarts when the turn ends.

## Writes

- `task.comment.add {task, text, turn, done?}` (`hl task comment T<n> add "<text>" --turn <to> [--done "<evidence>"]`): acting, or the
  founder, adds the comment and puts `to` on top in one step. The comment is the turn's message (`memo`). With `done`, a pass closes
  the task (`on_pass: "done"`), for review of finished work. Refused: `to` already in the stack or the owner, a fourth turn, an ended task.
- `task.turn.pop {task, outcome, note?}` (`hl task pass T<n> ["<note>"]`, `hl task back T<n> "<note>"`): by acting or the founder.
  The one below gets the task at the front of its queue, with the note as a comment. A pass of an `on_pass: "done"` turn closes the task
  through the usual done gates; if a gate refuses, the turn still comes off and the refusal is added to the note.
- **The founder's turn** ends when the founder comments on the task (a pass, the comment its note). Reading it ends nothing. A founder
  comment while an agent is on top is a comment only.
- The founder's turn is the task's one question slot: a new `--turn human` comment while it is on top moves the turn to that comment
  (`task_turn_ask` keeps the old text). A comment whose first word is `@human` is news: it moves no turn, and `requests.pending` lists it
  as a note until the founder reads the task.
- `task.handoff` and `task.edit owner` replace acting: the top turn if there is one, else the owner. An agent's handoff to `human` is a
  push of the founder's turn; `task.new` with owner `human` stays refused for agents.
- `task.release` by the top agent is a send-back with its reason; by anyone else with turns open it is refused.
- `task.done`, `task.draft`, `task.discard` and `task.reopen` clear the stack and tell the agents in it.
- `blocked_by` and `task.wait` block the task whoever acts; the acting agent's slot is the one freed.
- Retiring an agent takes off each turn it holds: one on top is sent back with the note "retired", one lower down just goes.

## Reads

`Task` carries `turns` and `acting`; `tasks.list` takes `acting`. A row shows acting and the chain below it.
