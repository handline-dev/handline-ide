// Generated from contract/spec/ by contract/generate.py; do not edit.
import type { Topic } from '@handline/client';

/** The cache entry of each read (x-app-cache): its name in the key, the param that names the entry, the topic that keeps it fresh. */
export const reads = {
  "daemon.status": { resource: "status", feed: "status" },
  "load.get": { resource: "load" },
  "operation.get": { resource: "operation", id: "operation", feed: "operations" },
  "debug.terminals": { resource: "debug-terminals" },
  "prefs.get": { resource: "prefs", feed: "prefs" },
  "memory.status": { resource: "memory" },
  "connection.get": { resource: "connection" },
  "connection.addresses": { resource: "addresses" },
  "settings.get": { resource: "settings" },
  "for_you.list": { resource: "for-you", feed: "for_you" },
  "accounts.list": { resource: "accounts", feed: "accounts" },
  "stats.get": { resource: "stats" },
  "stats.outlook": { resource: "outlook" },
  "stats.cost": { resource: "cost" },
  "stats.retro": { resource: "retro" },
  "agents.list": { resource: "agents", feed: "agents" },
  "agent.get": { resource: "agent", id: "agent", feed: "agents" },
  "subagents.counts": { resource: "subagents" },
  "pr.exists": { resource: "pr-exists" },
  "agent.timeline": { resource: "timeline", feed: "agents" },
  "agent.options": { resource: "agent-options" },
  "prompt.list": { resource: "prompts", id: "agent", feed: "notifications" },
  "agent.strip": { resource: "strip", id: "agent", feed: "agents" },
  "agent.triggers": { resource: "triggers" },
  "access.list": { resource: "access", id: "agent", feed: "agents" },
  "chat.messages": { resource: "chat-messages", feed: "chat" },
  "chat.requests": { resource: "chat-requests" },
  "chat.transcript": { resource: "chat-transcript", id: "agent", feed: "chat" },
  "chat.locate": { resource: "chat-locate" },
  "chat.refs": { resource: "chat-refs", id: "agent" },
  "chat.search": { resource: "chat-search" },
  "inbox.list": { resource: "inbox", feed: "inbox" },
  "inbox.get": { resource: "inbox-item", id: "id", feed: "inbox" },
  "requests.list": { resource: "requests", feed: "requests" },
  "answers.list": { resource: "answers" },
  "devices.list": { resource: "devices", feed: "devices" },
  "repos.list": { resource: "repos" },
  "repos.recent": { resource: "recent_repos" },
  "repo.facts": { resource: "repo_facts" },
  "worktrees.list": { resource: "worktrees" },
  "worktree.diff": { resource: "worktree-diff" },
  "files.list": { resource: "files" },
  "file.read": { resource: "file" },
  "tasks.list": { resource: "tasks", feed: "tasks" },
  "task.get": { resource: "task", id: "task", feed: "tasks" },
  "task.history": { resource: "task-history", id: "task" },
  "task.time": { resource: "task-time", id: "task", feed: "tasks" },
  "requests.pending": { resource: "requests-pending", feed: "tasks" },
  "task.comment.list": { resource: "memos", id: "task" },
  "task.comment.recipients": { resource: "comment-recipients", feed: "tasks" },
  "task.view.list": { resource: "task-views" },
  "task.board": { resource: "board", feed: "tasks" },
  "trust.list": { resource: "trust" },
  "team.get": { resource: "team" },
  "proposal.list": { resource: "proposals", feed: "proposals" },
  "schedules.list": { resource: "schedules" },
} as const satisfies Record<string, { resource: string; id?: string; feed?: Topic }>;

/** The reads the app does not cache (x-app-cache false). */
export const NOT_CACHED = ["hello", "whoami", "subscribe", "unsubscribe", "lease.take", "lease.release", "lease.get", "events.feed", "status.pick", "session.launch_prepare", "permission.ask", "progress.get", "attachment.get", "inbox.queue", "request.read", "doc.open", "doc.pull", "doc.close", "task.shot", "task.memo.list", "terminal.attach", "terminal.history", "terminal.open", "terminal.detach"] as const;

/** The writes the app does not make from a screen (x-app-write false). */
export const NOT_WRITTEN = ["hook.record", "subagent.start", "session.wake", "say", "request.withdraw", "request.cancel", "device.redeem", "device.push_token", "task.handoff", "task.progress", "task.begin", "task.start", "task.memo.add", "task.memo.edit", "task.memo.delete", "task.wait", "proposal.propose", "trust.propose"] as const;

/** What each event's data must carry (the schema's required fields); the bridge reads the topic again for one that does not. */
export const EVENT_REQUIRED: Record<string, readonly string[]> = {
  "status.changed": [
    "alive",
    "version",
    "protocol",
    "schema",
    "started_at",
    "terminal_host",
    "agents",
    "inbox"
  ],
  "account.changed": [
    "name",
    "login_state",
    "five_hour",
    "weekly",
    "agents",
    "moves",
    "label",
    "vendor",
    "cap",
    "cap_effective",
    "recover",
    "cycle"
  ],
  "agent.changed": [
    "name",
    "role",
    "model",
    "effort",
    "state",
    "state_detail",
    "state_since",
    "account",
    "task",
    "cache_until",
    "has_dialog",
    "parent"
  ],
  "agent.removed": [
    "name"
  ],
  "agent.control": [
    "id",
    "at",
    "kind",
    "actor",
    "text"
  ],
  "access.changed": [
    "agent",
    "grants"
  ],
  "inbox.added": [
    "id",
    "kind",
    "agent",
    "title",
    "body",
    "evidence",
    "task",
    "created",
    "read",
    "prompt"
  ],
  "inbox.changed": [
    "id",
    "kind",
    "agent",
    "title",
    "body",
    "evidence",
    "task",
    "created",
    "read",
    "prompt"
  ],
  "task.changed": [
    "id",
    "from",
    "to",
    "title",
    "state",
    "time_box",
    "parent",
    "created",
    "delivered",
    "done",
    "hold_reason",
    "block_reason",
    "evidence"
  ],
  "attachment.removed": [
    "id"
  ],
  "task_done": [
    "task"
  ],
  "prompt_asked": [
    "id",
    "agent",
    "kind",
    "lines",
    "options",
    "since"
  ],
  "stall": [
    "agent"
  ],
  "account_out": [
    "account"
  ],
  "disk_low": [
    "free_bytes",
    "total_bytes"
  ],
  "chat.message": [
    "id",
    "at",
    "state_at",
    "from",
    "to",
    "state",
    "landing",
    "sending_reason",
    "body",
    "delivered_at",
    "typed_at",
    "landed_at",
    "parent",
    "kind",
    "tasks"
  ],
  "chat.transcript": [
    "id",
    "at",
    "kind"
  ],
  "chat.working": [
    "turn_running",
    "line",
    "since"
  ],
  "operation.progress": [
    "id",
    "kind",
    "state",
    "step",
    "steps_done",
    "steps_total",
    "message",
    "result",
    "agent"
  ],
  "device.paired": [
    "id",
    "name",
    "role",
    "created",
    "last_seen",
    "connected",
    "push"
  ],
  "device.revoked": [
    "device"
  ],
  "device.renamed": [
    "id",
    "name",
    "role",
    "created",
    "last_seen",
    "connected",
    "push"
  ],
  "proposal.raised": [
    "proposal"
  ],
  "proposal.settled": [
    "proposal"
  ],
  "prefs.changed": [
    "nav"
  ],
  "request.changed": [
    "kind",
    "task",
    "ref",
    "title",
    "status",
    "agent",
    "holders",
    "open",
    "done",
    "prs",
    "now",
    "blocked",
    "progress",
    "unread_comments",
    "created",
    "last_activity",
    "cursor"
  ],
  "for_you.opened": [
    "item",
    "count"
  ],
  "for_you.closed": [
    "kind",
    "id",
    "count"
  ]
};
