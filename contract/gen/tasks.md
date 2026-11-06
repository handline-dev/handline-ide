<!-- Generated from contract/spec/ by contract/generate.py; do not edit. -->
# tasks methods (protocol 1)

A method marked write takes a required `request_id`. Framing, roles, errors and gates are in [protocol.md](../protocol.md); field types in `protocol.ts`.

| Method | Role | Write | Params | Result | Errors | Gates | App |
|---|---|---|---|---|---|---|---|
| `tasks.list` | agent |  | `owner?`, `acting?`, `reporter?`, `from?`, `state?`, `tags?`, `parent?`, `under?`, `request?`, `ids?`, `before?`, `order?`, `cursor?`, `limit?`, `all?`, `everyone?`, `my_turn?`, `reason_gaps?` | `{items, more, reason_gaps, next} & ReadCursor` |  |  | `tasks`, fed by tasks |
| `task.get` | agent |  | `task` | `TaskDetail & ReadCursor` | `not_found` |  | `task` by task, fed by tasks |
| `task.history` | agent |  | `task` | `{task, rows}` | `not_found`, `bad_request` |  | `task-history` by task, snapshot |
| `task.time` | agent |  | `task` | `{task, start, end, segments, estimate, expected} & ReadCursor` | `not_found`, `bad_request` |  | `task-time` by task, fed by tasks |
| `task.shot` | member |  | `task`, `n`, `offset?` | `{mime, bytes, offset, data, done}` | `not_found`, `bad_request` |  | not cached |
| `task.new` | agent | yes | `owner`, `reporter?`, `title`, `body?`, `time_box?`, `parent?`, `no_parent?`, `duplicate_ok?`, `blocked_by?`, `watch?`, `priority?`, `estimate?`, `tags?`, `expects?`, `ui?`, `request_id` | `Accepted & {task}` | `refused` | duplicate_title | authored: new task |
| `task.handoff` | agent | yes | `task`, `to`, `note?`, `request_id` | `Accepted` | `refused`, `not_found` |  | not from the app |
| `task.turn.push` | agent | yes | `task`, `to`, `ask?`, `on_pass?`, `evidence?`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | not from the app |
| `task.turn.pop` | agent | yes | `task`, `outcome`, `note?`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | decided: give back |
| `task.done` | agent | yes | `task`, `evidence?`, `request?`, `repo?`, `reason?`, `commit?`, `shots?`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` | open_children | authored: done |
| `task.reopen` | agent | yes | `task`, `note?`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | authored: reopen |
| `task.progress` | agent (only agent) | yes | `task?`, `note`, `request_id` | `Accepted` | `refused`, `not_found` | not_self | not from the app |
| `task.begin` | agent | yes | `title`, `body?`, `message`, `priority?`, `expects?`, `why?`, `request_id` | `Accepted & {task}` | `refused` |  | not from the app |
| `task.start` | agent | yes | `task`, `request_id` | `Accepted` | `refused`, `not_found` |  | not from the app |
| `task.seen` | member | yes | `task`, `upto?`, `request_id` | `Accepted` | `not_found`, `bad_request` |  | decided: mark seen |
| `requests.pending` | member |  | none | `{answer, comments, counts} & ReadCursor` |  |  | `requests-pending`, fed by tasks |
| `task.comment.list` | agent |  | `task`, `before?`, `limit?` | `{memos, more}` | `not_found` |  | `memos` by task, snapshot |
| `task.memo.list` | agent |  | `task`, `before?`, `limit?` | `{memos, more}` | `not_found` |  | not cached |
| `task.comment.add` | agent | yes | `task`, `text`, `now?`, `skip?`, `reply?`, `files?`, `request_id` | `Accepted & {memo}` | `refused`, `not_found`, `bad_request` |  | authored: add comment |
| `task.comment.recipients` | agent |  | `task`, `text`, `now?` | `{recipients}` | `not_found`, `bad_request` |  | `comment-recipients`, fed by tasks |
| `task.memo.add` | agent | yes | `task`, `text`, `now?`, `skip?`, `files?`, `request_id` | `Accepted & {memo}` | `refused`, `not_found`, `bad_request` |  | not from the app |
| `task.comment.edit` | agent | yes | `task`, `memo`, `text`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | authored: edit comment |
| `task.memo.edit` | agent | yes | `task`, `memo`, `text`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | not from the app |
| `task.comment.delete` | agent | yes | `task`, `memo`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | authored: delete comment |
| `task.comment.send_now` | member | yes | `memo`, `agent`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | decided: send now |
| `task.send_now` | member | yes | `task`, `request_id` | `Accepted` | `refused`, `not_found` |  | decided: run this one now |
| `task.memo.delete` | agent | yes | `task`, `memo`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | not from the app |
| `task.view.list` | member |  | none | `{views}` |  |  | `task-views`, snapshot |
| `task.view.save` | member | yes | `name`, `filter`, `request_id` | `Accepted` | `bad_request` |  | decided: save view |
| `task.view.delete` | member | yes | `name`, `request_id` | `Accepted` | `not_found`, `bad_request` |  | decided: delete view |
| `task.queue` | agent | yes | `tasks`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | authored: queue |
| `task.draft` | agent | yes | `tasks`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | authored: draft |
| `task.merge` | agent | yes | `into`, `tasks`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | authored: merge |
| `task.release` | agent | yes | `task`, `to?`, `reason?`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | authored: release |
| `task.request` | agent | yes | `request`, `title?`, `owner?`, `request_id` | `Accepted & {task, existing}` | `refused`, `not_found`, `bad_request` |  | decided: make a request |
| `task.none` | agent | yes | `request`, `reason`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | decided: mark no work |
| `task.wait` | agent | yes | `task`, `watch`, `time_box?`, `request_id` | `Accepted & {gate}` | `refused`, `not_found`, `bad_request` |  | not from the app |
| `task.discard` | agent | yes | `task?`, `tasks?`, `reason?`, `reasons?`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | decided: discard |
| `task.board` | agent |  | `everyone?` | `{mine, reports}` |  |  | `board`, fed by tasks |
| `task.withdraw` | agent | yes | `task`, `request_id` | `Accepted` | `refused`, `not_found` |  | authored: withdraw |
| `task.cancel` | member | yes | `task`, `reason?`, `request_id` | `Accepted` | `refused`, `not_found` |  | authored: cancel |
| `task.edit` | agent | yes | `task`, `owner?`, `reporter?`, `title?`, `body?`, `evidence?`, `time_box?`, `priority?`, `estimate?`, `blocked_by?`, `unblock?`, `add_tags?`, `remove_tags?`, `request_id`, `pr?` | `Accepted` | `refused`, `not_found` |  | authored: edit |
| `task.ask` | member | yes | `task`, `to`, `text`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | decided: ask |

### `tasks.list`

Task rows. Newest id first. To read every task, call with `all: true` and `limit: 200`, then again with `before` set to the smallest `id` of the page before, until `more` is false: nothing is skipped or repeated, because a page is the rows with an id below `before`. The caller's own scope still applies (itself and its reports for an agent, unless `everyone`).

### `task.get`

One task with body, delivery history and linked events.

### `task.history`

Every change to a task, newest first: one row per field changed, with who made it, the value before and after, and when. It is read from the events each task write keeps in its own transaction, so it is never edited or deleted; a fix is a later row. A deleted memo's words stay in it. Anyone may edit a task (task.edit); this history is what makes that safe to undo by hand.

### `task.time`

Where a task's time went, from its opening to its end (or now): segments in order that cover it with no gap or overlap. Read from what the daemon already records (the task's state events, its blockers, its asks of the founder, its holder's parks and starts); nothing is timed twice. Where two causes overlap, the stretch shows one, in this order: waiting on the founder, blocked by a task, waiting on another agent, queued, at an account's wall, starting, working.

### `task.shot`

A screenshot kept with a finished task. One screenshot of a task whose end recorded it, in chunks. A task or number the result does not hold is not_found.

### `task.new`

Create a task. The founder or a member device opens one for an agent only, and it opens queued. An agent owning it to `human` is refused as task.handoff does; a task begun from the founder's message answers to the founder (task.begin). The owner may not be the caller: a task an agent takes on itself is begun from the founder's message (task.begin). An agent may not open a task owned by human: the task is the agent's, and the founder's decision is a turn on it (task.turn.push human).

### `task.handoff`

Hand a task to another agent. Anyone may hand an open task on, not only its owner (the asker takes a task back so). It moves who acts on the task: its owner, or with turns open (task.turn.push) the agent of its top turn; it is refused to an agent that is the owner below the turns or already has a turn on it. task.history's row names who made the move (by). An agent's handoff to `human` is task.turn.push to human with the note as its ask: the task stays the agent's and waits on the founder. The founder may hand a task to `human`.

### `task.turn.push`

Put another agent, or the founder, on top of a task you act on. By the agent acting on the task (Task.acting) or the founder. `to` goes on top and acts on the task now; the one below keeps its turn but acts on nothing, so its doing slot frees and its next queued task starts. An agent is delivered the task with the ask; human is not delivered to: the task waits on the founder (requests.pending, my_turn), no agent is alarmed and its time box starts again when the turn ends. Refused: `to` is the owner or already has a turn, the task carries 3 turns, or the task is ended.

### `task.turn.pop`

End the top turn on a task: pass, or send it back with why. By the agent of the top turn or the founder. The turn comes off and the one below acts again: an agent gets the task at the front of its queue, with the note as a comment. A pass of a turn pushed with on_pass done closes the task with its evidence through the usual done gates instead; when a gate refuses (open children), the turn still comes off and the refusal is added to the note. While human is on top, the founder's comment on the task (task.comment.add) is a pass with the comment as its note, and the founder's give back is a pass with none.

### `task.done`

Finish a task with evidence. Exactly one of evidence, request or reason-with-evidence; none or two is bad_request. A commit and shots are optional, for any task; when given they are kept with the task (task.get result). A done by the assignee, the owner (who the task answers to, the opener unless another is named) or the founder closes the task: nothing waits on an accept. The founder's done with evidence words on a task they own whose card still waits on them decides that card with the words, so the asker is told and what waits on the answer (a trust proposal) is applied; it says `T<n> decided`. The agent acting on the task (Task.acting) may close it as its assignee may. A done clears its turns and tells the agents that held them.

### `task.reopen`

Reopen a done task: anyone may, with a reason the opener and assignee are told. A discarded task comes back as a draft, to be queued again. A reopened task goes to its owner with no turns.

### `task.progress`

Record progress (hl progress): on a task, or with no task on the caller's request in progress, else the task it holds, else the agent, even when it holds no task.

### `task.begin`

Start a task the founder told the caller to do, without approval: opened for the caller, linked to the founder's message, and answering to the founder (its reporter is human).

### `task.start`

Pick up a task as the caller's current one.

### `task.seen`

Record that the founder has a task's comments in front of them, up to a comment number. What it moves: the founder's read marker for the task (a named cursor, task_seen:<person>:<task>), which only goes forward. Task.unread and Task.first_unread count the comments above it, and requests.pending drops a task whose comments are all read. It also marks read the inbox lines that announced those comments, so the phone's counts agree. It never reads away something that is waiting for an answer (a question to the founder, a decision, a permission request): those end only by answering. A task with no marker counts from a floor set once when the daemon first ran this version, so comments from before it do not show as new. A client calls it when the task page opens, and again when a new comment arrives while the page is open and in view. It sends the tasks feed event, so every device updates.

### `requests.pending`

What waits on the founder, grouped by task, and the counts the sidebar badges show. A task the founder has read (task.seen) drops out on the next read; a client that wants the open row to stay while the page is open keeps the row it opened. Refetch on the tasks and inbox feed events.

### `task.comment.list`

Comments beside a task. The wire still calls a comment a memo (Memo, `memo`, `memos`); task.memo.list is its old name.

### `task.memo.list`

The old name of task.comment.list: the same params, result and errors. Kept for clients that call it.

### `task.comment.add`

Add a comment; anyone may. It is told only to an agent that has to act on it: the task's assignee (its holder), each agent it names as @name, and, when the founder wrote it, who the task answers to (its reporter, else its opener); with now every one of those readers, mid-turn too. It is never told to its writer, and a comment that names no one is told to the assignee only. task.comment.recipients says who will be told before posting; skip leaves some of them out. The founder is told by one inbox line per request (a task made from their message, task.request) while it stays unread, counting the agents' new comments. A reader at its prompt is told at once and a reader mid-turn at the end of its turn. comment.edit and comment.delete are told the same way. A comment by an agent that names `@human` asks the founder, as the description says. The wire still calls a comment a memo (Memo, `memo`, `memos`); task.memo.add is its old name. A comment by an agent that names `@human` asks the founder: it opens one ask to the founder, linked to the task (InboxItem kind ask, `task` and `memo` set), so the task reads waiting on the founder. The ask stays open until (1) the founder replies to it (inbox.reply), (2) the founder adds any comment to that task, (3) the founder marks it read (inbox.mark_read naming its id), (4) its comment is deleted or edited so it no longer names `@human`, or (5) the task is done or discarded. An edit that keeps `@human` keeps the one ask and updates its body; one that adds `@human` opens it. While human is the top turn (Task.acting), the founder's comment passes that turn, the comment its note (task.turn.pop). An @human comment by an agent asks without stopping the work: it does not free the agent's slot; to wait on the founder, task.turn.push human.

### `task.comment.recipients`

Who a comment would be told to, before it is posted: the rule task.comment.add uses, computed without writing anything. Given the task, the comment text, and whether it would be sent now, returns one row per agent task.comment.add would tell, each with why: `asked` (the text names it as @name), `assignee` (the task's holder, told of every comment) or `owner` (the founder's comment, to who the task answers to). An agent that fits several is named once, by the first of asked, assignee, owner. The caller is the writer and is never listed; one that names no one returns the holder only. Passing the rows' agents as skip to task.comment.add tells exactly the others.

### `task.memo.add`

The old name of task.comment.add: the same params, result and errors. Kept for clients that call it.

### `task.comment.edit`

Edit a comment: its author or the founder. An edit that drops `@human` withdraws the ask it opened (task.comment.add); one that adds `@human` opens one. The wire still calls a comment a memo (Memo, `memo`, `memos`); task.memo.edit is its old name.

### `task.memo.edit`

The old name of task.comment.edit: the same params, result and errors. Kept for clients that call it.

### `task.comment.delete`

Delete a comment: its author or the founder. Deleting a comment withdraws the ask it opened (task.comment.add). The wire still calls a comment a memo (Memo, `memo`, `memos`); task.memo.delete is its old name.

### `task.comment.send_now`

Send a founder's comment now to a reader whose tell waits for the reader's next digest. Refused when the tell is not held (already sent, read or never told).

### `task.send_now`

While new work is paused (daemon.pause), give this one queued task to its agent now, as one the founder sent: it is told, and its agent starts or wakes for it as for a person's message. The pause stays and holds the rest. Refused when nothing is paused, or when the pause is not what holds the task: it is not queued, not for an agent, or already told.

### `task.memo.delete`

The old name of task.comment.delete: the same params, result and errors. Kept for clients that call it.

### `task.view.list`

The saved views of the task board, by name.

### `task.view.save`

Keep a filter of the task board under a name, replacing a view already kept under it.

### `task.view.delete`

Delete a saved view.

### `task.queue`

Approve draft tasks to run: anyone may; each task's opener and assignee are told.

### `task.draft`

Send queued or doing tasks back to draft: anyone may; each task's opener and assignee are told. A holder working the task is told to stop and moved on to its next task, and commits or changes it left in its worktree are kept in a memo on the task. A draft clears the task's turns, telling the agents that held them: it goes back to its owner.

### `task.merge`

Fold tasks into one: the kept task takes their titles, requests and sub-tasks; anyone may, and the opener and assignee of each are told.

### `task.release`

Take a doing task back to queued or draft: anyone may, with a reason the opener and assignee are told (and the founder, for a task begun on their message). With turns open (Task.turns): by the top turn's agent or the founder it is a send_back of that turn with the reason (task.turn.pop); by anyone else, the owner too, it is refused: the top turn is popped first.

### `task.request`

Make a founder's chat message a request: a task the founder opened (opened_by human), owned by the lead the message was sent to. The message becomes its first comment, images included. A message converts once; a second call returns the task already made from it. The founder, or the agent the message was sent to, may call it. Clears a no-work mark on the message.

### `task.none`

Mark a founder's chat message as needing no work, with a reason in one line, so it is not chased as unanswered. Refused for a message already made a request. The founder, or the agent the message was sent to, may call it.

### `task.wait`

Make a task wait on outside state: a gate on watch (as task.new with owner gate), the task blocked by it and, when doing, released, all in one write, so the task never waits on a gate that blocks nothing. The task comes back to its holder's queue when the gate opens. The wait blocks the task whoever acts on it: the slot freed is the acting agent's (Task.acting).

### `task.discard`

Throw away a task that is not done: anyone may, with a reason the opener, assignee and asker are told. A request still open on it refuses, unless the caller leads a team whose tree holds the task (its owner or opener is the lead or reports to it): then each open request is withdrawn and its target told. tasks in place of task discards a batch in one call, all or none, with one discard_batch event: the one task_batch card in the caller's chat. A request that already landed in its agent's session does not refuse the task's asker or the founder: the task is discarded, that request closes with it and its holder is told why; anyone else is refused. A discard clears the task's turns and tells the agents that held them.

### `task.board`

The caller's open tasks and its reports', and who holds each now. The founder's board is the tasks the founder owns.

### `task.withdraw`

Withdraw a task the caller asked for; anyone else discards it with a reason.

### `task.cancel`

Cancel a task.

### `task.edit`

Change a task: owner (an agent giving it to `human` is refused, as task.handoff does), reporter, title, priority, body, result and time box, by anyone (the body at any state, a closed task included; the time box until it is delivered), except the result, which only the founder changes. Waiting on other tasks is added (blocked_by) or removed (unblock) with it. Never silent: the opener and assignee are told, and the edit is kept with who made it. A new owner (assignee) moves who acts as task.handoff does: with turns open it replaces the top turn's agent; an agent's owner `human` is task.turn.push human. blocked_by and unblock block the task whoever acts on it: the slot freed is the acting agent's (Task.acting).

### `task.ask`

The founder asks an agent a question about a task: a message linked to the task, whose holder does not change. The agent's reply joins the task.

