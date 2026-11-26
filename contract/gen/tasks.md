<!-- Generated from contract/spec/ by contract/generate.py; do not edit. -->
# tasks methods (protocol 1)

A method marked write takes a required `request_id`. Framing, roles, errors and gates are in [protocol.md](../protocol.md); field types in `protocol.ts`.

| Method | Role | Write | Params | Result | Errors | Gates | App |
|---|---|---|---|---|---|---|---|
| `tasks.list` | agent |  | `owner?`, `acting?`, `reporter?`, `from?`, `state?`, `tags?`, `parent?`, `under?`, `request?`, `my_requests?`, `decided_since?`, `ids?`, `before?`, `order?`, `cursor?`, `limit?`, `all?`, `everyone?`, `my_turn?`, `reason_gaps?` | `{items, more, reason_gaps, next} & ReadCursor` |  |  | `tasks`, fed by tasks |
| `task.get` | agent |  | `task` | `TaskDetail & ReadCursor` | `not_found` |  | `task` by task, fed by tasks |
| `task.history` | agent |  | `task` | `{task, rows}` | `not_found`, `bad_request` |  | `task-history` by task, snapshot |
| `task.time` | agent |  | `task` | `{task, start, end, segments, estimate, expected, box} & ReadCursor` | `not_found`, `bad_request` |  | `task-time` by task, fed by tasks |
| `task.changes` | member |  | `task`, `commit?` | `{task, commits, commit, files, diff, truncated, left_out}` | `not_found`, `bad_request` |  | `task-changes` by task, fed by tasks |
| `task.new` | agent | yes | `owner`, `reporter?`, `title`, `body?`, `time_box?`, `parent?`, `no_parent?`, `request?`, `duplicate_ok?`, `blocked_by?`, `watch?`, `priority?`, `estimate?`, `tags?`, `expects?`, `ui?`, `draft?`, `request_id` | `Accepted & {task}` | `refused` | duplicate_title | authored: new task |
| `task.handoff` | agent | yes | `task`, `to`, `note?`, `files?`, `checked?`, `misses?`, `tests?`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | not from the app |
| `task.turn.pop` | agent | yes | `task`, `outcome`, `note?`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | decided: give back |
| `task.done` | agent | yes | `task`, `evidence?`, `request?`, `repo?`, `reason?`, `commit?`, `shots?`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` | open_children | authored: done |
| `task.reopen` | agent | yes | `task`, `note?`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | authored: reopen |
| `task.progress` | agent (only agent) | yes | `task?`, `note`, `done?`, `total?`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` | not_self | not from the app |
| `task.begin` | agent | yes | `title`, `body?`, `message`, `priority?`, `expects?`, `why?`, `request_id` | `Accepted & {task}` | `refused` |  | not from the app |
| `task.start` | agent | yes | `task`, `request_id` | `Accepted` | `refused`, `not_found` |  | not from the app |
| `task.seen` | member | yes | `task`, `upto?`, `request_id` | `Accepted` | `not_found`, `bad_request` |  | decided: mark seen |
| `requests.pending` | member |  | none | `{answer, comments, counts} & ReadCursor` |  |  | `requests-pending`, fed by tasks |
| `task.comment.add` | agent | yes | `task`, `text`, `now?`, `reply?`, `turn?`, `done?`, `files?`, `request_id` | `Accepted & {memo}` | `refused`, `not_found`, `bad_request` |  | authored: add comment |
| `task.comment.recipients` | agent |  | `task`, `text`, `now?` | `{recipients}` | `not_found`, `bad_request` |  | `comment-recipients`, fed by tasks |
| `task.comment.edit` | agent | yes | `task`, `memo`, `text`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | authored: edit comment |
| `task.comment.delete` | agent | yes | `task`, `memo`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | authored: delete comment |
| `task.send_now` | member | yes | `task`, `request_id` | `Accepted` | `refused`, `not_found` |  | decided: run this one now |
| `task.queue` | agent | yes | `tasks`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | authored: queue |
| `task.draft` | agent | yes | `tasks`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | authored: draft |
| `task.merge` | agent | yes | `into`, `tasks`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | authored: merge |
| `task.release` | agent | yes | `task`, `to?`, `reason?`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | authored: release |
| `task.request` | agent | yes | `request`, `title?`, `owner?`, `request_id` | `Accepted & {task, existing}` | `refused`, `not_found`, `bad_request` |  | decided: make a request |
| `task.wait` | agent | yes | `task`, `watch`, `time_box?`, `request_id` | `Accepted & {gate}` | `refused`, `not_found`, `bad_request` |  | not from the app |
| `task.discard` | agent | yes | `task?`, `tasks?`, `reason?`, `reasons?`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | decided: discard |
| `task.board` | agent |  | `everyone?` | `{mine}` |  |  | `board`, fed by tasks |
| `task.withdraw` | agent | yes | `task`, `request_id` | `Accepted` | `refused`, `not_found` |  | authored: withdraw |
| `task.edit` | agent | yes | `task`, `owner?`, `reporter?`, `title?`, `body?`, `evidence?`, `time_box?`, `priority?`, `estimate?`, `blocked_by?`, `unblock?`, `parent?`, `no_parent?`, `add_tags?`, `remove_tags?`, `request_id`, `pr?` | `Accepted` | `refused`, `not_found` |  | authored: edit |
| `task.plan` | agent | yes | `task`, `text`, `request_id` | `Accepted & {version}` | `refused`, `not_found`, `bad_request` |  | decided: plan |
| `task.plan_ok` | owner | yes | `task`, `ok?`, `version?`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | decided: plan OK |
| `task.ok` | owner | yes | `task`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | decided: OK, merge |
| `task.redo` | owner | yes | `task`, `note`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | decided: Redo |
| `task.plan_ask` | member | yes | `task`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | decided: ask for a plan |
| `task.core_ask` | member | yes | `task`, `note?`, `request_id` | `Accepted` | `refused`, `not_found`, `bad_request` |  | decided: ask for core logic |
| `lesson.add` | agent (only agent) | yes | `text`, `task`, `about?`, `said?`, `request_id` | `Accepted & {id}` | `refused`, `not_found`, `bad_request` |  | not from the app |
| `lesson.merge` | agent (only agent) | yes | `ids`, `text`, `about?`, `said?`, `request_id` | `Accepted & {id}` | `refused`, `bad_request` |  | not from the app |
| `lessons.list` | agent |  | `agent?` | `{agent, items, bytes, cap}` |  |  | not cached |
| `lesson.remove` | owner | yes | `id`, `request_id` | `Accepted` | `not_found`, `bad_request` |  | decided: remove a lesson |

### `tasks.list`

Task rows. Newest id first. To read every task, call with `all: true` and `limit: 200`, then again with `before` set to the smallest `id` of the page before, until `more` is false: nothing is skipped or repeated, because a page is the rows with an id below `before`. The caller's own scope still applies (its own tasks for an agent, unless `everyone`).

### `task.get`

One task with body, delivery history and linked events.

### `task.history`

Every change to a task, newest first: one row per field changed, with who made it, the value before and after, and when. It is read from the events each task write keeps in its own transaction, so it is never edited or deleted; a fix is a later row. A deleted memo's words stay in it. Anyone may edit a task (task.edit); this history is what makes that safe to undo by hand.

### `task.time`

Where a task's time went, from its opening to its end (or now): segments in order that cover it with no gap or overlap, and the split of its time box. Read from what the daemon already records (the task's state events, its turns, its blockers and children, its asks of the founder, its acting agent's parks and starts); nothing is timed twice. Where two causes overlap, the stretch shows one, in this order: waiting on the founder, blocked by a task, waiting on another agent, queued, at an account's wall, starting, working.

### `task.changes`

What a task changed in code: its commits, found by the task named in their subject's last parenthesis ("(T9201)" or "(T9139, T9140)", T<n> as a whole token) in the repo of its owner's workspace (every agent's repo when the owner is no agent): on origin/<default branch> (landed) and on any local branch not on it, so every agent's worktree branch is seen. A change that is on two branches (a rebased copy) shows once: the landed copy, else the newest. With `commit`, the files and diff are that commit's; without, the whole task's: one range when the commits follow each other in one repo, else each file's changes from each commit in turn.

### `task.new`

Create a task. The founder or a member device opens one for an agent only, and it opens queued. An agent owning it to `human` is refused as task.handoff does; a task begun from the founder's message answers to the founder (task.begin). The owner may not be the caller: a task an agent takes on itself is begun from the founder's message (task.begin). An agent may not open a task owned by human: the task is the agent's, and the founder's decision is a turn on it (task.comment.add with turn human).

### `task.handoff`

Hand a task to another agent. Anyone may hand an open task on, not only its owner (the asker takes a task back so). It moves who acts on the task: its owner, or with turns open the agent of its top turn; it is refused to an agent that is the owner below the turns or already has a turn on it. task.history's row names who made the move (by). An agent's handoff to `human` adds the note as a comment and puts the founder's turn on it (Turn.memo), as task.comment.add with turn human does; it needs a note. The task stays the agent's and waits on the founder. The founder may hand a task to `human`. A story's sub-task (opened as a draft) that its assignee hands to its workspace's land_by agent does not go there: it is Ready for the founder (TaskDetail.review), the note a comment carrying the result with any files and the founder's turn on it, until task.ok hands it on or task.redo sends it back; it needs a note. That holds until the founder's newest verdict is OK. While it is Ready, an agent's handoff of it is refused.

### `task.turn.pop`

End the top turn on a task: pass, or send it back with why. By the agent of the top turn or the founder. The turn comes off and the one below acts again: an agent gets the task at the front of its queue, with the note as a comment. A pass of a turn pushed with on_pass done closes the task with its evidence through the usual done gates instead; when a gate refuses (open children), the turn still comes off and the refusal is added to the note. While human is on top, the founder's comment on the task (task.comment.add) is a pass with the comment as its note, and the founder's give back is a pass with none. On a task Ready for the founder (TaskDetail.review.ready) neither passes: their give back is refused ("T<n> is Ready for you: answer it with OK, merge or Redo"), and their comment leaves the turn on and is told to the agent below it.

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

Record that the founder has a task in front of them: its comments up to a number, and its result when it has ended. What it moves: the founder's read marker for the task (a named cursor, task_seen:<person>:<task>), which only goes forward. Task.unread and Task.first_unread count the comments above it, and requests.pending drops a task whose comments are all read. It never reads away something that is waiting for an answer (a question to the founder, a decision, a permission request): those end only by answering. A task with no marker counts from a floor set once when the daemon first ran this version, so comments from before it do not show as new. On a task that has ended it also marks the result seen (RequestRow.ended.seen), whatever upto says; a task reopened and ended again is unseen again. A client calls it when the task page opens, and again when a new comment arrives while the page is open and in view. It sends the tasks feed event, so every device updates.

### `requests.pending`

What waits on the founder to act, in one read, and the counts the bell and the sidebar badges show. Agents' chat replies and the task board's notices are in neither list. A task the founder has read (task.seen) leaves comments on the next read and stays in answer. Refetch on the tasks, inbox, requests and for_you feed events.

### `task.comment.add`

Add a comment; anyone may. It is told only to an agent that has to act on it: one that names agents as @name is told to those only; one that names no one is told to the task's assignee (its holder) and, when the founder wrote it on a task no agent holds, who the task answers to (its reporter, else its opener); with now every one of those readers, mid-turn too. The founder's comment that gives their turn back, or asks on a task Ready for them, always reaches the agent below their turn. It is never told to its writer. task.comment.recipients says who will be told before posting. The founder is told by one inbox line per request (a task made from their message, task.request) while it stays unread, counting the agents' new comments. A reader at its prompt is told at once and a reader mid-turn at the end of its turn. comment.edit and comment.delete are told the same way. A comment asks the founder nothing unless it carries turn human: that is their turn on the task. One whose first word is `@human` is news for them: it moves no turn, and requests.pending lists it as a note until they read the task. The wire still calls a comment a memo (Memo, `memo`, `memos`). An ask an agent's `@human` comment opened before comments stopped asking (InboxItem kind ask, `task` and `memo` set) stays open until (1) the founder adds any comment to that task, (2) the founder marks it read (inbox.mark_read naming its id), (3) its comment is deleted or edited so it no longer names `@human`, or (4) the task is done or discarded. An edit that keeps `@human` first keeps that ask and updates its body. While human is the top turn (Task.acting), the founder's comment passes that turn, the comment its note (task.turn.pop), except on a task Ready for them (TaskDetail.review.ready): there it is a question told to the agent below the turn, the task stays Ready, and a turn with it is refused. With turn, the comment also puts `turn` on top of the task in the same transaction and is that turn's message (Turn.memo): by the agent acting on the task or the founder, refused as a turn is when `turn` is the owner or already has a turn, the task carries 3 turns, or it is ended. An agent is delivered the task with the comment; human is not delivered to: the task waits on the founder (requests.pending, my_turn), no agent is alarmed and its time box starts again when the turn ends. While the founder's turn is on top, the agent below adding another comment with turn human moves that turn to the new comment.

### `task.comment.recipients`

Who a comment would be told to, before it is posted: the rule task.comment.add uses, computed without writing anything. Given the task, the comment text, and whether it would be sent now, returns one row per agent task.comment.add would tell, each with why: `asked` (the text names it as @name; a comment that names someone is told to those only), `assignee` (the task's holder, told of a comment that names no one, and of the founder's that gives their turn back or asks on a task Ready for them) or `owner` (who the task answers to, when the comment is sent now or is the founder's on a task no agent holds) or `last_writer` (the last agent that commented on the task, for the founder's comment naming no one on a task they hold). An agent that fits several is named once, by the first of asked, last_writer, assignee, owner. The caller is the writer and is never listed; one that names no one on a task an agent holds returns the holder only; the founder's on a task they hold returns the last_writer, or nothing when no agent wrote there. The founder's comment that passes their turn is told by who holds the task after it, and lists that holder whomever it names.

### `task.comment.edit`

Edit a comment: its author or the founder. An edit that drops `@human` withdraws an ask the comment opened (task.comment.add). The wire still calls a comment a memo (Memo, `memo`, `memos`).

### `task.comment.delete`

Delete a comment: its author or the founder. Deleting a comment withdraws the ask it opened (task.comment.add). The wire still calls a comment a memo (Memo, `memo`, `memos`).

### `task.send_now`

While new work is paused (daemon.pause), give this one queued task to its agent now, as one the founder sent: it is told, and its agent starts or wakes for it as for a person's message. The pause stays and holds the rest. Refused when nothing is paused, or when the pause is not what holds the task: it is not queued, not for an agent, or already told.

### `task.queue`

Approve draft tasks to run: anyone may; each task's opener and assignee are told.

### `task.draft`

Send queued or doing tasks back to draft: anyone may; each task's opener and assignee are told. A holder working the task is told to stop and moved on to its next task, and commits or changes it left in its worktree are kept in a memo on the task. A draft clears the task's turns, telling the agents that held them: it goes back to its owner.

### `task.merge`

Fold tasks into one: the kept task takes their titles, requests and sub-tasks; anyone may, and the opener and assignee of each are told.

### `task.release`

Take a doing task back to queued or draft: anyone may, with a reason the opener and assignee are told (and the founder, for a task begun on their message). With turns open (Task.turns): by the top turn's agent or the founder it is a send_back of that turn with the reason (task.turn.pop); by anyone else, the owner too, it is refused: the top turn is popped first.

### `task.request`

Make a founder's chat message a request: a task the founder opened (opened_by the person's key), owned by the lead the message was sent to. The message becomes its first comment, images included. A message converts once; a second call returns the task already made from it. The founder, or the agent the message was sent to, may call it.

### `task.wait`

Make a task wait on outside state: a gate on watch (as task.new with owner gate), the task blocked by it and, when doing, released, all in one write, so the task never waits on a gate that blocks nothing. The task comes back to its holder's queue when the gate opens. The wait blocks the task whoever acts on it: the slot freed is the acting agent's (Task.acting).

### `task.discard`

Throw away a task that is not done: anyone may, with a reason the opener, assignee and asker are told. A request still open on it refuses an agent; the founder's discard withdraws each open request and tells its target. tasks in place of task discards a batch in one call, all or none, with one discard_batch event: the one task_batch card in the caller's chat. A request that already landed in its agent's session does not refuse the task's asker or the founder: the task is discarded, that request closes with it and its holder is told why; anyone else is refused. A discard clears the task's turns and tells the agents that held them.

### `task.board`

The caller's open tasks, and who holds each now. The founder's board is the tasks the founder owns.

### `task.withdraw`

Withdraw a task the caller asked for; anyone else discards it with a reason.

### `task.edit`

Change a task: owner (an agent giving it to `human` is refused, as task.handoff does), reporter, title, priority, body, result and time box, by anyone (the body at any state, a closed task included; the time box until it is delivered), except the result, which only the founder changes. Waiting on other tasks is added (blocked_by) or removed (unblock) with it. It moves under another task (parent) or to the top (no_parent). Never silent: the opener and assignee are told (a move tells the old and new parent's owners one line each), and the edit is kept with who made it. A new owner (assignee) moves who acts as task.handoff does: with turns open it replaces the top turn's agent; an agent's owner `human` puts the founder's turn on. blocked_by and unblock block the task whoever acts on it: the slot freed is the acting agent's (Task.acting).

### `task.plan`

The task's assignee keeps a new version of its plan (TaskDetail.plan). A new version clears the founder's OK and answers the ask for a plan. Refused (`refused`): a caller that is not the assignee, a task that has ended or that no agent works, and a text the plan already reads.

### `task.plan_ok`

The founder marks the task's plan as it stands OK, or with ok false takes that back. It is a plan_ok event on the task naming the version, so a newer version, or a change of the task's brief or assignee, clears it. Refused (`refused`): a task with no plan, one that has ended, and a plan already in that state.

### `task.ok`

"OK, merge": the founder's verdict on a story's sub-task that is Ready. Their turn on it comes off and it is handed to the land_by agent it waited for (TaskDetail.review.land_by), with the agent's result as the note, as the agent's own handoff would have been. A task_verdict event (ok) records it. Refused (`refused`): a task that is not Ready now (never finished, or already answered); an agent caller (gate role).

### `task.redo`

"Redo": the founder sends a story's sub-task that is Ready back to its assignee with a note, kept as their comment; the agent is typed it at once. A task_verdict event (redo) records it, and TaskDetail.review.redo counts them. The agent's next finish is Ready again. Bad request (`bad_request`, field note): no note. Refused (`refused`): a task that is not Ready now; an agent caller (gate role).

### `task.plan_ask`

Ask the task's assignee for its plan (Ask for a plan, or a plan asked again); no plan is asked without it. The ask waits until the agent is no longer doing the task it was doing, then is typed to it (TaskDetail.plan_ask): it reads and changes nothing, and keeps the plan with task.plan. Refused (`refused`): a task that has ended or that no agent works, and one whose plan ask has not been sent yet (plan_state asked); a sent one (writing) may be asked again. An agent with a plan ask sent in the last hour and unanswered gets no other ask until it answers.

### `task.core_ask`

Ask the task's assignee to write only its key function and that function's test, committed on a branch of the task's own in its worktree, not merged, and to say so in a comment. The ask waits as task.plan_ask's does (TaskDetail.core_ask). The code is read with task.changes; when the task is run its assignee carries on from that branch. Refused (`refused`): a task that has ended or that no agent works, and one with a core-logic ask that has not been sent yet.

### `lesson.add`

An agent keeps one thing it learned (from a Redo, on task): its next sessions start with it in their launch prompt; with about founder, a preference of the founder's that every agent's next session starts with. Writes a `lesson` event on the task. Refused (`refused`) when the lessons would pass their 3 KB limit (the prompt section; the founder's have their own): merge similar ones first with lesson.merge; or when said is not the founder's. Not found: no such task.

### `lesson.merge`

An agent replaces two or more of its live lessons (or with about founder, of the founder's) with one: they are kept, marked merged into it; its task is the newest source's that names one. Refused (`refused`): an id that is not one of the caller's live lessons, or a merge still past the 3 KB limit.

### `lessons.list`

An agent's live lessons (or the founder's preferences), oldest first, as the launch prompt carries them, and how many of the limit's bytes they take.

### `lesson.remove`

The founder removes one of an agent's lessons; it is kept, marked removed with who and when. Not found (`not_found`, data.what `lesson`): no live lesson of that id.

