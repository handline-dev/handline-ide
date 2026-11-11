// Generated from contract/spec/ by contract/generate.py; do not edit.
/* eslint-disable */
import type { Accepted, AgentName, AttachmentId, Duration, ImagePart, Prompt, ReadCursor, RequestId, RequestName, RequestRef, Role, TextPart, Timestamp, Topic, WriteKind } from './core.ts';

export type InboxItem = {
  id: number;
  ref?: RequestRef;
  kind: string;
  agent: AgentName | null;
  title: string;
  body: string;
  evidence: string | null;
  task: number | null;
  created: Timestamp;
  read: boolean;
  prompt: Prompt | null;
  /** Only on a card the daemon raised after its own recovery failed: the action to offer as a button. open_terminal opens the item's agent's terminal, since every step the daemon could take was taken. */
  fix?: "open_terminal";
  /** Only on an ask an agent's task comment raised by naming `@human` (task.comment.add): that comment's id on `task`. A client opens the comment itself. Absent on any other item. */
  memo?: number;
};

/** Landing state of a message; open set. */
export type Landing = "queued" | "typed" | "delivered" | "withdrawn";

/** One request between the person and an agent. */
export type ChatMessage = {
  /** Request id. */
  id: number;
  ref?: RequestRef;
  at: Timestamp;
  /** When the request entered its current state (queued, typed, landed, withdrawn...). Never null; the creation time until it first changes. */
  state_at: Timestamp;
  from: string;
  to: string;
  state: string;
  landing: Landing;
  sending_reason: string | null;
  body: string;
  delivered_at: Timestamp | null;
  typed_at: Timestamp | null;
  landed_at: Timestamp | null;
  parent: number | null;
  /** The words of the parent message this one quotes, when it was sent with quote: as chat.send kept them, whitespace collapsed. Absent with no quote. */
  quote?: string;
  kind: string;
  tasks: {
    id: number;
    title: string;
  }[];
  /** Only in chat.search hits: the agent whose chat shows the message. */
  agent?: string;
  /** Only in chat.search hits: which of that agent's chats shows it (the jump is chat.locate). */
  scope?: "mine" | "all";
  /** Only for a message to a structured agent, while it is drawn (a withdrawn one is not). Changes ride the chat event. See x-delivery. */
  receipt?: Receipt;
  /** Every attachment of the message (images, files, folders), in order: those the body names that are still kept, then those sent in `attachments`. Absent when it has none. */
  attachments?: Attachment[];
  /** The paths the founder sent by reference (chat.send refs), as sent. Absent when it has none. */
  refs?: string[];
  /** `body` cut at the images it names, in order, then one part per image sent with it in `attachments` that the body does not name (an empty caption); present only when the message has an image. The text parts joined give `body` with each reference left out. */
  parts?: (TextPart | ImagePart)[];
  parent_item?: {
    /** The transcript item id. chat.transcript with since=id reads the page that shows it. */
    id: number;
    /** The reply's first 300 characters (Unicode code points, no ellipsis) when the message was sent. */
    text: string;
  } | null;
  /** Only on the message that tells an agent how the founder decided its proposal: that proposal's id, as proposal.list gives it. A client that draws the decided proposal as a card can leave this line out. */
  proposal?: number;
  /** One-tap buttons for a message to the founder that waits on a decision about a task it is tied to. Present only while that task is owned by the founder and open (OK, No). Never read from the words; gone once the task changes, so a client refetches when a task event arrives. A tap calls `verb` with `params` (the client adds request_id); an action with `needs` takes that param from the person first. No answer state: each is an ordinary task verb. */
  actions?: ChatAction[];
  /** On a message of the founder's to an agent: the request task made from it (task.request); null for none. */
  request_task?: number | null;
  /** The runs of this message's command blocks, one per block, oldest start first. Absent when none was started. Phone and PC draw 'Running · 4s', 'Done · exit 0 · 3s', 'Failed · exit 1 · 3s' or 'Stopped' from it. */
  runs?: CommandRun[];
};

export type TranscriptItem = {
  /** Its time and its source's row, so it never changes. An item that arrives in a second a client has read up to can sort before the last one it has: `since` reads from that second and a client drops the ids it already holds. For kind reply it is also what chat.send takes as parent_item. A reply has one of two sources, told apart by the last digit of its number within the second: a structured session's text block (even; the same id on every read and in the live event's `item`) and the answer to a request (odd); each is the same on every read, and chat.transcript lists both. Source numbers within the second: 0 reply, 1 prompt, 2 task event, 3 tool call, 4 session start (its number is the session_starts row), 5 bash command (its number is its place in the session's transcript). */
  id: number;
  at: Timestamp;
  kind: "reply" | "tool" | "prompt" | "task_event" | "session" | "task_batch" | "bash" | "task_comment";
  text?: string;
  tool?: string;
  /** For kind tool: the text the tool printed, as the store holds it. Absent when the store holds none. */
  output?: string;
  /** For kind tool or bash: how many lines the store cut from the end of the output (it keeps the start). 0 or absent when nothing was cut. */
  dropped_lines?: number;
  /** Kind reply only: the runs of this reply's command blocks (shell.open block "r<id>:<n>"), one per block, oldest start first, as ChatMessage.runs. Absent when none was started. A start and an end send the item again as a chat.transcript event. */
  runs?: CommandRun[];
  /** Prompt card; `open` says whether it can still be answered. */
  prompt?: Prompt;
  open?: boolean;
  /** For kind prompt, once it is answered or closed: what it got. Answered through Handline (prompt.answer): `label` is the words for the choice (Allowed or Denied for a permission; the picked options' labels, joined by ", ", for a question, or Other for typed text; the option's label for a dialog; Cancelled for Esc), `note` the typed text or a deny's reason when there is one, `by` who answered (a person or a paired device, by name) and `at` when. Closed any other way (answered in the terminal, or gone): `outside` true and `at` when the daemon saw it close. Absent while the prompt is open and unanswered. */
  answer?: {
    label: string;
    note?: string;
    by: string;
    at: Timestamp;
  } | {
    outside: true;
    at: Timestamp;
  };
  task_event?: {
    /** unblocked: its last open blocker (closed_task) closed; who closed it. In the chat of the agent acting on the task. */
    event: "opened" | "changed" | "done" | "withdrawn" | "unblocked";
    task: number;
    request: number | null;
    evidence: string | null;
    /** Who did it: the agent (or human) that opened, changed, closed or withdrew the task, as ChatRequestItem.who. */
    who: string;
    /** unblocked only: the blocker whose close freed the task. */
    closed_task?: number;
    /** What a `changed` event changed, as chat.requests words it: `assigned to X, priority 2 -> 1`, `title changed`, `handed to X`. Absent when the event word says it all. An open set: a client draws a word it does not know as text. */
    change?: string;
  };
  /** For kind session: a conversation of this agent began here, or it was restarted into the same one. Listed for every session start that did not resume the old conversation, and for a resume whose cause is `restart`, `settings_applied` or `move` or that changed a setting, once its session id is known (a fresh launch has none for its first seconds). When one begins the daemon publishes the item as chat.transcript, as it does a reply. */
  session?: {
    id: string;
    started: Timestamp;
    handoff: string | null;
    /** Why the conversation began. Words the daemon writes: `launch` (first start of a spawned agent), `start` (work arrived for a closed agent), `wake` (a closed agent woken), `restart`, `fresh close` (the old conversation was closed and not resumed; the one word with a space), `move` (to another account), `handoff` (folded into a handoff), and, where the daemon knows it, `context_full`, `settings_applied`, `crash_recovery`, `idle_close`. Open set: a client draws a word it does not know as text. Absent from a daemon that does not serve it. */
    cause?: string;
    /** The session went on with the same conversation (a restart, not a new conversation). Absent means false. */
    resumed?: boolean;
    /** What the settings of this start changed from the session before, in words a person reads: `model changed (opus-5-5 -> sonnet-5-5), team rules changed (shared/team/handline.md)`. Empty when nothing did. Absent means empty. */
    changed?: string;
  };
  /** For kind bash: a person's ! line as it ran, without the !. */
  command?: string;
  /** For kind bash: what it printed, cut as a tool's output is, with dropped_lines saying how much. A command that succeeds can have its stderr here. Absent until done. */
  stdout?: string;
  /** For kind bash: as stdout. The transcript records no exit code, and neither field says whether the command failed. */
  stderr?: string;
  /** For kind bash: its output is recorded. The live event says the same id again when it is. */
  done?: boolean;
  /** For kind bash: the id of the person's message that sent this line, to draw the two as one row. Absent when none is known, for a line typed at the terminal or sent before messages recorded their landing. */
  request?: number;
  /** For kind task_batch: a batch discard, shown once in the chat of whoever made it. */
  task_batch?: {
    batch: string;
    who: string;
    tasks: {
      task: number;
      title: string;
      reason: string;
    }[];
  };
  /** For kind task_comment: a task comment the founder is part of, as a card in an agent's chat. turn: the founder's turn put on the task, in the chat of the agent below it; answer: the founder's comment that gave the turn back, in the chat of the agent it went back to; note: an `@human` comment (news, no turn), in its author's chat. The card opens the task; the answer is written there. */
  task_comment?: {
    task: number;
    /** The comment (Memo.id); null for a turn put on with no comment (Turn.memo), whose text is the turn's ask. */
    memo: number | null;
    kind: "turn" | "answer" | "note";
    /** Who wrote it: the agent, or human for an answer. */
    who: string;
    /** The comment's text as it reads now (edited or not); empty when it was deleted. */
    text: string;
  };
};

export type Working = {
  turn_running: boolean;
  line: string | null;
  since: Timestamp | null;
};

export type StandingAnswer = {
  /** trust: a folder trusted for every session; dialog: a rule that answers a vendor's dialog. */
  kind: "trust" | "dialog";
  /** What answer.remove takes: [folder] for trust, [vendor, title, label] for dialog. */
  key: string[];
  text: string;
};

export type ChatRequestItem = {
  type: "request" | "task_event" | "session";
  /** Where it sits in time: a request where its agent read it (landed_at) or else when it was made. */
  at: Timestamp;
  /** request only. */
  message?: ChatMessage;
  task_state?: string | null;
  /** task_event only. */
  task?: number;
  /** task_event only: the task's title. */
  title?: string;
  /** task_event only: who did it. */
  who?: string;
  /** task_event only: `queued -> doing`, `handed to X`, `assigned to X`, `title changed`, `priority 2 -> 1`. An open set: a client draws a word it does not know as text. */
  change?: string;
  /** task_event only: the text typed with a handoff, else empty. Text from an agent. */
  note?: string;
  request?: number | null;
  /** task_event only: the ref of the delivery in request. */
  request_ref?: RequestRef | null;
  /** session only: why a new conversation started or the session restarted (restart, wake, fresh close, move, handoff, launch). An open set. */
  cause?: string;
  /** session only: it went on with the same conversation. Absent means false. */
  resumed?: boolean;
  /** session only: what its settings changed, in words a person reads; empty when nothing did. Absent means empty. */
  changed?: string;
  handoff?: string | null;
};

/** Where a message to a structured agent is on its way into the CLI. Only for an agent whose AgentView.structured is true; see x-delivery. */
export type Receipt = {
  /** queued: held by the daemon behind a running turn, not written to the CLI. sent: written to the CLI, not yet echoed. confirmed: the CLI echoed the message's uuid. unknown: written, and the daemon or the CLI ended before the echo; never written again. */
  state: "queued" | "sent" | "confirmed" | "unknown";
  /** The message's uuid, derived by the daemon from the request_id; the id the transcript stream's turn_start carries. Read it, do not compute it. */
  uuid: string;
  /** The seq of the transcript event that carries this uuid, when there is one; absent otherwise (a message that joined a running turn has no turn_start of its own). Best effort; a client does not wait for it. */
  seq?: number;
};

/** A file the person attached to a message: an image, any other file, or a folder of files (x-attachments, folders). The file lives on the daemon's machine; a client draws an image with attachment.get. */
export type Attachment = {
  id: AttachmentId;
  /** The file name the client gave, with no folder in it (the daemon cuts one off). Shown to the person; never used as a path. */
  name: string;
  /** The type the file was put with. For a PNG, JPEG, GIF or WebP it is the type its first bytes say; for any other file it is the type the client gave, kept as sent. A folder is inode/directory. */
  mime: string;
  /** The file's size; for a folder, the sum of its files so far. */
  bytes: number;
  /** The picture's width in pixels as it is drawn (a JPEG's Exif orientation applied), read from its header when it was put, so a client can size it before it arrives. Absent when the header could not be read, or for an image put before it was kept. */
  width?: number;
  /** The picture's height in pixels; present exactly when width is. */
  height?: number;
  /** Only for a folder (mime inode/directory): how many files it holds so far. */
  files?: number;
};

export type ChatAction = {
  key: "ok" | "no";
  label: string;
  verb: "task.done" | "task.handoff";
  task: number;
  /** The verb's params but request_id. */
  params: { [key: string]: unknown };
  /** A param the person must write before the tap sends. */
  needs?: string;
};

/** One row of the founder's requests. A request is a task whose reporter is the founder (a task opened by them or made from their message), top of its tree. */
export type RequestRow = {
  /** task: a request the founder opened (task.request, or a task of their own for a lead). A message of theirs that is not a task is the chat's, not a request. */
  kind: "task";
  /** The request's task; always set (null only from a daemon that still lists messages). */
  task: number | null;
  /** The founder's message it was made from; null for a task opened without one. */
  ref: RequestRef | null;
  title: string;
  /** Read from the request's tree of tasks, never stored; the first that holds. waiting_on_you: a task assigned to the founder or a decision card waiting on them, anywhere in the tree. review: an open task has a PR. doing: some task in the tree is open. done: the request's own task has ended, done or discarded (ended says which; the agent that holds it judges and closes it; the founder does not). */
  status: "waiting_on_you" | "review" | "doing" | "done";
  /** Who holds the request. */
  agent: string;
  /** The agent the message it was made from was sent to. Only on a task made from a message. */
  origin?: string;
  /** Agents holding an open task in the tree; empty once done. */
  holders: string[];
  /** Open tasks under the request's own. */
  open: number;
  /** Ended tasks under the request's own. */
  done: number;
  /** PRs of the open tasks in the tree. */
  prs: number[];
  /** The agent holding the open task with the latest activity under the request (the one at work now); null once done or when the founder holds the open one. */
  now: string | null;
  /** The open task named by `now` waits on another open task (task_blocks, not dormant). */
  blocked: boolean;
  /** The request's state in one line for the list, from the same facts as the fields: `sub 3/5 done, now daemon-core, in review` (subtasks done of all under the request's own task; the agent at work now; `in review` with a PR, `waiting on you`, `blocked`). Without subtasks `now daemon-core, in review`; `done` once done. */
  progress: string;
  /** Why the request needs the founder's eye; null when nothing calls for it and once done. Absent from a daemon that does not send it. */
  attention?: RequestAttention | null;
  /** Agents' comments on the request that the founder has not read (Task.unread). */
  unread_comments: number;
  created: Timestamp;
  last_activity: Timestamp;
  /** Orders the list; pass the last row's as requests.list before. */
  cursor: string;
  /** Set when status is done; null otherwise. Absent from a daemon that does not send it. */
  ended?: RequestEnded | null;
  [key: string]: unknown;
};

/** One run of a command block of a message or of an agent's reply (shell.open command and block): the last run only. */
export type CommandRun = {
  /** "<message id>:<n>" for a chat message's block or "r<item id>:<n>" for a reply item's, as shell.open took it. */
  block: string;
  /** The run's window, as terminal.attach takes it; gone once the window is closed. */
  terminal: string;
  started_at: Timestamp;
  /** running: the command has not ended. done: it ended with exit 0. failed: it ended with another exit status. stopped: its window was closed before it ended, so there is no exit status. */
  state: "running" | "done" | "failed" | "stopped";
  /** The command's exit status; only when state is done or failed. A command ended by a signal reads 128 plus the signal, as a shell says it. */
  exit?: number;
  /** Seconds from start to end; only when state is done or failed. */
  seconds?: number;
};

/** Why a request needs the founder's eye (RequestRow.attention), read from facts of its tree and stored nowhere. The thresholds are settings; it only labels the row. */
export type RequestAttention = {
  /** The first that holds, in this order. overdue: an open tree task is past its time box (Task.time.overdue_since). bounced: an open tree task was sent back (reopened, not restored from discarded, or a turn on it sent back by task.turn.pop, not by a retiring agent) attention_sent_back times or more. waiting: an open tree task, held by no open blocker and no decision card, has waited on cap, wall or owner (Task.waiting.on) for attention_wait_s or more. quiet: no tree task's clock runs and nothing in the tree has moved for attention_quiet_s or more. */
  kind: "overdue" | "bounced" | "waiting" | "quiet";
  /** The line to show, with no elapsed time in it: `T8504 sent back 2 times`, `T8513 waiting on owner`, `no recent activity`. */
  text: string;
  /** The tree task it is about; the request's own for quiet. */
  task: number;
};

/** How a finished request ended, and whether the founder has seen it since. */
export type RequestEnded = {
  outcome: "done" | "discarded";
  at: Timestamp;
  /** True once task.seen has been called on the task since it ended, when the founder ended it themselves, or when it ended before this daemon first kept the mark. */
  seen: boolean;
};

export interface ChatMethods {
  "chat.messages": { params: {
  agent: AgentName;
  /** mine: person and this agent; all: everything it sent or received. */
  scope: "mine" | "all";
  before?: number;
  since?: number;
  limit?: number;
}; result: {
    messages: ChatMessage[];
    more: boolean;
  } & ReadCursor };
  "chat.requests": { params: {
  agent: AgentName;
  /** mine: person and this agent; all: everything it sent or received. */
  scope: "mine" | "all";
  /** Request id: read the page just older than it. */
  before?: number;
  /** Request id: read every request from it on (no `more`); how a window already drawn is brought up to date. Not with `before`. */
  since?: number;
  /** Requests in a page; default 10. */
  limit?: number;
}; result: {
    items: ChatRequestItem[];
    /** Older requests remain (never true with `since`). */
    more: boolean;
    /** Opaque: equal on two reads when nothing in the chat changed between them (a request made, closed, changed, delivered or landed, a task event, a session start). Cheap to compare; a client that sees it change reads again. */
    version: string;
    /** The newest stream cursor when this was read: subscribe with since=cursor and topics [chat] to hear what changes after it. */
    cursor: number;
  } };
  "chat.transcript": { params: {
  agent: AgentName;
  before?: number;
  since?: number;
  limit?: number;
}; result: {
    items: TranscriptItem[];
    more: boolean;
    working: Working;
  } & ReadCursor };
  "chat.locate": { params: {
  request: RequestName;
  /** Optional: the daemon finds the conversation from the message itself. */
  agent?: AgentName;
  /** Page size, default 50. */
  limit?: number;
}; result: {
    /** The agent whose chat shows it. */
    agent: AgentName;
    /** mine: person and this agent; all: everything it sent or received. */
    scope: "mine" | "all";
    message: ChatMessage;
    /** A page, newest first, containing message; continue with chat.messages before (older) or since (newer). */
    messages: ChatMessage[];
    more_older: boolean;
    more_newer: boolean;
    /** chat.transcript's items for the agent from the second of the page's oldest message to the end of its newest's (on to now when more_newer is false), oldest first: up to 200, kept around message. */
    transcript: TranscriptItem[];
    /** The span has older items than transcript holds; continue with chat.transcript before the first item's id. */
    transcript_more_older: boolean;
    /** The span has newer items than transcript holds; continue with chat.transcript since the last item's id. */
    transcript_more_newer: boolean;
  } };
  "chat.refs": { params: {
  agent: AgentName;
}; result: {
    /** Up to 30, newest first: every request the agent sent or received that was not withdrawn or turned away. */
    requests: {
      request: number;
      from: string;
      to: string;
      /** The first line of the body, at most 60 characters. */
      line: string;
    }[];
  } };
  "chat.search": { params: {
  agent?: AgentName;
  query: string;
  /** mine: person and this agent; all: everything it sent or received. */
  scope?: "mine" | "all";
  /** Only messages whose other party is this name: an agent, or `human` for the person. When one agent's messages are searched (`agent`, or the caller for an agent), the party other than that agent; when the person searches every agent (no `agent`), messages from or to the name. A name no message has finds nothing. */
  with?: string;
  /** Only messages sent at or after this time; one that is not a timestamp is bad_request (field since). */
  since?: Timestamp;
  limit?: number;
}; result: {
    messages: ChatMessage[];
  } };
  "chat.send": { params: {
  agent: AgentName;
  /** Markdown. It may name images (x-attachments, texts), with or without `task`; with `task` it is the task's brief. */
  body: string;
  task?: {
    title: string;
    time_box?: Duration;
  };
  /** The message this one replies to: a reference, for any message whatever it asked of the person. The message is only a say to the agent with the parent link; it answers, settles or changes nothing about the parent. Not with task. A message that does not exist is not_found. The message goes to agent whoever the parent's requester or target is, so a row relayed from another agent can be quoted in this agent's chat. request in the result is the new message. */
  parent?: RequestName;
  /** The words of the parent message this one answers, picked by the person: kept with the message (ChatMessage.quote) and delivered as a `> ` line, then a blank line, above the body. Whitespace is collapsed, and then it must appear in the parent's body, whitespace collapsed the same way, so a quote never shows words the parent did not say. Only with parent; alone, with parent_item or task, empty, longer than 1000 characters as sent, or not in the parent, it is bad_request, field quote. */
  quote?: string;
  /** Attachments from attachment.put (images, files, folders) to send with this message, drawn after those the body names (x-attachments, flow); an id the body names too counts once, and the two together are at most 6 (bad_request, field attachments when the list goes over). Then body may be empty. An id may be sent again. A malformed id or an unfinished upload or a folder with no file finished is bad_request, field attachments; an attachment never put or no longer kept is not_found, what attachment. Not with task (bad_request): a brief names its images in its text. */
  attachments?: AttachmentId[];
  /** Files and folders on the daemon's machine named by absolute path, not copied (x-attachments, refs). Founder only: another caller is refused, gate role. Counted with attachments, at most 6 in all (bad_request, field refs). Then body may be empty. Not with task (bad_request, field refs). With chat_refs_need_grant on, a ref the agent may not read is refused, gate access_needed (x-access). */
  refs?: string[];
  /** A reply of the agent's own that this message answers: the `id` of an item of kind reply that chat.transcript lists for this agent (a structured session's text block, whose live event carries the same id as `item`, or the answer to a request). The agent's turn text has no message number, so it cannot be `parent`; the two do not go together, nor does this with task (bad_request, field parent_item). It may go with attachments. An id that is no item of this agent's transcript, or one of another agent's, is not_found (what: item); one that is not a reply (a tool call, a prompt, a task event, a session) is bad_request. */
  parent_item?: number;
  /** For a structured agent whose turn is running: now joins the running turn, queue (the default) is held by the daemon and written when the turn ends, one message per turn. With no turn running they are the same. A terminal agent accepts the field and ignores it. Another word is bad_request. */
  delivery?: "now" | "queue";
  request_id: RequestId;
}; result: Accepted & {
    request: number;
    receipt?: Receipt;
  } };
  "chat.send_now": { params: {
  /** A ChatMessage.id still in state queued. */
  request: RequestName;
  request_id: RequestId;
}; result: Accepted & {
    receipt?: Receipt;
  } };
  "chat.cancel": { params: {
  request: RequestName;
  /** Needed once the agent has been told the message. Without it the call is refused (gate confirm) and error data.plan holds what cancelling would do and who is told; send it again with confirm true to do it. */
  confirm?: boolean;
  request_id: RequestId;
}; result: Accepted };
  "chat.seen": { params: {
  agent: AgentName;
  /** The highest message number that has been in view. A lower number than before changes nothing: what arrives later has a higher one, so nothing unseen is hidden. */
  up_to: number;
  request_id: RequestId;
}; result: Accepted };
  "attachment.put": { params: {
  name: string;
  /** The file's type, as a MIME type (`application/octet-stream` when the client does not know). PNG, JPEG, GIF and WebP are checked against the first bytes; any other type is kept as sent. */
  mime: string;
  /** The whole file's size, the same in every chunk of one upload. */
  bytes: number;
  /** Where this chunk starts. Default 0. */
  offset?: number;
  /** From the first reply. Absent on the chunk at offset 0 that opens the upload. */
  upload?: string;
  /** The chunk, at most 512 KiB of file. */
  data: string;
  /** A folder from attachment.folder this file goes in. Only on the chunk at offset 0, with rel_path. */
  folder?: AttachmentId;
  /** The file's path inside the folder, `/` between parts: no empty, `.` or `..` part, no leading `/`. folder and rel_path go together, only on the chunk at offset 0: one without the other, or either on a later chunk, is bad_request with that field; a path the folder already holds is bad_request, field rel_path. */
  rel_path?: string;
  request_id: RequestId;
}; result: Accepted & {
    upload: string;
    /** Bytes held so far. */
    received: number;
    /** Only on the chunk that completes the file. */
    attachment?: Attachment;
  } };
  "attachment.folder": { params: {
  /** The folder's name as dropped, shown to the person. */
  name: string;
  request_id: RequestId;
}; result: Accepted & {
    /** mime inode/directory, bytes 0, files 0. */
    attachment: Attachment;
  } };
  "attachment.get": { params: {
  id: AttachmentId;
  /** A copy with its longest side at most 320 px, as PNG or JPEG, in one reply (at most 256 KiB). Default false. */
  thumb?: boolean;
  /** Where to read the original from. Default 0. Not with thumb. */
  offset?: number;
}; result: {
    /** The file's type, or the thumbnail's when thumb. */
    mime: string;
    /** The size of the whole original, or of the thumbnail when thumb. */
    bytes: number;
    offset: number;
    /** At most 512 KiB of file; the rest is another call at offset + the length read. */
    data: string;
    /** True when this reply reaches the end. */
    done: boolean;
  } };
  "attachment.remove": { params: {
  id: AttachmentId;
  /** Remove nothing: answer removed_from alone, for the confirm. */
  dry_run?: boolean;
  request_id: RequestId;
}; result: Accepted & {
    /** How many chat messages, task texts and memos name it or carry it, each once; 0 when it was removed already. */
    removed_from: number;
  } };
  "inbox.list": { params: {
  unread?: boolean;
  kind?: string;
  agent?: AgentName;
  /** Return rows older than this id. */
  before?: number;
  /** Default 50, at most 200. */
  limit?: number;
  /** Hold the call up to this many seconds while no item matches the other params (unread, kind, agent, before; read items count unless unread is true), and return when one arrives (an empty list on timeout). Default 0, at most 600; the call is in flight and the connection is not idle. */
  wait_s?: number;
}; result: {
    items: InboxItem[];
    more: boolean;
  } & ReadCursor };
  "inbox.queue": { params: {
  /** An agent's name, or `human` for the founder's mailbox. Default: the caller's own. */
  whose?: string;
  /** Add `history` (an agent's inbox) or `dealt` (the founder's). Default false. */
  all?: boolean;
  /** With `wait_s`: a `newest` from an earlier reply. The call holds until something newer waits. Default 0: it holds while nothing waits. */
  since?: number;
  /** Hold the call up to this many seconds while `newest` is not above `since`. On timeout the reply is the inbox as it is, `newest` unchanged. Default 0, at most 600. */
  wait_s?: number;
}; result: {
    whose: string;
    /** The number of the newest thing waiting in this inbox, 0 for nothing: for an agent a request no state has settled, for the founder a mailbox row. */
    newest: number;
    /** The head of an agent's queue; null when the queue is empty and for the founder's mailbox. */
    head: {
      id: number;
      requester: string;
      chain_depth: number;
      body: string;
      /** How many requests wait in the queue behind this one. */
      more_waiting: number;
      /** The sentence that says so and when the next one comes; empty when none wait. */
      more_waiting_line: string;
      /** What the agent does to close it: the reply that answers it, or for a task the hand-on and done commands. */
      how_to_close: string;
    } | null;
    /** An agent's inbox with `all`: every request sent to it, oldest first. */
    history?: {
      id: number;
      requester: string;
      state: string;
      chain_depth: number;
      /** The body as one line, cut to 60 characters. */
      line: string;
    }[];
    /** The founder's mailbox: how many requests and answers wait, before folding. */
    waiting?: number;
    /** The founder's mailbox: decisions first, then the rest, oldest first in each. */
    lines?: {
      /** The newest request the line stands for. */
      id: number;
      from: string;
      kind: string;
      /** Only the founder can decide it. */
      decision: boolean;
      /** It is an answer to something the founder asked; `id` is the founder's request. */
      answer: boolean;
      /** Its first line, cut to 60 characters. */
      line: string;
      /** How many unseen messages of that agent the line stands for. */
      unread: number;
    }[];
    /** The founder's mailbox with `all`: newest first, without what `lines` still lists. */
    dealt?: {
      id: number;
      /** The founder sent it; `agent` is whom it went to. */
      outgoing: boolean;
      agent: string;
      state: string;
      /** The first line of what closed it, else of its body, cut to 60 characters. */
      line: string;
    }[];
  } };
  "inbox.get": { params: {
  id: RequestName;
  /** Hold the call up to this many seconds until the request settles. Default 0, at most 600. */
  wait_s?: number;
}; result: InboxItem & ReadCursor };
  "request.read": { params: {
  id: RequestName;
  /** Hold the call up to this many seconds while the request is queued. Default 0, at most 600. */
  wait_s?: number;
}; result: {
    id: number;
    ref?: RequestRef;
    requester: string;
    target: string;
    /** ask, task, say and the like; a request with none is an ask. */
    kind: string;
    /** queued until it settles; then landed, or denied, withdrawn or replaced for one that never reached its target. */
    state: string;
    created: Timestamp;
    chain_depth: number;
    parent: {
      id: number;
      ref?: RequestRef;
      /** The start of the parent's first line, at most 40 characters and an ellipsis. */
      line: string;
    } | null;
    body: string;
    /** What came back, or for a request that never reached its target why it closed. Null while there is none. */
    answer: string | null;
    /** `body` cut at the images it names, in order, then one part per image sent with it in `attachments` that the body does not name (an empty caption); present only when the message has an image. The text parts joined give `body` with each reference left out. */
    parts?: (TextPart | ImagePart)[];
    /** The paths the founder sent by reference (chat.send refs), as sent, and whether each was a folder then. Absent when it has none. */
    refs?: {
      path: string;
      folder: boolean;
    }[];
  } };
  "request.withdraw": { params: {
  id: RequestName;
  /** Why, kept with the withdrawal. Not empty when given. */
  reason?: string;
  request_id: RequestId;
}; result: {
    /** Whether the request was withdrawn. */
    ok: boolean;
    /** What was withdrawn, or why nothing was. */
    said: string;
  } };
  "request.cancel": { params: {
  id: RequestName;
  /** Change nothing: answer `plan`. */
  dry_run?: boolean;
  request_id: RequestId;
}; result: {
    /** Dry run: whether the request can be cancelled. Otherwise: whether it was. */
    ok: boolean;
    /** Dry run only: what cancelling would do and who is told, or why it cannot be cancelled. */
    plan?: string;
    /** Without dry_run: what was cancelled and who was told, or `not cancelled - ` and the reason. */
    said?: string;
  } };
  "inbox.mark_read": { params: {
  ids: RequestName[];
  request_id: RequestId;
}; result: Accepted };
  "inbox.reply": { params: {
  id: RequestName;
  /** Markdown. It may name images (x-attachments, texts). */
  text: string;
  request_id: RequestId;
}; result: Accepted };
  "requests.list": { params: {
  state?: "waiting_on_you" | "review" | "doing" | "done";
  /** true: only finished requests the founder has not seen. The bell's list. */
  unseen?: boolean;
  /** A row's cursor: the rows after it. */
  before?: string;
  limit?: number;
}; result: {
    items: RequestRow[];
    more: boolean;
  } };
  "answers.list": { params: { [key: string]: unknown }; result: {
    answers: StandingAnswer[];
  } };
  "answer.add": { params: {
  /** A folder to trust; must exist. Give folder, or the three rule fields. */
  folder?: string;
  vendor?: string;
  /** The dialog's title; a long command's truncated title is refused. */
  title?: string;
  /** The option label to choose. */
  label?: string;
  request_id: RequestId;
}; result: Accepted };
  "answer.remove": { params: {
  kind: "trust" | "dialog";
  key: string[];
  request_id: RequestId;
}; result: Accepted };
}

export const CHAT_METHODS: { [K in keyof ChatMethods]: { role: Role; callers: Role[] | null; write: boolean; since: number } } = {
  "chat.messages": { role: "member", callers: null, write: false, since: 1 },
  "chat.requests": { role: "member", callers: null, write: false, since: 1 },
  "chat.transcript": { role: "member", callers: null, write: false, since: 1 },
  "chat.locate": { role: "member", callers: null, write: false, since: 1 },
  "chat.refs": { role: "member", callers: null, write: false, since: 1 },
  "chat.search": { role: "agent", callers: null, write: false, since: 1 },
  "chat.send": { role: "member", callers: null, write: true, since: 1 },
  "chat.send_now": { role: "member", callers: null, write: true, since: 1 },
  "chat.cancel": { role: "member", callers: null, write: true, since: 1 },
  "chat.seen": { role: "member", callers: null, write: true, since: 1 },
  "attachment.put": { role: "member", callers: null, write: true, since: 1 },
  "attachment.folder": { role: "member", callers: null, write: true, since: 1 },
  "attachment.get": { role: "member", callers: null, write: false, since: 1 },
  "attachment.remove": { role: "founder", callers: null, write: true, since: 1 },
  "inbox.list": { role: "agent", callers: null, write: false, since: 1 },
  "inbox.queue": { role: "agent", callers: null, write: false, since: 1 },
  "inbox.get": { role: "agent", callers: null, write: false, since: 1 },
  "request.read": { role: "agent", callers: null, write: false, since: 1 },
  "request.withdraw": { role: "agent", callers: null, write: true, since: 1 },
  "request.cancel": { role: "founder", callers: null, write: true, since: 1 },
  "inbox.mark_read": { role: "agent", callers: null, write: true, since: 1 },
  "inbox.reply": { role: "agent", callers: null, write: true, since: 1 },
  "requests.list": { role: "member", callers: null, write: false, since: 1 },
  "answers.list": { role: "founder", callers: null, write: false, since: 1 },
  "answer.add": { role: "founder", callers: null, write: true, since: 1 },
  "answer.remove": { role: "founder", callers: null, write: true, since: 1 },
};

/** x-app-cache of each read (x-app in core.json). */
export const CHAT_APP_CACHE: { [K in "chat.messages" | "chat.requests" | "chat.transcript" | "chat.locate" | "chat.refs" | "chat.search" | "attachment.get" | "inbox.list" | "inbox.queue" | "inbox.get" | "request.read" | "requests.list" | "answers.list"]: { resource: string; id?: string; feed: Topic | null } | false } = {
  "chat.messages": {"resource": "chat-messages", "feed": "chat"},
  "chat.requests": {"resource": "chat-requests", "feed": null},
  "chat.transcript": {"resource": "chat-transcript", "id": "agent", "feed": "chat"},
  "chat.locate": {"resource": "chat-locate", "feed": null},
  "chat.refs": {"resource": "chat-refs", "id": "agent", "feed": null},
  "chat.search": {"resource": "chat-search", "feed": null},
  "attachment.get": false,
  "inbox.list": {"resource": "inbox", "feed": "inbox"},
  "inbox.queue": false,
  "inbox.get": {"resource": "inbox-item", "id": "id", "feed": "inbox"},
  "request.read": false,
  "requests.list": {"resource": "requests", "feed": "requests"},
  "answers.list": {"resource": "answers", "feed": null},
};

/** x-app-write of each write (x-app in core.json). */
export const CHAT_APP_WRITES: { [K in "chat.send" | "chat.send_now" | "chat.cancel" | "chat.seen" | "attachment.put" | "attachment.folder" | "attachment.remove" | "request.withdraw" | "request.cancel" | "inbox.mark_read" | "inbox.reply" | "answer.add" | "answer.remove"]: { kind: WriteKind; label: string } | false } = {
  "chat.send": {"kind": "authored", "label": "send"},
  "chat.send_now": {"kind": "decided", "label": "send now"},
  "chat.cancel": {"kind": "decided", "label": "withdraw"},
  "chat.seen": {"kind": "decided", "label": "mark seen"},
  "attachment.put": {"kind": "decided", "label": "attach image"},
  "attachment.folder": {"kind": "decided", "label": "attach folder"},
  "attachment.remove": {"kind": "decided", "label": "delete the photo"},
  "request.withdraw": false,
  "request.cancel": false,
  "inbox.mark_read": {"kind": "authored", "label": "mark read"},
  "inbox.reply": {"kind": "authored", "label": "reply"},
  "answer.add": {"kind": "authored", "label": "add standing answer"},
  "answer.remove": {"kind": "authored", "label": "remove standing answer"},
};
