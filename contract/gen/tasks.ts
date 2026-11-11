// Generated from contract/spec/ by contract/generate.py; do not edit.
/* eslint-disable */
import type { Accepted, Actor, AgentName, AttachmentId, Duration, ImagePart, ReadCursor, RequestId, RequestName, RequestRef, Role, Status, TextPart, Timestamp, Topic, WriteKind } from './core.ts';

/** The task's clock: it counts while the task is doing and nothing holds it, stands still keeping what is left while anything does, and runs on past its length. */
export type TaskClock = {
  /** The time box in seconds. */
  length_s: number;
  /** True when the length is the daemon's default, none was set. */
  default: boolean;
  start: Timestamp | null;
  end: Timestamp | null;
  overdue_since: Timestamp | null;
  /** How many overrun notices the holder has had in this box: the first at `end`, then one each further length the clock runs. An edit of the timeout, or the task going back to queued or draft, starts a new box. */
  told: number;
  left_s: number | null;
};

/** A task row without its body. */
export type Task = {
  id: number;
  from: string;
  to: string;
  /** Who acts on the task now: the agent of its top turn, or human, else its owner (to). Every rule that asks who works the task now (delivery, the doing slot, the time box and whom it tells, my_turn, requests.pending, the board's holder, whom a comment tells) reads this. Absent from a daemon that does not send it. */
  acting?: string;
  /** The turns above the owner, bottom first; empty for most tasks, at most 3. The top acts now (acting); an agent below it acts on nothing, so its next task starts. Absent from a daemon that does not send it. */
  turns?: Turn[];
  title: string;
  /** draft, queued, doing, done or discarded. doing is its owner's current, unblocked task (one per agent). */
  state: string;
  time_box: Duration | null;
  parent: number | null;
  /** The founder request the task serves, as the number of that request's own task; null for an agent's own work. Set when the task is made (db.SetTaskRequest). Absent from a daemon that does not send it. */
  request?: number | null;
  /** work or check: check when the owner's row says role: check (an audit or landing agent, whose tasks gate other work), else work. Read from the owner's row when the task is read, not stored. Absent from a daemon that does not send it. */
  role?: "work" | "check";
  created: Timestamp;
  delivered: Timestamp | null;
  done: Timestamp | null;
  hold_reason: string | null;
  block_reason: string | null;
  evidence: string | null;
  /** 1 high, 2 default, 3 low (written P1-P3). */
  priority?: 1 | 2 | 3;
  /** Its tags, in order. */
  tags?: Tag[];
  /** The newest of the task's creation, edit, done and discard and of any request linked to it: what a list sorts a person's tasks by. Absent from a daemon that does not send it. */
  last_activity?: Timestamp;
  /** Who last changed the task's owner, title, priority or brief; null when never. */
  edited_by?: string | null;
  /** Who the task answers to when it closes: its reporter when one was set; else the founder (`human`) if any request linked to it came from the founder, else whoever opened it. */
  asker?: string;
  /** Null when the task was not discarded. */
  discarded?: {
    by: string;
    reason: string;
    at: Timestamp;
    /** The requests the discard withdrew with it (a lead's or the founder's), as request ids. */
    withdrew?: number[];
    /** The batch it was discarded in, null for a discard of its own. */
    batch?: string | null;
  } | null;
  /** Where the task's state and its delivery disagree (a draft with work out, an approved task nothing is out for), in words; null when they agree. */
  mismatch?: string | null;
  /** The requests linked to the task, oldest first. */
  requests?: {
    id: number;
    ref?: RequestRef;
    from: string;
    to: string;
    state: string;
    at: Timestamp;
  }[];
  /** The ids of the open tasks that hold this one (block_reason names them as text); empty when nothing does. Absent from a daemon that does not send it. */
  blocked_by?: number[];
  /** Every task this one has waited on, oldest first, with whether it is still open: blocked_by keeps only the open ones, this keeps a done one too (a discarded blocker's wait ends with it and drops out). A child task is one its parent waits on. Absent from a daemon that does not send it. */
  waits?: {
    task: number;
    open: boolean;
  }[];
  /** Why the open task waits, as one field every view shows the same way; null when nothing waits, and for a task that is not queued or doing. hold_reason and block_reason stay until the apps read this. Absent from a daemon that does not send it. */
  waiting?: TaskWaiting | null;
  /** The task's clock, the same object task.get gives; null once it is done or discarded. Absent from a daemon that does not send it. */
  time?: TaskClock | null;
  /** The pull request its work merges in (task.edit pr), as a link. Absent from a daemon that does not send it. */
  pr?: number | null;
  /** What the task watches when its owner is gate; null for any other. Absent from a daemon that does not send it. */
  watch?: TaskWatch | null;
  /** What it is doing and why (x-status). Absent from a daemon that does not send it. */
  status?: Status;
  /** How many comments by anyone but the founder have an id above the founder's read marker for this task (task.seen). 0 for an agent, and for a task nobody commented on. Absent from a daemon that does not send it. */
  unread?: number;
  /** The id of the first of those comments, where a task page draws its New divider; null when unread is 0. Absent from a daemon that does not send it. */
  first_unread?: number | null;
  /** API-equivalent dollars charged to the task and its subtasks at any depth (calls priced at list price); null when none was. tasks.list and task.get fill it; a task change notice carries null, as the calls change every tick. */
  spent_usd?: number | null;
};

/** Why a task waits (Task.waiting). */
export type TaskWaiting = {
  /** The cause, one word of an open set (a client shows text for a word it does not know): task (behind its open blockers, tasks names them), founder (an open request to the founder on the task), cap (the owner's account has no free slot), wall (the owner's account is at a usage limit), owner (the owner is busy: mid-turn, a dialog, closed, another task ahead), host (reserved: the host is full; not sent until the host cap exists). */
  on: string;
  /** The line every view shows for it: `blocked by T<n>`, `needs you`, else the daemon's own reason. */
  text: string;
  /** The tasks it waits on, for on task (by id, also one outside the caller's scope); empty otherwise. */
  tasks: number[];
  since: Timestamp;
};

/** The fields of Task a client reads to draw a listed task's state. */
export type TaskLook = {
  state: string;
  waiting: TaskWaiting | null;
  hold_reason: string | null;
  block_reason: string | null;
  evidence: string | null;
};

export type TaskDetail = Task & {
  body: string;
  /** `body` cut at the images it names, in order; present only when it names one. The text parts joined give `body` with each reference left out. */
  body_parts?: (TextPart | ImagePart)[];
  deliveries: {
    at: Timestamp;
    how: string;
    outcome: string;
  }[];
  events: TaskEvent[];
  memos: Memo[];
  blockers: ({
    task: number;
    reason: string;
    owner: string;
    title: string;
  } & TaskLook)[];
  children: number[];
  handoffs: {
    at: Timestamp;
    from: string;
    to: string;
    note: string;
  }[];
  /** Every change of the assignee or the owner made by an edit, oldest first; anyone may make one. Handoffs are listed in handoffs. */
  assignments?: {
    at: Timestamp;
    /** Who made the change. */
    by: string;
    /** assignee: who works the task (its owner column); owner: who it answers to (its reporter). */
    what: "assignee" | "owner";
    from: string;
    to: string;
  }[];
  /** Who the task answers to when it closes: its reporter when one was set; else the founder (`human`) if any request linked to it came from the founder, else whoever opened it. A client lets the founder reopen a done task only when this is the founder. */
  asker: string;
  discarded: {
    by: string;
    reason: string;
    at: Timestamp;
  } | null;
  merged_into: number | null;
  /** The task's clock; null once it is done or discarded. */
  time: TaskClock | null;
  parent_title: string | null;
  /** The sub-tasks by number: `children` with their titles, owners and states. */
  subtasks: ({
    id: number;
    title: string;
    owner: string;
    state: string;
  } & TaskLook)[];
  /** What happened on the task as one list, oldest first: its memos, its handoffs (a handoff's note is its entry, not a second one), the requests linked to it, the edits of its brief, its marks done and each time it was sent back. Overlaps `events`, `memos`, `handoffs` and `deliveries`, which stay. */
  timeline: {
    at: Timestamp;
    /** The author, or `from → to` for a handoff or a turn. */
    who: string;
    /** memo; handoff; a request linked to the task, except a memo's tell, the ask to the founder an @human memo opens, and a request that forwards a comment or a turn's return (any of them withdrawn too), which repeat their memo or turn; edit, the brief changed (text is the new brief); state, a change of state (text is `from -> done`); reject, the task sent back (text is the note), from before a review was a handoff; turn, a turn pushed (who is `by → to`, text its ask, empty when it was handed over with a comment: memo names that comment) or given back (who is `from → to`, text empty: a note is its comment). */
    kind: "memo" | "handoff" | "request" | "edit" | "state" | "reject" | "turn";
    /** Already stripped of control characters: a client draws it as it is. */
    text: string;
    /** On a turn row: push, the turn handed to `to` (text its ask, which may be empty); pop, given back to `to`. */
    turn?: "push" | "pop";
    /** On a request the daemon delivered on someone's behalf: queue, the queued task sent to its free holder (or raised on the founder's board); resend, the delivery sent again to the task's new holder. Absent on a request someone sent. */
    forwards?: "queue" | "resend";
    memo: number | null;
    edited_at: Timestamp | null;
    /** `text` cut at the images it names, for a memo, a handoff, an edit or a reject; present only when it names one. */
    parts?: (TextPart | ImagePart)[];
  }[];
  /** Whom the founder can ask about the task (task.ask), each agent once. */
  people: {
    /** Who the founder last chatted with, latest first. */
    recent: string[];
    /** The rest of the holder's team, in the order its agents were added. */
    team: string[];
    others: string[];
  };
  /** The task was opened as a ui task. Absent when it was not. */
  ui?: boolean;
  /** What the task's end kept beyond its evidence. Absent for a task without any. */
  result?: {
    /** The full sha it was done at. */
    commit?: string;
    /** Whether main reaches commit. */
    on_main?: boolean;
    /** Fetch each with task.shot. */
    shots: {
      n: number;
      bytes: number;
    }[];
  };
  /** What the task and its subtasks cost, null when nothing was charged to them. */
  spend?: {
    /** Dollars in all. */
    usd: number;
    /** Minutes of work: gaps of at most 15 minutes between one agent's calls on the task. */
    minutes: number;
    /** Dollars by the agent that made the calls. */
    by_agent: { [key: string]: number };
    /** Dollars by model. */
    by_model: { [key: string]: number };
  } | null;
};

/** An event as task.get lists it: the fields of Event but no cursor, because task events are not on the stream and have no position in it. */
export type TaskEvent = {
  topic: Topic;
  /** See x-topics for each topic's types. */
  type: string;
  at: Timestamp;
  actor: Actor | null;
  agent: AgentName | null;
  title: string | null;
  target: {
    screen: string;
    /** Agent name, task or item id. */
    id: string;
  } | null;
  data: { [key: string]: unknown };
};

export type Memo = {
  id: number;
  task: number;
  author: string;
  at: Timestamp;
  text: string;
  /** When the memo's text was last changed; null if never. */
  edited_at?: Timestamp | null;
  /** `text` cut at the images it names, as TaskDetail.body_parts; present only when it names one. */
  parts?: (TextPart | ImagePart)[];
  /** The files the memo carries, in the order they were given; present only when it carries one. */
  files?: MemoFile[];
  /** The comment this one answers; null for none. */
  reply_to?: number | null;
  /** Where the tell of a founder's comment stands for each agent it was told to, in the order told; present only on a founder comment that told someone. */
  readers?: MemoReader[];
};

export type TaskBoardRow = {
  task: Task;
  holder: AgentName | null;
  /** Who has the task now, read from its linked requests: `waiting` on open blocker tasks (`tasks`); `held` by the agents a request is with (`agents`); `sub_task` when an open child task's request is with them (`agents`); `held_as_sub_task` while an open child shares a request with it (`tasks`, `agents` their owners); `result_in` when the last request came back done from someone other than the caller or the founder; else `nobody`. */
  now?: {
    kind: "waiting" | "held" | "sub_task" | "held_as_sub_task" | "result_in" | "nobody";
    agents: string[];
    tasks: number[];
  };
};

/** What task.done must be given: sha, verdict, decision or report (default; decision when the owner is human). */
export type Expects = "sha" | "verdict" | "decision" | "report";

/** A file a memo carries (task.memo.add files): an image or a Markdown text, kept on the daemon's machine like an attached image and read with attachment.get by its id. */
export type MemoFile = {
  id: AttachmentId;
  kind: "image" | "markdown";
  /** The file's name as its writer gave it, with no folder. Shown to the person; never used as a path. */
  name: string;
  mime: "image/png" | "image/jpeg" | "image/gif" | "image/webp" | "text/markdown";
  bytes: number;
  /** Where the daemon keeps it; only in an agent's reads, which read it with their own file tool. */
  path?: string;
  /** True when it is no longer kept: attachment.get then answers not_found. */
  gone?: boolean;
  /** true when the founder deleted the file (attachment.remove), always with gone: the client says it was deleted. */
  removed?: boolean;
};

/** One change to a task: who changed which field from what to what, and when. A field's change is one row; an edit of several fields is several rows with the same at. */
export type HistoryRow = {
  at: Timestamp;
  /** Who made the change: an agent, or human. */
  by: string;
  /** evidence is a done task's result. assignee is who works the task, owner who it answers to. state is opened, draft, queued, doing, done, discarded, closed, released or merged. blocked_by is a task it waits on, as T<n>. tag is a tag added (was empty) or removed (now empty). */
  field: "title" | "assignee" | "owner" | "body" | "evidence" | "priority" | "estimate" | "timeout" | "state" | "tag" | "blocked_by" | "merged_into" | "memo";
  /** The value before; empty when there was none (a task just opened, a wait just added, a memo just added). */
  was: string;
  /** The value after; empty when it is gone (a wait removed, a memo deleted). */
  now: string;
  /** With a state row: the done evidence, the report, or the reason or note given. Absent when none was. */
  note?: string;
  /** With a memo row: the memo's id. */
  memo?: number;
  /** With a discarded state row: the requests withdrawn with it. Absent when none were. */
  withdrew?: number[];
};

/** A free label on a task, such as fe or be: lowercase letters, digits, '.', '_' or '-', up to 32. A writer lowercases what it is given; a task carries at most 20. */
export type Tag = string;

/** What tasks.list narrows to: every key given must hold (AND). It is tasks.list's own params, so a saved view's filter is passed to tasks.list as it is. */
export type TaskFilter = {
  /** Tasks that carry every one of these tags. */
  tags?: Tag[];
  state?: "draft" | "queued" | "doing" | "done" | "discarded";
  /** Who works the task (its assignee). */
  owner?: AgentName;
  /** Who the task answers to (its asker: hl's --owner). */
  reporter?: AgentName;
  /** Who opened it. */
  from?: AgentName;
};

/** A saved view: a filter kept under a name, so it is picked, not typed again. One set for the founder's devices. */
export type TaskView = {
  name: string;
  filter: TaskFilter;
  /** Who saved it last. */
  by: string;
  at: Timestamp;
};

/** One stretch of a task's time: one block of the bar under its title, sized by seconds. */
export type TimeSegment = {
  /** queued: not yet with its holder. waiting: held up, on says by what. starting: the session of the agent acting on it is starting, from the daemon's launch until the session's first hook, at most 2 minutes. working: from when the task delivery landed in its holder session (or task_start, if earlier) until reported, released or done. */
  kind: "queued" | "waiting" | "starting" | "working";
  on: ("founder" | "task" | "agent" | "account") | null;
  /** The reason as shown: `needs you`, `blocked by T<n>`, `waiting on T<n>` (an open child), `waiting on <agent>`, `wall`, `queued`, `agent starting`; empty for working. */
  text: string;
  task: number | null;
  from: Timestamp;
  to: Timestamp;
  /** to less from. */
  seconds: number;
};

/** What a gate watches: target of vendor, until what it waits for. The vendor answers waiting, passed or failed with a line of evidence. Vendors: github (target owner/name#N; until ci: a pull request's checks have all finished, or none started 10 minutes after its head commit; closed: the pull request merged (passed) or closed unmerged (failed); done: the issue closed as completed (passed) or for another reason (failed)). The github vendor reads through the gh CLI's own login. docs/watch-vendors.md has each vendor. */
export type Watch = {
  /** The outside tool: github. */
  vendor: string;
  /** The thing, written the way the vendor writes it: owner/name#N for github. */
  target: string;
  /** What is waited for; each vendor takes some. Absent: the vendor's first (github: ci). */
  until?: "ci" | "closed" | "done";
};

/** A gate's watch as the task carries it, with its label and what it opened with. */
export type TaskWatch = {
  vendor: string;
  target: string;
  until: "ci" | "closed" | "done";
  /** The watch in words ("PR #418 CI"). */
  label: string;
  /** What it opened with; null while it watches. A target that reads as gone (deleted, or no longer visible) three times in a row is failed. */
  result: ("passed" | "failed" | "timed_out") | null;
};

/** One reader of a founder's comment and where its tell stands. */
export type MemoReader = {
  agent: string;
  /** The request that told the comment to this agent. */
  request: number;
  /** held: waits for the agent's next digest (send it now with task.comment.send_now). sent: sent in a digest, not opened yet. read: typed to the agent at once and landed in its session, or opened with hl read. */
  state: "held" | "sent" | "read";
  /** held: when the tell began to wait. sent: when it was sent. read: when it was read. */
  at: Timestamp | null;
  /** Only when held: about how many minutes until the agent's next digest (at least 1), at the default window. */
  minutes?: number;
};

export type PendingRequest = {
  /** The task; null for an item that belongs to no task (a permission request such as Asks to use Bash). */
  task: number | null;
  /** The task's title; for an item with no task, its own line (a prompt's command or tool, a proposal's title, a warning in plain words). */
  title: string;
  /** The strongest item the row holds: allow (a permission request), decide (a decision, or a turn on the task), ask (a question to the founder), warning (the daemon needs the founder to act: see warning), or comment (unread comments only). note: an `@human` comment the founder has not read, on a task no other row stands for (news, no turn): source comment, id the comment, ask its first line, agent its author; it leaves when the founder reads the task (task.seen). */
  kind: "allow" | "decide" | "ask" | "warning" | "comment" | "note";
  /** The inbox ids of the answer-needed items; empty for a comment row. */
  items: number[];
  /** The task's unread comments (Task.unread); 0 for a row without a task. */
  unread: number;
  /** Task.first_unread. */
  first_unread: number | null;
  last_at: Timestamp;
  /** What the row was made from, as ForYouItem.kind: with id it names the item for NavPrefs.later. A row of a task that holds several takes the one kind says. Absent from a daemon that does not send it. */
  source?: "task" | "comment" | "prompt" | "proposal" | "machine";
  /** ForYouItem.id of that item. Absent from a daemon that does not send it. */
  id?: string;
  /** Who asks; null for the daemon's own warnings. Absent from a daemon that does not send it. */
  agent?: AgentName | null;
  /** What is asked, one line: the turn's ask or the first line of the comment asking them; null when there is nothing beyond the title. Absent from a daemon that does not send it. */
  ask?: string | null;
  /** The daemon's warning on the row: set on a warning row, and on a task row where a stronger item (an ask, a decision, a permission) outranks a warning the task also has; null otherwise. Absent from a daemon that does not send it. */
  warning?: PendingWarning | null;
};

/** One turn on a task (docs/design/turns.md): the agent who acts on the task while it is on top, or human. */
export type Turn = {
  /** Who acts while this turn is on top: an agent, or human. */
  agent: string;
  /** What it was asked to do, as it reads now: the text of its comment (memo) when it has one; empty when nothing was said. */
  ask: string;
  /** Who put it there: the one acting below it, or the founder. */
  by: string;
  since: Timestamp;
  /** done: a pass of this turn closes the task with evidence, through the usual done gates. */
  on_pass: "done" | null;
  /** The done evidence a pass closes the task with (on_pass done). */
  evidence: string | null;
  /** When ask was last rewritten; null when it reads as pushed. */
  edited: Timestamp | null;
  /** The texts ask replaced, oldest first, each with when it was set; empty when never rewritten. In task.get only. */
  earlier?: TurnAsk[];
  /** The comment the turn was handed over with (task.comment.add with turn), its message; null for a turn put on with no comment: the daemon's at a review's end, an agent's owner edit to human (task.edit), and turns from before. */
  memo: number | null;
};

/** A text a turn's ask once read. */
export type TurnAsk = {
  ask: string;
  at: Timestamp;
};

/** What a warning row is about, so a client can draw it and open the right place. It is read from state each time and leaves by itself. */
export type PendingWarning = {
  /** When each leaves. disk: space is back. login: the account is logged in or woken; one past its end day never warns. delivery (a message of the founder's never seen to arrive): seen to arrive, withdrawn, or their next message to that agent is written. undelivered (a task queued, its delivery refused, its agent with no lead): delivered, given to another or ended. */
  kind: "disk" | "login" | "delivery" | "undelivered";
  /** login: the account. */
  account?: string;
  /** delivery, undelivered: the agent it did not reach. */
  agent?: string;
  /** delivery: the message. */
  ref?: RequestRef;
};

export interface TasksMethods {
  "tasks.list": { params: {
  owner?: AgentName;
  /** Only the tasks this agent, or human, acts on now (Task.acting). */
  acting?: AgentName;
  /** Who the task answers to (Task.asker). */
  reporter?: AgentName;
  from?: AgentName;
  /** draft, queued, doing, done or discarded, or one of three groups: open (queued or doing), draft, and closed (done and discarded together). open and draft return every row of the caller's scope: no limit, `more` false, no cursor. closed needs order closed and pages by its cursor. */
  state?: string;
  /** Tasks that carry every one of these tags (lowercased first). With the other filters, every one given must hold. */
  tags?: string[];
  parent?: number;
  /** That task and every task below it, at any depth, done and discarded included (all is implied); still within the caller's scope, newest first and paged as without it. */
  under?: number;
  /** The tasks that serve this request (Task.request) and every task below its own, done and discarded included (all is implied); still within the caller's scope, newest first and paged as without it. */
  request?: number;
  /** Only these tasks, at most 200, newest id first (all is implied; the caller's scope still holds). An id that is not found or out of scope is left out, never an error. limit defaults to the number of ids. Not with before, cursor or order closed: bad_request. */
  ids?: number[];
  /** Return rows older than this id. Not with order closed: that pages by cursor. */
  before?: number;
  /** id (the default): newest id first. closed: newest close first, by (the time it was done or discarded, id); only with state done, discarded or closed, else bad_request. A task old by id but closed today comes first. */
  order?: "id" | "closed";
  /** With order closed: the `next` of the previous page, in place of before. Opaque; a client never builds one. A cursor with any other order, or a garbled one, is bad_request. */
  cursor?: string;
  /** Default 50, at most 200. Not applied to state open or draft, which return every row. */
  limit?: number;
  /** Done and discarded tasks too. */
  all?: boolean;
  /** Every team's tasks, not only the caller's and its reports'. */
  everyone?: boolean;
  /** Only the tasks the caller acts on (acting) that wait on it: queued or doing, never draft or ended; the founder's are the ones acting is human. One row per task. It takes no acting. */
  my_turn?: boolean;
  /** Also count, over the last 30 days, the requests that ended with no reason and how tasks were closed (the `reason_gaps` of the result). It costs seconds on a large database: a screen that polls never asks. */
  reason_gaps?: boolean;
}; result: {
    items: Task[];
    more: boolean;
    /** Present only when asked for (`reason_gaps`: true). How many requests ended in the last 30 days with no reason, and how tasks were closed in that time (by sha, by request, by reason): what hl task list prints under the list. */
    reason_gaps?: {
      retirements: number;
      retirements_blank: number;
      closures_sha: number;
      closures_request: number;
      closures_reason: number;
    };
    /** With order closed and more true: the cursor of the next page. Absent otherwise. */
    next?: string;
  } & ReadCursor };
  "task.get": { params: {
  task: number;
}; result: TaskDetail & ReadCursor };
  "task.history": { params: {
  task: number;
}; result: {
    task: number;
    rows: HistoryRow[];
  } };
  "task.time": { params: {
  task: number;
}; result: {
    task: number;
    start: Timestamp;
    end: Timestamp | null;
    segments: TimeSegment[];
    /** Its story points; null when it has none. */
    estimate: number | null;
    /** When it is expected to start and end, deterministic from its queue, its owner's account and the history of done tasks by estimate; computed on each read, so it moves as they do. */
    expected: {
      /** When it is expected to start: when it started for one being worked on; after the owner's current task and every task ahead of it in the order they are handed out, and no earlier than the reset of a limit the owner's account is at. Null when a task ahead has no expected duration. */
      start: Timestamp | null;
      /** start plus duration_s, and the wait for a reset when the account runs out at its current burn before then. Null when the duration is unknown. */
      end: Timestamp | null;
      /** How long it is expected to take (for one being worked on, what is left); null when unknown. */
      duration_s: number | null;
      /** What the duration rests on: the median working time of the last 20 done tasks of its estimate (history), else their median time per point times its points (points), else its time box (time_box); null for none. */
      basis: ("history" | "points" | "time_box") | null;
      /** The tasks its owner does first, by number. */
      ahead: number[];
      /** The reset of the account limit that holds it back; null when none does. */
      wall_until: Timestamp | null;
    } | null;
    box: {
      /** The time box in seconds, as TaskClock.length_s. */
      length_s: number;
      start: Timestamp | null;
      /** Who worked on it in this box, most seconds first: the agent acting on it (its owner, or the agent of its top turn) while it was working. */
      worked: {
        agent: string;
        seconds: number;
      }[];
      /** The seconds of this box its acting agent's session spent starting. */
      waking_s: number;
      /** `length_s` less what the clock ran; below zero once it is over. */
      left_s: number;
    } | null;
  } & ReadCursor };
  "task.shot": { params: {
  task: number;
  /** The screenshot's number in the task's result, from 1. */
  n: number;
  /** Where the chunk starts in the file; 0 when omitted. */
  offset?: number;
}; result: {
    mime: "image/png";
    bytes: number;
    offset: number;
    /** The chunk, in base64: at most 512 KiB of file. */
    data: string;
    done: boolean;
  } };
  "task.new": { params: {
  /** An agent, human, or gate: a task the daemon holds that watches outside state (watch) and closes when it changes. */
  owner: AgentName;
  /** Who the task answers to: told when it ends, and may edit or reassign it. Default: the caller. */
  reporter?: AgentName;
  title: string;
  /** The brief, Markdown. It may name images (x-attachments, texts). */
  body?: string;
  time_box?: Duration;
  /** Default: when the caller's current task is a request (made from a founder's message), that task, unless no_parent. */
  parent?: number;
  /** Do not hang the task under the caller's current request. Refused with parent. */
  no_parent?: boolean;
  /** Open the task even when an open task under the same parent, or the parent itself, has nearly its title (gate duplicate_title). */
  duplicate_ok?: boolean;
  /** Cannot be worked until this task closes. */
  blocked_by?: number;
  /** With owner gate, required, and refused with any other owner; a watch its vendor cannot take, or a vendor not set up, is refused. The gate is doing from the start; its time box (default 24h) is its deadline. The daemon marks it done with the vendor's evidence when the vendor answers passed or failed ("ci passed", "ci failed: <check names>", "merged <sha>", "closed unmerged", "no CI on this PR"), or "timed out" past its deadline. A task blocked by it (blocked_by) then comes back to its holder, who decides what follows. task.wait opens one and blocks a task on it in one write. */
  watch?: Watch;
  /** 1 high, 2 default, 3 low (written P1-P3). Absent means 2. */
  priority?: 1 | 2 | 3;
  /** Story points: what the task is expected to take, for its expected start and end (task.time). None when absent. */
  estimate?: 1 | 2 | 3 | 5 | 8 | 13;
  /** Its tags; each is kept as a history row of the task. */
  tags?: Tag[];
  expects?: Expects;
  /** The task shows something on a screen. A label only: task.done takes a screenshot (shots) but never requires one. */
  ui?: boolean;
  request_id: RequestId;
}; result: Accepted & {
    task: number;
  } };
  "task.handoff": { params: {
  task: number;
  to: AgentName;
  /** Markdown. It may name images (x-attachments, texts); they are the task's while it is open. */
  note?: string;
  request_id: RequestId;
}; result: Accepted };
  "task.turn.pop": { params: {
  task: number;
  outcome: "pass" | "send_back";
  /** Why, for a send_back (required from an agent); for a pass, optional. */
  note?: string;
  request_id: RequestId;
}; result: Accepted };
  "task.done": { params: {
  task: number;
  /** A merge sha on main, or a reason in words. The founder may leave it out (and request): the task is done with the words Done. */
  evidence?: string;
  /** A request where the result was already reported. */
  request?: RequestName;
  /** The repo whose main a sha is checked against. */
  repo?: string;
  /** Record evidence as words even if it looks like hex. */
  reason?: boolean;
  /** The sha the work was done at; it must be a commit in repo (it need not be on main). The result says whether main reaches it. A sha given as evidence is a commit on main already. */
  commit?: string;
  /** Absolute paths of PNG files, at most 10 MiB each. An agent names files under its worktree, its folder in the daemon's home or the scratch folder; the founder any file. The daemon copies them under its home (shots/T<n>/<k>.png, outside any repo) once the task's end has committed, and serves them with task.shot. */
  shots?: string[];
  request_id: RequestId;
}; result: Accepted };
  "task.reopen": { params: {
  task: number;
  note?: string;
  request_id: RequestId;
}; result: Accepted };
  "task.progress": { params: {
  task?: number;
  note: string;
  request_id: RequestId;
}; result: Accepted };
  "task.begin": { params: {
  title: string;
  body?: string;
  /** The founder's message it answers. */
  message: RequestName;
  priority?: 1 | 2 | 3;
  expects?: Expects;
  /** What led to this, in a line; optional, since the linked message is the reason. */
  why?: string;
  request_id: RequestId;
}; result: Accepted & {
    task: number;
  } };
  "task.start": { params: {
  task: number;
  request_id: RequestId;
}; result: Accepted };
  "task.seen": { params: {
  task: number;
  /** The highest comment number that has been in view; the task's newest comment when absent. A lower number than the marker changes nothing: what arrives later has a higher one, so nothing unseen is hidden. */
  upto?: number;
  request_id: RequestId;
}; result: Accepted };
  "requests.pending": { params: { [key: string]: unknown }; result: {
    /** The founder's turn: one row per task with something only they can act on (a turn on it, a comment asking them, a decision), newest first, and an item with no task (a permission request, a proposal, a daemon warning) as its own row. A row leaves when it is resolved (answered, passed, delivered, logged in, its time gone) or put off (NavPrefs.later) and never by being read. A note row (an unread `@human` comment) is listed here too, but is not the founder's turn: counts.answer leaves it out. */
    answer: PendingRequest[];
    /** Not the founder's turn: tasks the founder answers for (its reporter is the founder, or its opener when it has none, or it is the founder's own) with unread comments and no row in answer, newest first. task.seen removes a row. No badge counts it. */
    comments: PendingRequest[];
    /** Every badge comes only from here, so the page and the badge cannot disagree: bell and Requests are answer, Tasks is needs_you. */
    counts: {
      /** The founder's turn: the rows of answer that are not notes. The bell and the sidebar Requests badge both show this. */
      answer: number;
      /** The length of comments. */
      comments: number;
      /** Open tasks owned by the founder or waiting on the founder (Task.waiting.on is founder). */
      needs_you: number;
      /** Open draft tasks, everyone's. */
      drafts: number;
      /** Finished requests the founder has not seen (requests.list unseen): the bell's dot, not a number on it. Absent from a daemon that does not send it. */
      unseen_results?: number;
      /** The note rows of answer: unread `@human` comments. The bell's dot, not a number on it. */
      notes: number;
    };
  } & ReadCursor };
  "task.comment.list": { params: {
  task: number;
  /** With limit: the page just older than this memo id. */
  before?: number;
  /** 1 to 200. The newest limit memos, or the limit just older than before; absent: every memo. */
  limit?: number;
}; result: {
    memos: Memo[];
    /** There are older memos than the page. */
    more: boolean;
  } };
  "task.memo.list": { params: {
  task: number;
  /** With limit: the page just older than this memo id. */
  before?: number;
  /** 1 to 200. The newest limit memos, or the limit just older than before; absent: every memo. */
  limit?: number;
}; result: {
    memos: Memo[];
    /** There are older memos than the page. */
    more: boolean;
  } };
  "task.comment.add": { params: {
  task: number;
  /** Markdown. It may name images (x-attachments, texts). */
  text: string;
  /** Tell every reader at once, even mid-turn. */
  now?: boolean;
  /** Agents not to tell of this comment, from the ones task.comment.recipients names. Skipping the assignee is allowed; the comment stays on the task either way. A name that would not have been told is ignored. */
  skip?: string[];
  /** The comment on the same task this one answers (Memo.reply_to). Refused (not_found memo) when the task has no such comment. */
  reply?: number;
  /** Hand the task to this agent, or human, with this comment: it goes on top and acts now; the one below keeps its turn but acts on nothing, so its doing slot frees. For human, the comment is the decision and what you recommend. */
  turn?: AgentName;
  /** With turn naming an agent: the done evidence a pass of that turn closes the task with (a review of finished work). */
  done?: string;
  /** Absolute paths of files the memo carries (Memo.files): PNG, JPEG, GIF or WebP images and .md Markdown, at most 10 MiB each. The daemon reads each and keeps a copy under its home. An agent may name only files under its worktree, its folder or the scratch folder; the founder any. A path outside those, a file of another type, an image whose first bytes do not match its ending, or a .md that is not text is bad_request (field files). */
  files?: string[];
  request_id: RequestId;
}; result: Accepted & {
    memo: number;
  } };
  "task.comment.recipients": { params: {
  task: number;
  /** The comment as typed; only its @names count. */
  text: string;
  /** As task.comment.add: every reader, not only the ones aimed at. */
  now?: boolean;
}; result: {
    recipients: {
      agent: string;
      why: "asked" | "assignee" | "owner";
    }[];
  } };
  "task.memo.add": { params: {
  task: number;
  /** Markdown. It may name images (x-attachments, texts). */
  text: string;
  /** Tell every reader at once, even mid-turn. */
  now?: boolean;
  /** Agents not to tell of this comment, from the ones task.comment.recipients names. Skipping the assignee is allowed; the comment stays on the task either way. A name that would not have been told is ignored. */
  skip?: string[];
  /** Absolute paths of files the memo carries (Memo.files): PNG, JPEG, GIF or WebP images and .md Markdown, at most 10 MiB each. The daemon reads each and keeps a copy under its home. An agent may name only files under its worktree, its folder or the scratch folder; the founder any. A path outside those, a file of another type, an image whose first bytes do not match its ending, or a .md that is not text is bad_request (field files). */
  files?: string[];
  request_id: RequestId;
}; result: Accepted & {
    memo: number;
  } };
  "task.comment.edit": { params: {
  task: number;
  memo: number;
  /** Markdown. It may name images (x-attachments, texts). */
  text: string;
  request_id: RequestId;
}; result: Accepted };
  "task.memo.edit": { params: {
  task: number;
  memo: number;
  /** Markdown. It may name images (x-attachments, texts). */
  text: string;
  request_id: RequestId;
}; result: Accepted };
  "task.comment.delete": { params: {
  task: number;
  memo: number;
  request_id: RequestId;
}; result: Accepted };
  "task.comment.send_now": { params: {
  memo: number;
  agent: AgentName;
  request_id: RequestId;
}; result: Accepted };
  "task.send_now": { params: {
  task: number;
  request_id: RequestId;
}; result: Accepted };
  "task.memo.delete": { params: {
  task: number;
  memo: number;
  request_id: RequestId;
}; result: Accepted };
  "task.view.list": { params: { [key: string]: unknown }; result: {
    views: TaskView[];
  } };
  "task.view.save": { params: {
  /** One line, 1 to 60 characters. */
  name: string;
  filter: TaskFilter;
  request_id: RequestId;
}; result: Accepted };
  "task.view.delete": { params: {
  name: string;
  request_id: RequestId;
}; result: Accepted };
  "task.queue": { params: {
  tasks: number[];
  request_id: RequestId;
}; result: Accepted };
  "task.draft": { params: {
  tasks: number[];
  request_id: RequestId;
}; result: Accepted };
  "task.merge": { params: {
  into: number;
  tasks: number[];
  request_id: RequestId;
}; result: Accepted };
  "task.release": { params: {
  task: number;
  to?: "queued" | "draft";
  reason?: string;
  request_id: RequestId;
}; result: Accepted };
  "task.request": { params: {
  /** The founder's message. Refused when it was not sent by the founder to an agent. */
  request: RequestName;
  /** Default: the message's first line. */
  title?: string;
  /** The agent who holds the request. Default: the agent the message was sent to. */
  owner?: AgentName;
  request_id: RequestId;
}; result: Accepted & {
    task: number;
    /** True when the message had already been made a request. */
    existing?: boolean;
  } };
  "task.wait": { params: {
  task: number;
  watch: Watch;
  /** The gate's deadline, a span like 4h; default 24h. */
  time_box?: string;
  request_id: RequestId;
}; result: Accepted & {
    /** The gate task opened. */
    gate: number;
  } };
  "task.discard": { params: {
  task?: number;
  tasks?: number[];
  reason?: string;
  reasons?: { [key: string]: string };
  request_id: RequestId;
}; result: Accepted };
  "task.board": { params: {
  /** Every team's open tasks, not only the caller's and its reports'. */
  everyone?: boolean;
}; result: {
    mine: TaskBoardRow[];
    reports: TaskBoardRow[];
  } };
  "task.withdraw": { params: {
  task: number;
  request_id: RequestId;
}; result: Accepted };
  "task.edit": { params: {
  task: number;
  owner?: AgentName;
  /** A new reporter (who the task answers to); by its opener, its reporter or the founder. */
  reporter?: AgentName;
  title?: string;
  /** The new brief, Markdown. It may name images (x-attachments, texts). */
  body?: string;
  /** The new result of a done task (Task.evidence), Markdown, not empty: a done task keeps a result. Only the founder changes it: an agent is refused and adds a comment instead. A task that is not done has no result to edit and is refused. */
  evidence?: string;
  time_box?: Duration;
  /** 1 high, 2 default, 3 low (written P1-P3). */
  priority?: 1 | 2 | 3;
  /** Story points (as task.new); null clears it. */
  estimate?: (1 | 2 | 3 | 5 | 8 | 13) | null;
  /** A task number this one waits on, added in the same transaction as the other parts; a refusal of any part undoes all of them. A doing task goes back to queued. */
  blocked_by?: number;
  /** A task number this one no longer waits on; clearing the last one lets its owner's current task advance. */
  unblock?: number;
  /** Tags to add; one it has already is left as it is. Each change is a history row. Nobody is told of a tag change. */
  add_tags?: Tag[];
  /** Tags to remove; one it has not got is left as it is. */
  remove_tags?: Tag[];
  request_id: RequestId;
  /** The pull request the task's work merges in (`hl task edit T<n> --pr <n>`), on an open task. A link only: nothing waits on it. */
  pr?: number;
}; result: Accepted };
  "task.ask": { params: {
  task: number;
  to: AgentName;
  /** The question; not empty. */
  text: string;
  request_id: RequestId;
}; result: Accepted };
}

export const TASKS_METHODS: { [K in keyof TasksMethods]: { role: Role; callers: Role[] | null; write: boolean; since: number } } = {
  "tasks.list": { role: "agent", callers: null, write: false, since: 1 },
  "task.get": { role: "agent", callers: null, write: false, since: 1 },
  "task.history": { role: "agent", callers: null, write: false, since: 1 },
  "task.time": { role: "agent", callers: null, write: false, since: 1 },
  "task.shot": { role: "member", callers: null, write: false, since: 1 },
  "task.new": { role: "agent", callers: null, write: true, since: 1 },
  "task.handoff": { role: "agent", callers: null, write: true, since: 1 },
  "task.turn.pop": { role: "agent", callers: null, write: true, since: 1 },
  "task.done": { role: "agent", callers: null, write: true, since: 1 },
  "task.reopen": { role: "agent", callers: null, write: true, since: 1 },
  "task.progress": { role: "agent", callers: ["agent"], write: true, since: 1 },
  "task.begin": { role: "agent", callers: null, write: true, since: 1 },
  "task.start": { role: "agent", callers: null, write: true, since: 1 },
  "task.seen": { role: "member", callers: null, write: true, since: 1 },
  "requests.pending": { role: "member", callers: null, write: false, since: 1 },
  "task.comment.list": { role: "agent", callers: null, write: false, since: 1 },
  "task.memo.list": { role: "agent", callers: null, write: false, since: 1 },
  "task.comment.add": { role: "agent", callers: null, write: true, since: 1 },
  "task.comment.recipients": { role: "agent", callers: null, write: false, since: 1 },
  "task.memo.add": { role: "agent", callers: null, write: true, since: 1 },
  "task.comment.edit": { role: "agent", callers: null, write: true, since: 1 },
  "task.memo.edit": { role: "agent", callers: null, write: true, since: 1 },
  "task.comment.delete": { role: "agent", callers: null, write: true, since: 1 },
  "task.comment.send_now": { role: "member", callers: null, write: true, since: 1 },
  "task.send_now": { role: "member", callers: null, write: true, since: 1 },
  "task.memo.delete": { role: "agent", callers: null, write: true, since: 1 },
  "task.view.list": { role: "member", callers: null, write: false, since: 1 },
  "task.view.save": { role: "member", callers: null, write: true, since: 1 },
  "task.view.delete": { role: "member", callers: null, write: true, since: 1 },
  "task.queue": { role: "agent", callers: null, write: true, since: 1 },
  "task.draft": { role: "agent", callers: null, write: true, since: 1 },
  "task.merge": { role: "agent", callers: null, write: true, since: 1 },
  "task.release": { role: "agent", callers: null, write: true, since: 1 },
  "task.request": { role: "agent", callers: null, write: true, since: 1 },
  "task.wait": { role: "agent", callers: null, write: true, since: 1 },
  "task.discard": { role: "agent", callers: null, write: true, since: 1 },
  "task.board": { role: "agent", callers: null, write: false, since: 1 },
  "task.withdraw": { role: "agent", callers: null, write: true, since: 1 },
  "task.edit": { role: "agent", callers: null, write: true, since: 1 },
  "task.ask": { role: "member", callers: null, write: true, since: 1 },
};

/** x-app-cache of each read (x-app in core.json). */
export const TASKS_APP_CACHE: { [K in "tasks.list" | "task.get" | "task.history" | "task.time" | "task.shot" | "requests.pending" | "task.comment.list" | "task.memo.list" | "task.comment.recipients" | "task.view.list" | "task.board"]: { resource: string; id?: string; feed: Topic | null } | false } = {
  "tasks.list": {"resource": "tasks", "feed": "tasks"},
  "task.get": {"resource": "task", "id": "task", "feed": "tasks"},
  "task.history": {"resource": "task-history", "id": "task", "feed": null},
  "task.time": {"resource": "task-time", "id": "task", "feed": "tasks"},
  "task.shot": false,
  "requests.pending": {"resource": "requests-pending", "feed": "tasks"},
  "task.comment.list": {"resource": "memos", "id": "task", "feed": null},
  "task.memo.list": false,
  "task.comment.recipients": {"resource": "comment-recipients", "feed": "tasks"},
  "task.view.list": {"resource": "task-views", "feed": null},
  "task.board": {"resource": "board", "feed": "tasks"},
};

/** x-app-write of each write (x-app in core.json). */
export const TASKS_APP_WRITES: { [K in "task.new" | "task.handoff" | "task.turn.pop" | "task.done" | "task.reopen" | "task.progress" | "task.begin" | "task.start" | "task.seen" | "task.comment.add" | "task.memo.add" | "task.comment.edit" | "task.memo.edit" | "task.comment.delete" | "task.comment.send_now" | "task.send_now" | "task.memo.delete" | "task.view.save" | "task.view.delete" | "task.queue" | "task.draft" | "task.merge" | "task.release" | "task.request" | "task.wait" | "task.discard" | "task.withdraw" | "task.edit" | "task.ask"]: { kind: WriteKind; label: string } | false } = {
  "task.new": {"kind": "authored", "label": "new task"},
  "task.handoff": false,
  "task.turn.pop": {"kind": "decided", "label": "give back"},
  "task.done": {"kind": "authored", "label": "done"},
  "task.reopen": {"kind": "authored", "label": "reopen"},
  "task.progress": false,
  "task.begin": false,
  "task.start": false,
  "task.seen": {"kind": "decided", "label": "mark seen"},
  "task.comment.add": {"kind": "authored", "label": "add comment"},
  "task.memo.add": false,
  "task.comment.edit": {"kind": "authored", "label": "edit comment"},
  "task.memo.edit": false,
  "task.comment.delete": {"kind": "authored", "label": "delete comment"},
  "task.comment.send_now": {"kind": "decided", "label": "send now"},
  "task.send_now": {"kind": "decided", "label": "run this one now"},
  "task.memo.delete": false,
  "task.view.save": {"kind": "decided", "label": "save view"},
  "task.view.delete": {"kind": "decided", "label": "delete view"},
  "task.queue": {"kind": "authored", "label": "queue"},
  "task.draft": {"kind": "authored", "label": "draft"},
  "task.merge": {"kind": "authored", "label": "merge"},
  "task.release": {"kind": "authored", "label": "release"},
  "task.request": {"kind": "decided", "label": "make a request"},
  "task.wait": false,
  "task.discard": {"kind": "decided", "label": "discard"},
  "task.withdraw": {"kind": "authored", "label": "withdraw"},
  "task.edit": {"kind": "authored", "label": "edit"},
  "task.ask": {"kind": "decided", "label": "ask"},
};
