// Generated from contract/spec/ by contract/generate.py; do not edit.
/* eslint-disable */
import type { DocUpdate } from './repos.ts';
import type { Proposal } from './team.ts';

/** UTC, RFC 3339. */
export type Timestamp = string;

/** Who: an agent's or person's key, or a word the daemon resolves to one (human, an agent's name as hl team lists it). */
export type Who = string;

/** Ordered lowest to highest. A method's role is the lowest that may call it; every higher role may too. */
export type Role = "anonymous" | "agent" | "member" | "owner";

/** chat is added for the chat view (the UI design update): messages, landing state, transcript items and working state of one agent; events carry `agent`. devices (founder, which includes a paired device; subscribed on its own, live only): pairing and revocation of devices. proposals: proposals, so a client's tab dot is live; any proven caller, as proposal.list. transcript: the per-agent event stream of a structured session (transcript.event), served only for an agent the daemon runs as a structured session; see the Transcript stream section. prefs (member, which includes the founder): the person's view preferences, so a pin made on one device shows on the others. requests (member, which includes the founder): the founder's requests (RequestRow), so a Requests page is live. for_you (member, which includes the founder): the set of open items waiting on the founder, as for_you.list; an event whenever the count's set changes, with the new count. A client that misses events reads for_you.list again. */
export type Topic = "status" | "accounts" | "agents" | "people" | "workspaces" | "inbox" | "tasks" | "notifications" | "operations" | "chat" | "devices" | "proposals" | "transcript" | "prefs" | "requests" | "for_you";

export type Caller = {
  kind: "person" | "device" | "agent" | "anonymous";
  /** The key the caller's writes are stored under: the agent's (a<n>), or the person's (p<n>) for a person or their device; empty for anonymous. */
  key: string;
  /** The user, the device's name, or the agent's name. */
  name: string;
  /** Device id, when kind is device. */
  device?: string;
};

/** Who did a write; recorded on its event. */
export type Actor = {
  kind: "user" | "device" | "agent" | "daemon";
  name: string;
  device?: string;
};

/** Every write's result. A refusal is the `refused` error, never an accepted:false result. */
export type Accepted = {
  accepted: true;
  /** One sentence for a person. */
  said: string;
  /** True when this is the stored reply to a retried request_id; the action was not repeated. */
  replayed?: boolean;
  [key: string]: unknown;
};

export type RefusalData = {
  code: "refused";
  /** The gate's own sentence. */
  reason: string;
  /** Stable id of the gate that refused; see x-gates. */
  gate?: string;
  /** The command or step that fixes it, when there is one. */
  hint?: string;
  /** With gates proposal_changed and proposal_settled: the proposal as it is now. */
  proposal?: Proposal;
  /** With gate confirm: what the call would do and who is told. */
  plan?: string;
  /** With gate duplicate_title: the open task the new one repeats. */
  task?: number;
  /** With gate open_children: the task's open child tasks. */
  children?: {
    task: number;
    state: string;
    assignee: string;
    title: string;
  }[];
  /** With gate access_needed: the agent the message is for. */
  agent?: string;
  /** With gate access_needed: each ref the agent may not read. */
  paths?: string[];
  /** With gate behind: the doc's current version. */
  version?: number;
};

/** Like 2h, 30m, 1d. */
export type Duration = string;

export type InputHolder = {
  kind: "daemon" | "person" | "device";
  /** `daemon`, or the person or device. */
  name: string;
  since: Timestamp | null;
};

export type PromptOption = {
  /** What prompt.answer takes as `choice`. */
  key: string;
  label: string;
  answer?: "yes" | "no" | "other";
  highlighted?: boolean;
};

export type Prompt = {
  /** Opaque; changes whenever the open prompt changes (the daemon uses the screen block's fingerprint). */
  id: string;
  agent: Who;
  /** permission, question, dialog or exit_confirm today; open set. */
  kind: string;
  tool?: string | null;
  command?: string | null;
  reason?: string | null;
  /** The dialog's panel as it is on screen, one string per line. */
  lines: string[];
  /** The choices, parsed from the dialog. Empty until the session-control task fills it; a client falls back to lines. */
  options: PromptOption[];
  /** When the daemon first saw this prompt open. */
  since: Timestamp;
  /** Several options may be picked, then submitted: prompt.answer takes `choice` as an array of PromptOption.key. Absent means false, as for every other prompt, where an array is bad_request. */
  multi?: boolean;
  /** hook: asked through permission.ask, answered allow or deny with no terminal typed into. Absent for a prompt read from the agent's screen. */
  source?: "hook";
};

/** What an agent or task is doing and why (x-status). */
export type Status = {
  /** One of x-status states. */
  state: string;
  /** One of x-status reasons. */
  reason: string;
  /** The line every view shows for it, as it is. */
  text: string;
  /** When it ends by itself (a clock, a wall's reset); null when nothing says. */
  until: Timestamp | null;
  /** Since when it has been so; null when not known. */
  since: Timestamp | null;
  /** The tasks it waits on, for reason task; for reason paused on an agent, the queued tasks the pause keeps from it, oldest first; empty otherwise. */
  tasks: number[];
};

export type Event = {
  /** Opaque, increasing integer: the daemon's own stream position, not the id of any other table's row. A client keeps the newest one it saw and resumes from it. */
  cursor: number;
  topic: Topic;
  /** See x-topics for each topic's types. */
  type: string;
  at: Timestamp;
  actor: Actor | null;
  agent: Who | null;
  title: string | null;
  target: {
    screen: string;
    /** Agent key, task or item id. */
    id: string;
  } | null;
  data: { [key: string]: unknown };
};

export type MissingTool = {
  /** The tool. */
  tool: "claude" | "git" | "tmux";
  /** A shell command that installs the tool, for a client to show and copy, for example "brew install tmux". */
  fix: string;
};

export type DaemonStatus = {
  alive: boolean;
  /** The loaded build's identity: its git sha, or <version>+<sha>[-dirty] for a release build. */
  version: string;
  /** The absolute path of the executable this process runs. */
  exe?: string;
  protocol: number;
  schema: number;
  started_at: Timestamp;
  terminal_host: {
    name: string;
    health: "ok" | "degraded" | "down";
    detail?: string;
  };
  agents: {
    total: number;
    by_state: { [key: string]: number };
  };
  inbox: {
    unread: number;
  };
  /** Null until the first measure, and on a daemon that does not measure. */
  resources?: Resources | null;
  /** The machine's own needs-you items, oldest first; absent when there are none, and on a daemon that does not serve it. A client draws each beside the agents' needs-you cards, in the text given. */
  needs_you?: MachineNeed[];
  /** The tools agents are run with (claude, git, tmux) that this machine lacks, read at each call; absent when none is missing, when the daemon runs no agent side, and on a daemon that does not serve it. No agent can start while one is missing: a client shows each with its fix. */
  missing_tools?: MissingTool[];
  /** Null when new work is not paused; absent on a daemon that does not serve it. */
  paused?: DaemonPause | null;
  /** Absent on a daemon that does not serve it. */
  health?: DaemonHealth;
  /** Absent on a daemon that does not serve it. */
  keep_awake?: DaemonKeepAwake;
  /** The machine this daemon runs on. Absent on a daemon that does not serve it. */
  machine?: Machine;
};

/** A pause of new work on one machine (daemon.pause). */
export type DaemonPause = {
  since: Timestamp;
  /** When it goes on by itself; null when only daemon.resume ends it. */
  until: Timestamp | null;
  /** The machine that is paused: this daemon's (DaemonStatus.machine). Absent on a daemon that does not serve it. */
  machine?: Machine;
};

/** Keeping the machine from idle sleep: what is set and whether it holds now. status.changed is told within 10 s of a change. */
export type DaemonKeepAwake = {
  /** config.yaml keep_awake (settings.get): off; on, held while the daemon runs; working, held while an agent is mid-turn. */
  mode: "off" | "on" | "working";
  /** Whether the daemon holds macOS caffeinate -i now. False under on when caffeinate could not start (not macOS). */
  held: boolean;
  /** When the hold now in force began; absent while not held. */
  since?: Timestamp;
};

/** The machine's state in one line. held, the first that applies: the founder's pause (daemon.pause); the load holding starts (Resources.start_hold, by its own rules); any needs_you item; any agent whose status reason is cap, wall or pace, with the count in text. near, when nothing holds: the load per core at least its limit less 10 percentage points (load_per_core_pct), or free memory less than its limit plus 10 (ram_free_min_pct), when that limit is set; never before the first measure. host_active_cap is never near: at it, starts are held. status.changed is told within 10 s of a change of level or text. */
export type DaemonHealth = {
  /** held: something holds new work back now. near: nothing does, but a machine limit that is set is close. ok: neither. A client with no connection shows down itself; the daemon never says it. */
  level: "ok" | "near" | "held";
  /** One line for a person naming what set the level, for held and near; empty for ok. */
  text: string;
  /** When this level began; started_at when it has not changed since the start. */
  since: Timestamp;
};

export type Repo = {
  name: string;
  path: string;
  default_branch: string;
};

export type Operation = {
  id: string;
  kind: string;
  state: "running" | "done" | "failed";
  step: string | null;
  steps_done: number;
  steps_total: number | null;
  message: string | null;
  result: { [key: string]: unknown } | null;
  agent: Who | null;
};

export type Settings = {
  timings: { [key: string]: number };
  terminal_host: {
    choice: string;
    available: string[];
    health: "ok" | "degraded" | "down";
  };
  listener: Listener;
  trust_folders: {
    account: string;
    agent: Who | null;
    path: string;
  }[];
  hooks: {
    installed: boolean;
    accounts: {
      account: string;
      installed: boolean;
    }[];
  };
  values: SettingsValues;
  /** With settings.get `agent`: only that agent's own values for the Per agent keys. */
  overrides?: { [key: string]: unknown };
  /** Without settings.get `agent`: the built-in handoff prompt, what `values.handoff_prompt` shows while none is set, so a client can tell the two apart. */
  handoff_prompt_default?: string;
  /** Without settings.get `agent`: the agents whose row sets a handoff prompt of their own, which the global one does not reach. */
  handoff_prompt_own?: Who[];
};

/** One terminal row: plain text, no escapes; in format `styled` also its style runs. */
export type Row = {
  text: string;
  /** Set only in format `styled`, never in `text` (those rows have none). Sorted by `from`, not overlapping, none empty, only runs whose style is not the default; what no span covers is drawn in the default style. `text` is the same string `text` would send, so a client that ignores spans loses nothing. */
  spans?: {
    /** First character, counted in Unicode code points of `text`, 0-based. */
    from: number;
    /** One past the last, so the run is [from, to). May go past the end of `text` for blank cells with a background; see x-terminal-styled `spans` for how those count. */
    to: number;
    style: SpanStyle;
  }[];
};

/** Text and styled attaches; null when hidden or above the frame. `x` is in code points of the row's `text`, like `Row.spans`; `y` is in the frame's rows and may be len(lines) or more. Rules: `x-terminal-styled`, Cursor. */
export type Cursor = {
  x: number;
  y: number;
} | null;

export type Capabilities = {
  chat: boolean;
  /** Worktree review beyond worktree.diff; false in v1, the UI hides the Review tab. */
  review: boolean;
  /** Formats the daemon serves on at least one host. */
  terminal_format: ("text" | "bytes" | "styled")[];
  /** Each terminal host the daemon has and the formats it serves there, e.g. {tmux: [text, bytes]}. Absent: text on the one host it has. */
  terminal_hosts?: { [key: string]: string[] };
  /** chat.send takes parent_item, a reply to one chat.transcript item. Absent means false. */
  transcript?: boolean;
  /** The founder's own messages to a terminal agent are typed into its terminal (unless its row turns peer_inbox off), so hl launch tells the agent so. Absent means false. */
  founder_typed?: boolean;
};

export type Listener = {
  enabled: boolean;
  /** loopback, lan, tailscale or an address. */
  bind: string;
  /** Always an encrypted WebSocket with a certificate; `none` only on loopback. A Tailscale address is just a bind address. */
  encryption: "tls" | "none";
  fingerprint: string | null;
};

/** Chosen by the caller; unique per intended action. */
export type RequestId = string;

/** Every global setting by its config.yaml name, with its default; every one is the founder's to set. Per-account keys (label, cap, usage recovery, cycle) are account.set; per-agent keys are agent.set and schedule.*; a key marked Per agent also takes `agent` in settings.set. */
export type SettingsValues = {
  /** Default cap: most agents mid-turn at once on an account with no cap of its own. */
  active_cap?: number;
  /** Idle close, seconds; 0 never. The UI shows whole minutes. Per agent with `agent`. */
  park_timeout_s?: number;
  /** Agents write a note for the next session before a close or move. */
  handoff_note?: boolean;
  /** Resume the old session even after its cache went cold. Per agent with `agent`. */
  ignore_cache_expiry?: boolean;
  /** The daemon restarts itself when its own code changed (hl dev-mode). */
  dev_mode?: boolean;
  /** Seconds of nothing shown before an idle agent with background work is told to finish or stop it; default follows park_timeout_s. Per agent with `agent`. */
  bg_notice_s?: number;
  /** Seconds after that notice before the work is stopped. Per agent. */
  bg_grace_s?: number;
  /** How often an agent with background work is asked which to stop. Per agent. */
  bg_check_s?: number;
  /** Seconds after the first waiting notice before the agent is typed one digest, at its prompt. Per agent. */
  notice_digest_s?: number;
  /** Input price per million tokens by model-name pattern, laid over the built-in table; an entry that is not a positive number is ignored. */
  token_prices?: { [key: string]: number };
  /** NEW, no code today: seconds a person or device may hold a terminal's input without typing before the daemon takes it back (see terminal `holder` notice). */
  input_hold_idle_s?: number;
  /** Tell a queued task's holder when other open tasks have waited on it (blocked by it, or its parent) for blocker_alarm_s, and the holder's lead at blocker_alarm_lead_s; once each per wait, in the digest, never the founder. */
  blocker_alarm?: boolean;
  /** Seconds the first open task waiting on a queued task may wait before its holder is told (blocker_alarm). */
  blocker_alarm_s?: number;
  /** Seconds the first open task waiting on a queued task may wait before its holder's lead is told (blocker_alarm). */
  blocker_alarm_lead_s?: number;
  /** How many times a task of a request may be sent back (reopened) before the request shows attention bounced. */
  attention_sent_back?: number;
  /** Seconds a request's open tree may go with no activity before it shows attention quiet. */
  attention_quiet_s?: number;
  /** Seconds an open task of a request may wait on cap, wall or owner before the request shows attention waiting (RequestRow.attention). */
  attention_wait_s?: number;
  /** The prompt typed at an agent to write its handoff note, one line; {path} is the note's file and {max} its size limit (3000). Without {path} the path is added on a line of its own. Line breaks are joined into one space; under 20 characters after trimming is bad_request (blank means the built-in). settings.get shows the built-in prompt when none is set. Per agent. */
  handoff_prompt?: string;
  /** Whether a notifications `prompt_asked` event (an agent asks (a permission or a question)) raises a notice on the founder's devices. The event is on the topic and in events.feed either way; the shell that raises notices reads this. */
  push_prompt_asked?: boolean;
  /** Whether a notifications `task_done` event (a task is done) raises a notice on the founder's devices. The event is on the topic and in events.feed either way; the shell that raises notices reads this. */
  push_task_done?: boolean;
  /** Whether a notifications `stall` event (an agent stalls) raises a notice on the founder's devices. The event is on the topic and in events.feed either way; the shell that raises notices reads this. */
  push_stall?: boolean;
  /** Whether a notifications `account_out` event (an account is out) raises a notice on the founder's devices. The event is on the topic and in events.feed either way; the shell that raises notices reads this. */
  push_account_out?: boolean;
  /** Most agents mid-turn at once on this machine, over every account; 0 is no limit. Over it the daemon starts no new work (status reason load). */
  host_active_cap?: number;
  /** Most 1-minute load per core, in percent (150 is 1.5 per core); 0 is off, the default (the limits wait for a limit based on what an agent's processes use). Over it the daemon starts no new work. */
  load_per_core_pct?: number;
  /** Least free memory, in percent of all; 0 is off, the default. Under it the daemon starts no new work. */
  ram_free_min_pct?: number;
  /** Least free disk on the volume the daemon home is on, in GB; 0 is off, the default. The disk needs-you item uses it as its floor when it is set, else 10 GB. */
  disk_free_min_gb?: number;
  /** Turn on every hold the pace prediction makes on a start (usage Predict): an account ahead of its weekly pace line (Account.pace) starts fewer turns, and one ahead by more than pace_burst_pct starts none (status reason pace); the cap shrinks where the agents mid-turn would reach the week's block before it ends; and a start waits when one more agent would run the 5-hour window or the week out before its reset. The prediction prices each agent mid-turn at its model's dollar pace over the account's factors (Account.factors). A turn a person starts is never held, and a running turn is never stopped. Off by default: the prediction is still read and shown, and holds nothing; only the windows' walls hold work. */
  launch_pace_hold?: boolean;
  /** Where each account's pace line ends: its weekly usage, in percent, at the end of its week (its weekly reset, or its subscription end when that comes first). Under 100 because a reading lags by minutes. */
  pace_target_pct?: number;
  /** How far, in points of the week, an account may run ahead of its pace line. Inside it the account's cap shrinks in step; past it the account starts no new turn. */
  pace_burst_pct?: number;
  /** Refuse a subagent (the Agent tool) when one more would take the agent's account to its 5-hour or weekly limit before that window resets (subagent.start). Off: every start is recorded and none is refused. Off by default until the founder turns it on. */
  subagent_pace_guard?: boolean;
  /** When an idle agent's account reaches this percent of its 5-hour or weekly window and work is waiting for it, the agent moves to an account still under it that has room, before the work starts a turn; 0 is off, the default, and an agent moves only at the wall. */
  usage_move_before_pct?: number;
  /** Let a context clear go ahead when the agent's turn has ended and only background work keeps its session busy: it is told, and the work is cut off after 5 minutes as for any clear. Off: the clear waits for that work with no limit. */
  context_clear_past_background?: boolean;
  /** Store each running turn's replies, tool calls and tool output (cut to 16 KiB) from the session transcript, so chat.transcript serves them as reply and tool items; only what is written while a turn runs is stored. One database write per entry. Off: nothing is stored. */
  chat_transcript_rows?: boolean;
  /** Refuse chat.send when a ref (a path sent by reference) is outside what the agent may read, gate access_needed, so the founder grants it first (x-access). Off: refs are delivered as they are and the session asks for a read it may not make. */
  chat_refs_need_grant?: boolean;
  /** Keep the machine from idle sleep: off; on (held while the daemon runs); working (held while an agent is mid-turn, released when the last one stops). The daemon holds macOS caffeinate -i; closing the lid still sleeps. Applied at the next tick, without a restart. */
  keep_awake?: "off" | "on" | "working";
  /** What the daemon does with work its agents left running after its parent died (headless Chrome, vitest, vite, a go test binary, yes, Electron) that has run 30 minutes under a worktree: dry (the default) only logs and records an orphan_would_stop event every 10 minutes; on stops it and records orphan_stopped; off does nothing. Nothing outside a worktree is touched. */
  reap_orphans?: "dry" | "on" | "off";
  /** Most test or build runs (go test, cargo, xcodebuild, vitest and the like) held at once through hl use, over every agent; 0 is no limit, the default. Over it an agent's hl use heavy waits its turn (lease.take), and the PreToolUse guard refuses a test or build command run outside hl use unless the agent holds one; with 0 nothing waits or is refused. A script that runs tests inside (tools/check.sh) is not read. */
  heavy_max?: number;
  /** Most simulators (iOS Simulator, Android emulator) held at once through hl use, over every agent; 0 is no limit, the default. Over it an agent's hl use simulator waits its turn (lease.take), and the PreToolUse guard refuses a simulator started outside hl use unless the agent holds one; with 0 nothing waits or is refused. */
  simulator_max?: number;
  /** Most browsers (headless Chrome or Chromium, Playwright) held at once through hl use, over every agent; 0 is no limit, the default. Over it an agent's hl use browser waits its turn (lease.take), and the PreToolUse guard refuses a browser started outside hl use unless the agent holds one; with 0 nothing waits or is refused. */
  browser_max?: number;
  /** The ports a port lease (hl use port) is picked from, as lo-hi (e.g. 4000-4999) with 1024 <= lo <= hi <= 65535: a port no lease holds and that binds at grant time, given to the command as $HL_PORT; over the range an ask waits. Unset, the default, a port lease is any free port the system gives and none waits. */
  port_range?: string | null;
  /** Hosts the network listener serves, from connection.addresses, each on the -listen port: a door on each of their addresses, made again within seconds when the setting or the interfaces change (no restart). The web page is served on each. null (the default) serves -listen alone; [] serves none. A host the machine has no address for now is served when it has one again. A write answers unreachable_devices: the paired devices whose host is no longer served. */
  network_addresses?: string[] | null;
};

/** settings.set's changes: any of SettingsValues' keys; null removes one. */
export type SettingsChanges = {
  /** Default cap: most agents mid-turn at once on an account with no cap of its own. null removes the key (back to the default). */
  active_cap?: number | null;
  /** Idle close, seconds; 0 never. The UI shows whole minutes. Per agent with `agent`. null removes the key (back to the default). */
  park_timeout_s?: number | null;
  /** Agents write a note for the next session before a close or move. null removes the key (back to the default). */
  handoff_note?: boolean | null;
  /** Resume the old session even after its cache went cold. Per agent with `agent`. null removes the key (back to the default). */
  ignore_cache_expiry?: boolean | null;
  /** The daemon restarts itself when its own code changed (hl dev-mode). null removes the key (back to the default). */
  dev_mode?: boolean | null;
  /** Seconds of nothing shown before an idle agent with background work is told to finish or stop it; default follows park_timeout_s. Per agent with `agent`. null removes the key (back to the default). */
  bg_notice_s?: number | null;
  /** Seconds after that notice before the work is stopped. Per agent. null removes the key (back to the default). */
  bg_grace_s?: number | null;
  /** How often an agent with background work is asked which to stop. Per agent. null removes the key (back to the default). */
  bg_check_s?: number | null;
  /** Seconds after the first waiting notice before the agent is typed one digest, at its prompt. Per agent. null removes the key (back to the default). */
  notice_digest_s?: number | null;
  /** Input price per million tokens by model-name pattern, laid over the built-in table; an entry that is not a positive number is ignored. null removes the key (back to the default). */
  token_prices?: { [key: string]: number } | null;
  /** NEW, no code today: seconds a person or device may hold a terminal's input without typing before the daemon takes it back (see terminal `holder` notice). null removes the key (back to the default). */
  input_hold_idle_s?: number | null;
  /** Tell a queued task's holder when other open tasks have waited on it (blocked by it, or its parent) for blocker_alarm_s, and the holder's lead at blocker_alarm_lead_s; once each per wait, in the digest, never the founder. */
  blocker_alarm?: boolean;
  /** Seconds the first open task waiting on a queued task may wait before its holder is told (blocker_alarm). null removes the key (back to the default). */
  blocker_alarm_s?: number | null;
  /** Seconds the first open task waiting on a queued task may wait before its holder's lead is told (blocker_alarm). null removes the key (back to the default). */
  blocker_alarm_lead_s?: number | null;
  /** How many times a task of a request may be sent back (reopened) before the request shows attention bounced. null removes the key (back to the default). */
  attention_sent_back?: number | null;
  /** Seconds a request's open tree may go with no activity before it shows attention quiet. null removes the key (back to the default). */
  attention_quiet_s?: number | null;
  /** Seconds an open task of a request may wait on cap, wall or owner before the request shows attention waiting (RequestRow.attention). null removes the key (back to the default). */
  attention_wait_s?: number | null;
  /** The prompt typed at an agent to write its handoff note, one line; {path} is the note's file and {max} its size limit (3000). Without {path} the path is added on a line of its own. settings.get shows the built-in prompt when none is set. Per agent. An empty string is the built-in prompt; null removes the key (back to the default). */
  handoff_prompt?: string | null;
  /** Whether a notifications `prompt_asked` event (an agent asks (a permission or a question)) raises a notice on the founder's devices. The event is on the topic and in events.feed either way; the shell that raises notices reads this. null removes the key (back to the default). */
  push_prompt_asked?: boolean | null;
  /** Whether a notifications `task_done` event (a task is done) raises a notice on the founder's devices. The event is on the topic and in events.feed either way; the shell that raises notices reads this. null removes the key (back to the default). */
  push_task_done?: boolean | null;
  /** Whether a notifications `stall` event (an agent stalls) raises a notice on the founder's devices. The event is on the topic and in events.feed either way; the shell that raises notices reads this. null removes the key (back to the default). */
  push_stall?: boolean | null;
  /** Whether a notifications `account_out` event (an account is out) raises a notice on the founder's devices. The event is on the topic and in events.feed either way; the shell that raises notices reads this. null removes the key (back to the default). */
  push_account_out?: boolean | null;
  /** Most agents mid-turn at once on this machine, over every account; 0 is no limit. Over it the daemon starts no new work (status reason load). null removes the key (back to the default). */
  host_active_cap?: number | null;
  /** Most 1-minute load per core, in percent (150 is 1.5 per core); 0 is off, the default (the limits wait for a limit based on what an agent's processes use). Over it the daemon starts no new work. null removes the key (back to the default). */
  load_per_core_pct?: number | null;
  /** Least free memory, in percent of all; 0 is off, the default. Under it the daemon starts no new work. null removes the key (back to the default). */
  ram_free_min_pct?: number | null;
  /** Least free disk on the volume the daemon home is on, in GB; 0 is off, the default. The disk needs-you item uses it as its floor when it is set, else 10 GB. null removes the key (back to the default). */
  disk_free_min_gb?: number | null;
  /** Turn on every hold the pace prediction makes on a start (usage Predict): an account ahead of its weekly pace line (Account.pace) starts fewer turns, and one ahead by more than pace_burst_pct starts none (status reason pace); the cap shrinks where the agents mid-turn would reach the week's block before it ends; and a start waits when one more agent would run the 5-hour window or the week out before its reset. The prediction prices each agent mid-turn at its model's dollar pace over the account's factors (Account.factors). A turn a person starts is never held, and a running turn is never stopped. Off by default: the prediction is still read and shown, and holds nothing; only the windows' walls hold work. */
  launch_pace_hold?: boolean | null;
  /** Where each account's pace line ends: its weekly usage, in percent, at the end of its week (its weekly reset, or its subscription end when that comes first). Under 100 because a reading lags by minutes. null removes the key (back to the default). */
  pace_target_pct?: number | null;
  /** How far, in points of the week, an account may run ahead of its pace line. Inside it the account's cap shrinks in step; past it the account starts no new turn. null removes the key (back to the default). */
  pace_burst_pct?: number | null;
  /** Refuse a subagent (the Agent tool) when one more would take the agent's account to its 5-hour or weekly limit before that window resets (subagent.start). Off: every start is recorded and none is refused. Off by default until the founder turns it on. */
  subagent_pace_guard?: boolean;
  /** When an idle agent's account reaches this percent of its 5-hour or weekly window and work is waiting for it, the agent moves to an account still under it that has room, before the work starts a turn; 0 is off, the default, and an agent moves only at the wall. null removes the key (back to the default). */
  usage_move_before_pct?: number | null;
  /** Let a context clear go ahead when the agent's turn has ended and only background work keeps its session busy: it is told, and the work is cut off after 5 minutes as for any clear. Off: the clear waits for that work with no limit. */
  context_clear_past_background?: boolean;
  /** Store each running turn's replies, tool calls and tool output (cut to 16 KiB) from the session transcript, so chat.transcript serves them as reply and tool items; only what is written while a turn runs is stored. One database write per entry. Off: nothing is stored. */
  chat_transcript_rows?: boolean;
  /** Refuse chat.send when a ref (a path sent by reference) is outside what the agent may read, gate access_needed, so the founder grants it first (x-access). Off: refs are delivered as they are and the session asks for a read it may not make. */
  chat_refs_need_grant?: boolean;
  /** Keep the machine from idle sleep: off; on (held while the daemon runs); working (held while an agent is mid-turn, released when the last one stops). The daemon holds macOS caffeinate -i; closing the lid still sleeps. Applied at the next tick, without a restart. null removes the key (back to the default). */
  keep_awake?: ("off" | "on" | "working") | null;
  /** What the daemon does with work its agents left running after its parent died (headless Chrome, vitest, vite, a go test binary, yes, Electron) that has run 30 minutes under a worktree: dry (the default) only logs and records an orphan_would_stop event every 10 minutes; on stops it and records orphan_stopped; off does nothing. Nothing outside a worktree is touched. */
  reap_orphans?: ("dry" | "on" | "off") | null;
  /** Most test or build runs (go test, cargo, xcodebuild, vitest and the like) held at once through hl use, over every agent; 0 is no limit, the default. Over it an agent's hl use heavy waits its turn (lease.take), and the PreToolUse guard refuses a test or build command run outside hl use unless the agent holds one; with 0 nothing waits or is refused. A script that runs tests inside (tools/check.sh) is not read. null removes the key (back to the default). */
  heavy_max?: number | null;
  /** Most simulators (iOS Simulator, Android emulator) held at once through hl use, over every agent; 0 is no limit, the default. Over it an agent's hl use simulator waits its turn (lease.take), and the PreToolUse guard refuses a simulator started outside hl use unless the agent holds one; with 0 nothing waits or is refused. null removes the key (back to the default). */
  simulator_max?: number | null;
  /** Most browsers (headless Chrome or Chromium, Playwright) held at once through hl use, over every agent; 0 is no limit, the default. Over it an agent's hl use browser waits its turn (lease.take), and the PreToolUse guard refuses a browser started outside hl use unless the agent holds one; with 0 nothing waits or is refused. null removes the key (back to the default). */
  browser_max?: number | null;
  /** The ports a port lease (hl use port) is picked from, as lo-hi (e.g. 4000-4999) with 1024 <= lo <= hi <= 65535: a port no lease holds and that binds at grant time, given to the command as $HL_PORT; over the range an ask waits. Unset, the default, a port lease is any free port the system gives and none waits. Another value is bad_request (field port_range). null removes the key (back to the default). */
  port_range?: string | null;
  /** Hosts the network listener serves, from connection.addresses, each on the -listen port: a door on each of their addresses, made again within seconds when the setting or the interfaces change (no restart). The web page is served on each. null (the default) serves -listen alone; [] serves none. A host the machine has no address for now is served when it has one again. A write answers unreachable_devices: the paired devices whose host is no longer served. A host this machine cannot be reached on is refused, gate address. A client connected through a host it drops is cut off before the answer; its retry with the same request_id reads it. null removes the key (back to the default). */
  network_addresses?: string[] | null;
};

/** One method as a daemon lists it: its role and whether this caller may call it. */
export type MethodInfo = {
  name: string;
  since: number;
  role: Role;
  allowed: boolean;
};

/** An xterm palette index (0-15 are the client's theme ANSI colours, 16-255 the fixed cube and greys) or a true colour as lower-case `#rrggbb`. A colour the cell does not have (the default) is left out, never sent as a number. */
export type CellColor = number | string;

/** Only what differs from the default cell: a flag is sent as true or left out, a colour is left out for the default. `inverse` is sent as the flag, not applied: the client swaps the foreground and background it draws, so default colours swap too. A client ignores a key it does not know. */
export type SpanStyle = {
  fg?: CellColor;
  bg?: CellColor;
  bold?: boolean;
  dim?: boolean;
  italic?: boolean;
  underline?: boolean;
  inverse?: boolean;
  strike?: boolean;
};

/** Chosen by the daemon when an upload completes. */
export type AttachmentId = string;

/** One pane of the terminal host, with what the daemon knows of it. A field the daemon does not know is null. */
export type DebugTerminalPane = {
  session: string;
  /** The window's index in its session. */
  window: string;
  window_name: string;
  /** The pane's handle, as terminal.attach takes it. */
  terminal: string;
  /** The pane's own process (its shell). */
  pid: number | null;
  /** What the host says runs in the pane now. */
  command: string;
  path: string | null;
  dead: boolean;
  last_activity: Timestamp | null;
  /** Seconds since the pane's window last drew output. */
  idle_s: number | null;
  /** agent: an agent CLI (claude, codex, opencode) runs at or under the pane's process; shell: none does and the pane shows a shell; unknown: the process table could not be read and the pane's command says neither. */
  process: "agent" | "shell" | "other" | "unknown";
  agent_process: {
    pid: number;
    name: string;
  } | null;
  /** The agent whose current terminal this pane is. */
  agent: Who | null;
  /** No agent has this pane as its terminal. */
  orphan: boolean;
  /** The agent is closed; it keeps the handle of the pane it had. */
  closed: boolean;
  /** For an orphan: the agent the daemon opened the pane for. */
  recorded_agent: Who | null;
  state: string | null;
  account: string | null;
  session_id: string | null;
  worktree: string | null;
  /** When the agent's prompt cache lapses, from its view. */
  cache_until: Timestamp | null;
  /** Always null: the daemon keeps no link from a tmux pane to an Orca terminal. */
  orca_terminal: null;
};

/** Words of a task's text between its image references. */
export type TextPart = {
  text: string;
};

/** An image a task's text names, where it names it. */
export type ImagePart = {
  /** The Attachment; only `id` when the image is gone. */
  image: {
    id: AttachmentId;
    name?: string;
    /** As Attachment.mime. */
    mime?: string;
    bytes?: number;
  };
  /** The words in the reference's brackets; what a client shows when it cannot draw the image. */
  caption: string;
  /** true when the image is no longer kept: the client shows the caption without a picture. */
  gone?: boolean;
  /** Where the image is stored. Only for an agent caller, which reads it by path; a member or founder client never gets it. */
  path?: string;
  /** true when the founder deleted the image (attachment.remove), always with gone: the client says it was deleted. */
  removed?: boolean;
};

/** The request's ref: 12 lowercase hex characters, unique and not guessable from another request's. The integer id stays the request's key everywhere else. */
export type RequestRef = string;

/** How this process is reached, read only. */
export type ConnectionInfo = {
  /** The door a pairing names when device.pair gets no address; null when there is none. */
  listener: null | ConnectionListener;
  /** Every host a door is open for now: the network_addresses served (a chosen host the machine has no address for is left out), or the -listen door while it is unset. */
  listeners: ConnectionListener[];
  /** SettingsValues.dev_mode. */
  dev_mode: boolean;
};

export type ConnectionListener = {
  /** The host:port a pairing names: this door's host, or -advertise for the default door while network_addresses is unset. */
  address: string;
  /** none on a Tailscale (or loopback) door: plain ws, the tailnet encrypts. tls on a LAN door: the certificate given with -cert, else one the daemon made. */
  encryption: "tls" | "none";
  /** sha256 of the certificate presented; null on a plain door. */
  fingerprint: string | null;
  /** The address's host when it is a *.ts.net name, else null. */
  tailscale_host: string | null;
  /** The web page is served on this door, so device.pair gives a web_link for it. */
  web?: boolean;
};

/** One host this machine can be reached on from another device. */
export type NetworkAddress = {
  /** An IPv4 literal or a name: what network_addresses and device.pair's address take. */
  host: string;
  /** lan: a private IPv4 (10/8, 172.16/12, 192.168/16) of a LAN or Wi-Fi interface. tailscale: the Tailscale interface's 100.64/10 address. tailscale_name: the MagicDNS name (*.ts.net), served on that address. mdns: name.local, served on every LAN address. */
  kind: "lan" | "tailscale" | "tailscale_name" | "mdns";
  /** The interface it is on; empty for mdns, which spans them. */
  interface: string;
  /** A door is open for it now. */
  listening: boolean;
  /** How it is served when chosen: tls for lan and mdns, none for the Tailscale kinds (unless the daemon was given -cert). */
  encryption: "tls" | "none";
};

/** The hosts this machine can be reached on, read now: loopback and public addresses are never listed. */
export type ConnectionAddresses = {
  addresses: NetworkAddress[];
  /** SettingsValues.network_addresses. */
  network_addresses: string[] | null;
};

/** A request named by its integer id, or by its ref: msg_ and the 12 hex characters, or the hash alone, or a prefix of at least 6 of them that begins only one request the caller may read (an ambiguous one is bad_request). A request the caller may not read is not_found, the same answer as one that does not exist. */
export type RequestName = number | string;

/** What the machine and each agent use, measured every 15 s: one ps read for every agent. A figure the daemon could not read is 0. */
export type Resources = {
  /** When it was measured. */
  at: Timestamp;
  /** Per agent with a terminal pane: its pane process and every process under it. */
  agents: { [key: string]: {
    /** CPU in percent of one core, summed over the tree (ps %cpu): over 100 on several cores. */
    cpu_pct: number;
    /** Resident memory summed over the tree. */
    rss_bytes: number;
  } };
  machine: {
    ram_total_bytes: number;
    ram_used_bytes: number;
    /** Free for use: macOS free, inactive and speculative pages; Linux MemAvailable. */
    ram_free_bytes: number;
    /** The 1-minute load average; compare with cores. */
    load_1m: number;
    cores: number;
    /** Free space of the volume the daemon home is on. */
    disk_free_bytes: number;
    disk_total_bytes: number;
    /** load_1m divided by cores; the load limit is compared with this (150 percent is 1.5). */
    load_per_core: number;
    /** ram_free_bytes in percent of ram_total_bytes; the free-memory floor is compared with this. */
    ram_free_pct: number;
  };
  /** What all agents with a terminal pane use together. */
  agents_total: {
    /** CPU in percent of one core, summed over every agent's tree. */
    cpu_pct: number;
    /** Resident memory summed over every agent's tree. */
    rss_bytes: number;
  };
  /** Why the daemon starts no new work now (the machine is over a load limit: text names the limit, as the status reason load does), null when starts may go. Level-triggered on the same measure; a machine with no agent mid-turn never holds. */
  start_hold: string | null;
};

/** How the person's agent list and dock show agents, the same on every device. */
export type NavPrefs = {
  /** Pinned agents in the person's order: the dock shows them first and the list on top. No name twice. */
  pins: Who[];
  /** The list shows only agents with something unread or waiting on the person. */
  unread_only: boolean;
  /** State groups the list hides: needs (waiting, blocked), working, waking, idle, sleeping (parked, none). */
  hidden: ("needs" | "working" | "waking" | "idle" | "sleeping")[];
  /** tree: team, lead, members by parent. flat: urgency first, then by name. */
  layout: "tree" | "flat";
  /** The person's order for the tree layout: agents under one parent, and the top-level agents, stand in this order; one not listed (a new agent) comes after the listed ones, by name. The flat layout ignores it. No name twice. */
  order: Who[];
  /** For you items the founder put off, by `kind:id` (ForYouItem.kind and id, such as `task:42`). prefs.set merges this key entry by entry: an entry sets or replaces one item, null clears it; the other entries stay, so two devices deferring different items lose nothing. An entry whose time passed or whose task ended is dead and the daemon drops it at the next prefs.set that changes later. Absent entries mean not deferred. */
  later?: { [key: string]: Later | null };
};

/** Any keys of NavPrefs; each given key replaces the stored value whole, except `later`, which merges entry by entry. */
export type NavPrefsChanges = {
  /** Pinned agents in the person's order: the dock shows them first and the list on top. No name twice. */
  pins?: Who[];
  /** The list shows only agents with something unread or waiting on the person. */
  unread_only?: boolean;
  /** State groups the list hides: needs (waiting, blocked), working, waking, idle, sleeping (parked, none). */
  hidden?: ("needs" | "working" | "waking" | "idle" | "sleeping")[];
  /** tree: team, lead, members by parent. flat: urgency first, then by name. */
  layout?: "tree" | "flat";
  /** The person's order for the tree layout: agents under one parent, and the top-level agents, stand in this order; one not listed (a new agent) comes after the listed ones, by name. The flat layout ignores it. No name twice. */
  order?: Who[];
  /** Entries to set or clear in NavPrefs.later: an entry sets or replaces one item's deferral, null clears it; the other items stay. A key is `kind:id` of a For you item (kind task, comment or machine: a prompt or a proposal cannot be put off); an entry has exactly one of until and after_task. */
  later?: { [key: string]: Later | null };
};

/** The person's view preferences, one set for the founder's devices, kept by the daemon. A new daemon serves the defaults: no pins, unread_only false, hidden empty, layout tree, order empty. */
export type Prefs = {
  nav: NavPrefs;
};

export type ReadCursor = {
  /** The newest stream cursor when this read began. An event above it may be in the answer already (applying it again changes nothing); none at or below it is missing. A client drops the events at or below it for this answer. */
  cursor: number;
};

/** Something the machine needs of the founder, which no agent has: not a task and not an inbox message. */
export type MachineNeed = {
  /** disk: free space is under the floor (config disk_free_min_gb, else 10 GB). It stays until space is back over the floor plus 2 GB. */
  kind: "disk";
  since: Timestamp;
  /** The item in plain words, with the free space in it. */
  text: string;
};

/** attachment.remove deleted this image or file: a client marks every ImagePart and MemoFile with this id gone and removed, without refetching. */
export type AttachmentRemoved = {
  id: AttachmentId;
};

/** Why a For you item is out of its group and when it returns: at the time `until`, or when the task `after_task` is done or discarded; exactly one is present. The founder's choice, kept in NavPrefs.later under the item's `kind:id`; the daemon reads it in for_you.list and the apps hide and return the item on their clock and on task.changed. */
export type Later = unknown | unknown;

/** One open item waiting on the founder. A task given to the founder (or the ask on it) is `task`, an agent's `@human` ask in a task comment is `comment`, an agent's open permission prompt or dialog is `prompt`, a proposal that waits is `proposal`, and something the machine needs is `machine`. A plain say is not an item: nothing is asked. */
export type ForYouItem = {
  kind: "task" | "comment" | "prompt" | "proposal" | "machine";
  /** task: the task number. comment: the comment's number (InboxItem.memo), or the request's ref for an ask that is on no task. prompt: Prompt.id. proposal: the proposal's id (Proposal.id). machine: the warning, `disk`, `login:<account>`, `delivery:<ref>` or `undelivered:<task>`. The pair (kind, id) names the item, as in the keys of NavPrefs.later. */
  id: string;
  /** Who asks; null for the machine. */
  agent: Who | null;
  /** One line: a task's title, a comment's first line, a prompt's command or tool, a proposal's title, the warning in plain words. */
  title: string;
  /** What the item wants of the founder. A task or comment: `review` when the task is tagged review, else `decide`. A prompt: `allow` for a permission, else `decide`. An proposal: `allow`. The machine: `do`. */
  asks: "allow" | "decide" | "review" | "do";
  /** When it began to wait. */
  since: Timestamp;
  /** Only while the founder has the item deferred and the deferral is not over (NavPrefs.later). */
  later?: Later;
};

export type ForYouList = {
  /** Every open item, oldest `since` first (then kind and id), the deferred ones too. */
  items: ForYouItem[];
  /** The items without `later`: what a badge and the Requests count show. */
  count: number;
};

/** The data of a for_you.opened event: an item joined the count (it opened, or came back from Later). */
export type ForYouOpened = {
  item: ForYouItem;
  /** The count with it. */
  count: number;
};

/** The data of a for_you.closed event: an item left the count (answered anywhere, closed, discarded, withdrawn, or deferred). */
export type ForYouClosed = {
  kind: "task" | "comment" | "prompt" | "proposal" | "machine";
  id: string;
  /** The count without it. */
  count: number;
};

/** A machine a daemon runs on. The pause is a machine's state: with several machines each has its own. */
export type Machine = {
  /** The machine's id: made once by its daemon and kept, so a rename keeps it. */
  id: string;
  /** The machine's name as its owner set it (the computer name), else its host name. */
  name: string;
};

export interface CoreMethods {
  "hello": { params: {
  /** name/version of the caller. */
  client: string;
  /** The protocol the client will speak. */
  protocol: number;
  /** Device token; network listener only. */
  token?: string;
  /** The founder key, from the founder's own key file; local socket only. Wrong key: refused, gate `role`. */
  key?: string;
  /** The git sha the client was built from. When it is not the daemon's own, the daemon logs it once on this connection; nothing is refused. */
  client_build?: string;
  /** The directory the client runs in. A person's agent names resolve in the workspace whose project folder holds it; elsewhere only workspace names and handles resolve. An agent's resolve in its own workspace whatever this says. */
  cwd?: string;
  /** Local socket only: a record for the daemon to keep, which a command that writes nothing to the database cannot make itself. {agent: the $HANDLINE_AGENT it ran as, kind: verb_chain (a state-changing command ran) or nested_claude_refused, argv: its first three arguments, chain: the programs above it as program:pid joined by ' > '}. The daemon writes one event of that kind for the agent with argv and chain as its payload; a note it cannot read or write is dropped and changes nothing. */
  note?: { [key: string]: unknown };
}; result: {
    /** Newest protocol the daemon speaks. */
    protocol: number;
    /** Oldest client protocol it still serves. */
    min_client: number;
    /** The loaded build's identity: its git sha, or <version>+<sha>[-dirty] for a release build. */
    daemon: string;
    /** The same build identity as `daemon`, under the name daemon.status uses, so a client compares its own build with one field. */
    version?: string;
    /** The absolute path of the executable this process runs. */
    exe?: string;
    schema: number;
    role: Role;
    caller: Caller;
    capabilities: Capabilities;
    /** How this connection is protected. */
    transport: "local" | "tls";
    methods: MethodInfo[];
  } };
  "whoami": { params: { [key: string]: unknown }; result: {
    caller: Caller;
    role: Role;
    /** The same list hello gives, with allowed flags for this caller. */
    methods: MethodInfo[];
  } };
  "subscribe": { params: {
  topics: Topic[];
  /** Last cursor seen. Absent: live only. A cursor older than the daemon keeps, or with more than 5000 subscribed events after it, is answered by a `gap` notice (reason retention) instead of a replay; the connection stays open. Up to 5000 events behind, every missed event is replayed in full, in cursor order, before any live one. */
  since?: number;
  /** replay (the default): what was missed after `since` is sent first, as `since` says. latest: nothing missed is sent, neither a replay nor a gap; the reply's cursor is the newest and only later events follow. A client that resumes at the latest reads again what it shows: each read a stream event keeps current says its `cursor`, and an event at or below it is in that answer already. */
  resume?: "replay" | "latest";
  /** Only events of this agent, plus events that carry no agent. Meant for the chat topic. */
  agent?: Who;
}; result: {
    /** The newest cursor at subscribe time. */
    cursor: number;
  } };
  "unsubscribe": { params: {
  topics: Topic[];
}; result: { [key: string]: unknown } };
  "daemon.status": { params: { [key: string]: unknown }; result: DaemonStatus & ReadCursor };
  "daemon.pause": { params: {
  /** Go on by itself at this time; absent pauses until daemon.resume. A time not in the future is bad_request. */
  until?: Timestamp;
  request_id: RequestId;
}; result: Accepted };
  "daemon.resume": { params: {
  request_id: RequestId;
}; result: Accepted };
  "lease.take": { params: {
  agent: Who;
  kind: "heavy" | "simulator" | "browser" | "port";
  /** A lease kept across commands (hl use <kind> --hold) rather than one for the calling process. */
  hold?: boolean;
}; result: {
    /** The lease, for lease.release. */
    lease: number;
    /** How long the ask waited. */
    waited_s: number;
    /** A port lease: the port, given to the command as $HL_PORT. */
    port?: number;
  } };
  "lease.release": { params: {
  agent: Who;
  lease?: number;
  kind?: ("heavy" | "simulator" | "browser" | "port") | "";
}; result: {
    /** How many leases were given back. */
    released: number;
  } };
  "lease.extend": { params: {
  agent: Who;
  kind?: ("heavy" | "simulator" | "browser" | "port") | "";
}; result: {
    /** The leases extended. */
    extended: {
      lease: number;
      kind: "heavy" | "simulator" | "browser" | "port";
      /** When the lease now reaches its max_hold. */
      max_hold_at: string;
    }[];
  } };
  "lease.get": { params: { [key: string]: unknown }; result: {
    kinds: {
      kind: "heavy" | "simulator" | "browser" | "port";
      /** heavy_max, simulator_max or browser_max; for port, the size of port_range, 0 when unset; 0 is no limit. */
      max: number;
      held: {
        lease: number;
        agent: string;
        since: string;
        /** Taken with --hold, rather than for one command. */
        hold: boolean;
        /** Its processes at the last sample: the hl use process and its descendants, or the agent's processes of a hold's kind. */
        pids: number[];
        /** A reservation: the task it is kept for until its agent runs hl use on it (lease_claim). */
        reserved_for?: number;
        /** A reservation: when it goes to the next ask if not taken. */
        until?: Timestamp;
        /** A port lease: its port. */
        port?: number;
      }[];
      queue: {
        agent: string;
        since: string;
        /** 1 is next. */
        position: number;
        /** The wait in words, for the agent view: 'waiting for a browser, 2nd in line, held by a and b'. */
        says: string;
        /** The task the ask blocks (its gate waits on this lease); absent for a call held open. */
        task?: number;
      }[];
    }[];
  } };
  "load.get": { params: {
  /** Also give `series` for that many minutes back (0 or absent: none). */
  minutes?: number;
  /** Only this agent's rows (and series). */
  agent?: Who;
}; result: {
    /** The sample these figures are from. */
    at: string;
    /** Seconds between samples. */
    every_s: number;
    machine: {
      /** Logical cores. */
      cores: number;
      /** 1-minute load average. */
      load_1m: number;
      /** load_1m divided by cores. */
      load_per_core: number;
      ram_total_bytes: number;
      ram_used_bytes: number;
      ram_free_bytes: number;
      /** Free on the volume the daemon home is on. */
      disk_free_bytes: number;
      disk_total_bytes: number;
    };
    /** Every running process on the machine, owned or not, so the agents' and the unowned figures add up to it. */
    total: {
      /** Summed CPU of the running processes, in percent of one core. */
      cpu_pct: number;
      /** Summed resident memory. */
      rss_bytes: number;
      /** How many are running. */
      processes: number;
    };
    /** Agents with a process running or ended in the last 15 minutes, the heaviest first. */
    agents: {
      /** Summed CPU of the running processes, in percent of one core. */
      cpu_pct: number;
      /** Summed resident memory. */
      rss_bytes: number;
      /** How many are running. */
      processes: number;
      /** The agent. */
      agent: string;
      /** Of its running processes, how many were found by their environment. */
      found_by_env: number;
      /** How many by the pane's tree. */
      found_by_tree: number;
      /** Its three heaviest processes by CPU then memory, one that ended included. */
      top: {
        /** Process id. */
        pid: number;
        /** Parent process id (1 once it was reparented to init). */
        ppid: number;
        /** The agent that started it; null when none owns it. */
        agent: string | null;
        /** How the owner was found: env, the agent's name in the process's own environment (HANDLINE_AGENT, which survives a child reparented to init); tree, a descendant of the agent's terminal pane or of an owned process, for a process that cleared its environment. null when unowned. */
        found_by: ("env" | "tree") | null;
        /** The command and its first arguments, clipped to 120 characters. */
        command: string;
        /** When the process began. */
        started: string;
        /** CPU in percent of one core over the time between its last two samples (a process first seen is its CPU time over its age). */
        cpu_pct: number;
        /** Resident memory. */
        rss_bytes: number;
        /** false: it ended after `sampled`, and this is its last row, kept for 15 minutes. */
        running: boolean;
        /** The sample this row is from. */
        sampled: string;
      }[];
    }[];
    unowned: {
      /** Summed CPU of the running processes, in percent of one core. */
      cpu_pct: number;
      /** Summed resident memory. */
      rss_bytes: number;
      /** How many are running. */
      processes: number;
      /** The heaviest 20 running processes no agent owns. */
      top: {
        /** Process id. */
        pid: number;
        /** Parent process id (1 once it was reparented to init). */
        ppid: number;
        /** The agent that started it; null when none owns it. */
        agent: string | null;
        /** How the owner was found: env, the agent's name in the process's own environment (HANDLINE_AGENT, which survives a child reparented to init); tree, a descendant of the agent's terminal pane or of an owned process, for a process that cleared its environment. null when unowned. */
        found_by: ("env" | "tree") | null;
        /** The command and its first arguments, clipped to 120 characters. */
        command: string;
        /** When the process began. */
        started: string;
        /** CPU in percent of one core over the time between its last two samples (a process first seen is its CPU time over its age). */
        cpu_pct: number;
        /** Resident memory. */
        rss_bytes: number;
        /** false: it ended after `sampled`, and this is its last row, kept for 15 minutes. */
        running: boolean;
        /** The sample this row is from. */
        sampled: string;
      }[];
    };
    /** Every process an agent owns, ended ones included, then the heaviest unowned ones. */
    processes: {
      /** Process id. */
      pid: number;
      /** Parent process id (1 once it was reparented to init). */
      ppid: number;
      /** The agent that started it; null when none owns it. */
      agent: string | null;
      /** How the owner was found: env, the agent's name in the process's own environment (HANDLINE_AGENT, which survives a child reparented to init); tree, a descendant of the agent's terminal pane or of an owned process, for a process that cleared its environment. null when unowned. */
      found_by: ("env" | "tree") | null;
      /** The command and its first arguments, clipped to 120 characters. */
      command: string;
      /** When the process began. */
      started: string;
      /** CPU in percent of one core over the time between its last two samples (a process first seen is its CPU time over its age). */
      cpu_pct: number;
      /** Resident memory. */
      rss_bytes: number;
      /** false: it ended after `sampled`, and this is its last row, kept for 15 minutes. */
      running: boolean;
      /** The sample this row is from. */
      sampled: string;
    }[];
    /** The cost of measuring. */
    sampling: {
      /** Samples taken since the daemon started. */
      samples: number;
      /** Mean time one sample took (reading the process table and the environment of each new process). */
      mean_ms: number;
    };
    /** Present with `minutes`: each agent's per-minute rows for the window, oldest first; the key "" is the processes no agent owns. Kept for 7 days. */
    series?: { [key: string]: {
      /** The minute. */
      minute: string;
      /** Mean of the agent's summed CPU over the minute's samples. */
      cpu_avg_pct: number;
      /** Highest. */
      cpu_max_pct: number;
      /** Mean resident memory. */
      rss_avg_bytes: number;
      /** Highest. */
      rss_max_bytes: number;
      /** Most processes at once. */
      processes_max: number;
    }[] };
  } | null };
  "events.feed": { params: {
  since?: number;
  topics?: Topic[];
  limit?: number;
}; result: {
    events: Event[];
    cursor: number;
    /** True when `since` is older than retention; refetch snapshots. */
    gap: boolean;
  } };
  "operation.get": { params: {
  operation: string;
}; result: Operation & ReadCursor };
  "debug.terminals": { params: { [key: string]: unknown }; result: {
    host: string;
    now: Timestamp;
    panes: DebugTerminalPane[];
  } };
  "prefs.get": { params: { [key: string]: unknown }; result: Prefs & ReadCursor };
  "prefs.set": { params: {
  changes: {
    nav: NavPrefsChanges;
  };
  request_id: RequestId;
}; result: Accepted };
  "hook.record": { params: {
  agent: Who;
  kind: string;
  at: Timestamp;
  data: { [key: string]: unknown };
  request_id: RequestId;
}; result: Accepted & {
    /** Only for a Stop: the text that keeps the turn going, when a founder's message typed in this turn has no answer; the hook answers with it. Absent when there is none. A hook the daemon does not answer in time gets none. */
    nudge?: string;
  } };
  "subagent.start": { params: {
  agent: Who;
  session_id?: string;
  description?: string;
  subagent_type?: string;
  run_in_background?: boolean;
}; result: {
    /** Why the subagent may not start, in plain words with the times; the hook denies the call with it. Absent when it may start, and then its start is recorded. */
    deny?: string;
  } };
  "memory.status": { params: {
  agent?: Who;
}; result: {
    agents: {
      agent: Who;
      bytes: number;
      budget: number;
      entries: number;
      unused: string[];
    }[];
  } };
  "connection.get": { params: { [key: string]: unknown }; result: ConnectionInfo };
  "connection.addresses": { params: { [key: string]: unknown }; result: ConnectionAddresses };
  "settings.get": { params: {
  /** Return that agent's own overrides in `overrides`. */
  agent?: Who;
}; result: Settings };
  "settings.set": { params: {
  changes: SettingsChanges;
  /** Write the per-agent override of: park_timeout_s, ignore_cache_expiry, bg_notice_s, bg_grace_s, bg_check_s, notice_digest_s, handoff_prompt. Any other key with `agent` is bad_request. */
  agent?: Who;
  request_id: RequestId;
}; result: Accepted };
  "for_you.list": { params: { [key: string]: unknown }; result: ForYouList & ReadCursor };
}

export const CORE_METHODS: { [K in keyof CoreMethods]: { role: Role; callers: Role[] | null; write: boolean; since: number } } = {
  "hello": { role: "anonymous", callers: null, write: false, since: 1 },
  "whoami": { role: "agent", callers: null, write: false, since: 1 },
  "subscribe": { role: "agent", callers: null, write: false, since: 1 },
  "unsubscribe": { role: "agent", callers: null, write: false, since: 1 },
  "daemon.status": { role: "agent", callers: null, write: false, since: 1 },
  "daemon.pause": { role: "owner", callers: null, write: true, since: 1 },
  "daemon.resume": { role: "owner", callers: null, write: true, since: 1 },
  "lease.take": { role: "agent", callers: null, write: false, since: 1 },
  "lease.release": { role: "agent", callers: null, write: false, since: 1 },
  "lease.extend": { role: "agent", callers: null, write: false, since: 1 },
  "lease.get": { role: "agent", callers: null, write: false, since: 1 },
  "load.get": { role: "agent", callers: null, write: false, since: 1 },
  "events.feed": { role: "agent", callers: null, write: false, since: 1 },
  "operation.get": { role: "member", callers: null, write: false, since: 1 },
  "debug.terminals": { role: "member", callers: null, write: false, since: 1 },
  "prefs.get": { role: "member", callers: ["member", "owner"], write: false, since: 1 },
  "prefs.set": { role: "member", callers: ["member", "owner"], write: true, since: 1 },
  "hook.record": { role: "agent", callers: ["agent"], write: true, since: 1 },
  "subagent.start": { role: "agent", callers: ["agent"], write: true, since: 1 },
  "memory.status": { role: "agent", callers: null, write: false, since: 1 },
  "connection.get": { role: "owner", callers: null, write: false, since: 1 },
  "connection.addresses": { role: "owner", callers: null, write: false, since: 1 },
  "settings.get": { role: "owner", callers: null, write: false, since: 1 },
  "settings.set": { role: "owner", callers: null, write: true, since: 1 },
  "for_you.list": { role: "member", callers: null, write: false, since: 1 },
};

/** x-app-cache of each read (x-app in core.json). */
export const CORE_APP_CACHE: { [K in "hello" | "whoami" | "subscribe" | "unsubscribe" | "daemon.status" | "lease.take" | "lease.release" | "lease.extend" | "lease.get" | "load.get" | "events.feed" | "operation.get" | "debug.terminals" | "prefs.get" | "memory.status" | "connection.get" | "connection.addresses" | "settings.get" | "for_you.list"]: { resource: string; id?: string; feed: Topic | null } | false } = {
  "hello": false,
  "whoami": false,
  "subscribe": false,
  "unsubscribe": false,
  "daemon.status": {"resource": "status", "feed": "status"},
  "lease.take": false,
  "lease.release": false,
  "lease.extend": false,
  "lease.get": false,
  "load.get": {"resource": "load", "feed": null},
  "events.feed": false,
  "operation.get": {"resource": "operation", "id": "operation", "feed": "operations"},
  "debug.terminals": {"resource": "debug-terminals", "feed": null},
  "prefs.get": {"resource": "prefs", "feed": "prefs"},
  "memory.status": {"resource": "memory", "feed": null},
  "connection.get": {"resource": "connection", "feed": null},
  "connection.addresses": {"resource": "addresses", "feed": null},
  "settings.get": {"resource": "settings", "feed": null},
  "for_you.list": {"resource": "for-you", "feed": "for_you"},
};

/** x-app-write of each write (x-app in core.json). */
export const CORE_APP_WRITES: { [K in "daemon.pause" | "daemon.resume" | "prefs.set" | "hook.record" | "subagent.start" | "settings.set"]: { kind: WriteKind; label: string } | false } = {
  "daemon.pause": {"kind": "decided", "label": "pause new work"},
  "daemon.resume": {"kind": "decided", "label": "resume new work"},
  "prefs.set": {"kind": "authored", "label": "save view preferences"},
  "hook.record": false,
  "subagent.start": false,
  "settings.set": {"kind": "authored", "label": "save settings"},
};

export const PROTOCOL = 1;

export type ErrorCode = "not_found" | "refused" | "stale_prompt" | "daemon_busy" | "bad_request" | "internal" | "daemon_down";
export const RPC_CODES: { [K in ErrorCode]: number | null } = {
  not_found: -32001,
  refused: -32002,
  stale_prompt: -32003,
  daemon_busy: -32004,
  bad_request: -32600,
  internal: -32603,
  daemon_down: null,
};

export interface Notifications {
  event: Event;
  gap: {
  reason: "retention" | "slow_client";
  cursor: number;
  topic?: Topic;
};
  frame: {
  terminal: string;
  seq: number;
  full: boolean;
  lines?: Row[];
  changed?: {
    y: number;
    row: Row;
  }[];
  cursor: Cursor;
  /** Present on a `styled` attach's frames and absent on a text one's, so a connection that watches one terminal in both formats can tell which attach a frame belongs to. */
  format?: "styled";
};
  resize: {
  terminal: string;
  seq: number;
  cols: number;
  rows: number;
};
  marker: {
  terminal: string;
  seq: number;
  at: Timestamp;
  kind: "daemon_typed" | "control";
  actor: Actor | null;
  text?: string;
  /** The attach this notice belongs to, whose counter `seq` is: a terminal watched in both formats is sent each of these twice. Additive: a client that does not know it ignores it. */
  format: "text" | "bytes" | "styled";
};
  holder: {
  terminal: string;
  seq: number;
  holders: InputHolder[];
  /** The attach this notice belongs to, whose counter `seq` is: a terminal watched in both formats is sent each of these twice. Additive: a client that does not know it ignores it. */
  format: "text" | "bytes" | "styled";
};
  output: {
  terminal: string;
  seq: number;
  /** Base64 of what the program wrote. */
  data: string;
};
  snapshot: {
  terminal: string;
  seq: number;
  reason: "resize" | "slow_client";
  cols: number;
  rows: number;
  /** Base64, as in the attach snapshot. */
  data: string;
  alt_screen: boolean;
  cursor: {
    x: number;
    y: number;
  };
  cursor_visible: boolean;
};
  doc_changed: {
  agent: string;
  path: string;
  doc: string;
  /** The doc's version after these updates. */
  version: number;
  updates: DocUpdate[];
};
  doc_closed: {
  agent: string;
  path: string;
  doc: string;
  reason: "gone" | "unreadable";
};
}

/** The state words the daemon sends; an open set, so a client keeps a fallback. */
export const STATE_WORDS = ["running", "idle", "queued", "starting", "parked", "waiting", "stalled", "blocked", "no_terminal"] as const;

/** x-status: the state and reason every agent and task carries; open sets for a client, which shows `text` for a word it does not know. */
export const STATUS_STATES = ["preparing", "starting", "running", "waiting", "stopped", "idle", "off", "done"] as const;
export const STATUS_REASONS = ["work", "restart", "retry", "launch", "turn", "watch", "handoff", "background", "you", "opener", "task", "owner", "clock", "move", "load", "pace", "paused", "wall", "cap", "stalled", "none", "done", "discarded"] as const;

export const ROLE_ORDER: Role[] = ["anonymous", "agent", "member", "owner"];

export type WriteKind = "authored" | "decided" | "uncached";
