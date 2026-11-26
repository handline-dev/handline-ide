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

/** chat is added for the chat view (the UI design update): messages, landing state, transcript items and working state of one agent; events carry `agent`. devices (founder, which includes a paired device; live only): pairing and revocation of devices. proposals: proposals, so a client's tab dot is live; any proven caller, as proposal.list. transcript: the per-agent event stream of a structured session (transcript.event), served only for an agent the daemon runs as a structured session; see the Transcript stream section. prefs (member, which includes the founder): the person's view preferences, so a pin made on one device shows on the others. requests (member, which includes the founder): the founder's requests (RequestRow), so a Requests page is live. nodes (agent, as people and workspaces): the machines of this master, one event with no data on any change, so a client reads node.status again. for_you (member, which includes the founder): the set of open items waiting on the founder; an event whenever the count's set changes, with the new count. A client that misses events reads requests.pending again. */
export type Topic = "status" | "accounts" | "agents" | "people" | "workspaces" | "nodes" | "inbox" | "tasks" | "notifications" | "operations" | "chat" | "devices" | "proposals" | "transcript" | "prefs" | "requests" | "for_you";

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
  /** On a hook permission prompt's always option that keeps a rule: the exact rule, Tool(content). */
  rule?: string;
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
  /** hook: asked through permission.ask, answered allow or deny (or always, where offered) with no terminal typed into. Absent for a prompt read from the agent's screen. */
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
  tool: "claude" | "git" | "tmux" | "codex";
  /** A shell command that installs the tool, for a client to show and copy, for example "brew install tmux". */
  fix: string;
  /** The install window tools.install opened for it, while the terminal host keeps that window: running, or failed and not yet replaced by a new run or closed (one that ends 0 closes itself); absent otherwise and on a daemon that does not serve it. */
  install?: ToolInstall;
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
  /** The tools agents are run with (claude, git, tmux, and codex while an account or agent uses it) that this machine lacks, read at each call; absent when none is missing, when the daemon runs no agent side, and on a daemon that does not serve it. No agent can start while claude, git or tmux is missing, and no codex agent while codex is: a client shows each with its fix. */
  missing_tools?: MissingTool[];
  /** Every vendor this daemon can run agents with, installed or not, for the first-run pick; a client offers only the vendors it supports; absent on a daemon that does not serve it. Unlike missing_tools it says nothing about need: codex is in missing_tools only while an account or agent uses it. */
  vendors?: VendorStatus[];
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

/** The machine's state in one line. held, the first that applies: the founder's pause (daemon.pause); the load holding starts (Resources.start_hold, by its own rules); any needs_you item; any agent whose status reason is full or wall, with the count in text. near, when nothing holds: the load per core at least its limit less 10 percentage points (load_per_core_pct), or free memory less than its limit plus 10 (ram_free_min_pct), when that limit is set; never before the first measure. host_active_cap is never near: at it, starts are held. status.changed is told within 10 s of a change of level or text. */
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
  setup: RepoSetup;
};

/** What every worktree the daemon makes for the repo gets, set by repo.set. When a spawn makes a worktree (an agent's first, an extra one, or one made again from its branch), the daemon copies `copy` from the repo's own folder, then runs `script` in the new worktree; the agent starts after it. A failed copy runs no script. A failure is told in the spawn's report, as the agent's worktree_setup event and in `last`, never fatal: the agent still starts there. The founder's alone: kept in the protected store, which no agent can write, since the script runs outside any sandbox. */
export type RepoSetup = {
  /** Shell text, run as `$SHELL -lc` in the new worktree with HANDLINE_REPO (the repo's folder) and HANDLINE_WORKTREE set and nothing on stdin, for at most 5 minutes; "" runs nothing. At most 4000 characters. */
  script: string;
  /** Paths relative to the repo's folder, a glob each (`*`, `?`, `[...]`, no `**`), copied into the new worktree: a folder with what it holds. A file already in the worktree is kept, not overwritten. At most 50, each at most 300 characters, none absolute, above the repo or in .git. */
  copy: string[];
  /** The newest run of this setup in a new worktree; null before any. */
  last: RepoSetupRun | null;
};

export type RepoSetupRun = {
  /** When it ran, RFC 3339. */
  at: string;
  worktree: string;
  /** Every copy made and the script exited 0. */
  ok: boolean;
  /** The paths copied, relative to the repo. */
  copied: string[];
  /** Copy entries the repo's folder had nothing for: told, not a failure. */
  missing: string[];
  /** Why it failed in one line (`copying .env: ...`, `script exited 1`, `script stopped after 5m0s`); "" when ok. */
  error: string;
  /** The script's output, its last 4000 bytes. */
  output: string;
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
  /** This daemon is master: its node id and master number, raised by one each time the master moves (node.promote). Runners and apps obey only the highest number they have seen. Absent on a runner. */
  master?: {
    node?: string;
    number?: number;
  };
  cluster?: Cluster;
};

/** The cluster this daemon is the master of: its name (cluster_name) and how many machines it has, the master's own and those lent on Run or Pause, as node.status lists them. */
export type Cluster = {
  name: string;
  machines: number;
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

/** Every global setting by its config.yaml name, with its default; every one is the founder's to set. Per-account keys (label, usage recovery, cycle) are account.set; per-agent keys are agent.set and schedule.*; a key marked Per agent also takes `agent` in settings.set. */
export type SettingsValues = {
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
  /** How long a background job runs before its agent is asked whether to stop it. Per agent. */
  bg_check_s?: number;
  /** Seconds after the first waiting notice before the agent is typed one digest, at its prompt. Per agent. */
  notice_digest_s?: number;
  /** Input price per million tokens by model-name pattern, laid over the built-in table; an entry that is not a positive number is ignored. */
  token_prices?: { [key: string]: number };
  /** NEW, no code today: seconds a person or device may hold a terminal's input without typing before the daemon takes it back (see terminal `holder` notice). */
  input_hold_idle_s?: number;
  /** Tell a queued task's holder when other open tasks have waited on it (blocked by it, or its parent) for blocker_alarm_s, and the queued task's opener at blocker_alarm_opener_s (the founder when no agent opened it, or an hour later if it has not moved); once each per wait. */
  blocker_alarm?: boolean;
  /** Seconds the first open task waiting on a queued task may wait before its holder is told (blocker_alarm). */
  blocker_alarm_s?: number;
  /** Seconds the first open task waiting on a queued task may wait before the queued task's opener is told (blocker_alarm); the founder hears when no agent opened it, or an hour later if it has not moved. */
  blocker_alarm_opener_s?: number;
  /** How many times a task of a request may be sent back (reopened) before the request shows attention bounced. */
  attention_sent_back?: number;
  /** Seconds a request's open tree may go with no activity before it shows attention quiet. */
  attention_quiet_s?: number;
  /** Seconds an open task of a request may wait on full, wall or owner before the request shows attention waiting (RequestRow.attention). */
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
  /** Notices only while no Handline window is in front; off, the default, holds back only a notice about the agent on screen in a focused window. The shell that raises notices reads this. */
  push_only_away?: boolean;
  /** Whether a notice plays the system sound. The shell that raises notices reads this. */
  push_sound?: boolean;
  /** Most agents mid-turn at once on this machine, over every account; 0 is no limit. Over it the daemon starts no new work (status reason load). */
  host_active_cap?: number;
  /** Most 1-minute load per core, in percent (150 is 1.5 per core); 0 is off, the default (the limits wait for a limit based on what an agent's processes use). Over it the daemon starts no new work. */
  load_per_core_pct?: number;
  /** Least free memory, in percent of all; 0 is off, the default. Under it the daemon starts no new work. */
  ram_free_min_pct?: number;
  /** Least free disk on the volume the daemon home is on, in GB; 0 is off, the default. The disk needs-you item uses it as its floor when it is set, else 10 GB. */
  disk_free_min_gb?: number;
  /** Where each account's pace line ends: its weekly usage, in percent, at the end of its week (its weekly reset, or its subscription end when that comes first). Under 100 because a reading lags by minutes. */
  pace_target_pct?: number;
  /** How far, in points of the week, an account may run ahead of its pace line and read as ahead rather than over (Account.pace.state). It holds no start. */
  pace_burst_pct?: number;
  /** Refuse a subagent (the Agent tool) when one more would take the agent's account to its 5-hour or weekly limit before that window resets (subagent.start). Off: every start is recorded and none is refused. Off by default until the founder turns it on. */
  subagent_pace_guard?: boolean;
  /** How agents are placed on the cluster's devices at each session's start: auto, the default, keeps a light agent on the master's machine and sends a heavy one (over 2 h of heavy commands or a 6 GB peak in its last 7 days) to the lent device with the most room that has all it needs; manual starts an agent pinned to a device (its device setting) there and places the others as auto does. Switching keeps the pins. */
  agent_placement?: "auto" | "manual";
  /** How agents are spread over the accounts: auto, the default, places each new start and moves an agent off an account near its wall; manual never does: an agent stays on the account it is on, waits at its wall for the reset and goes on with the same conversation. A person's move (agent.move) goes in both. Applied at the next start or move, without a restart. Per agent with `agent`: that agent's own value, over the global one; manual keeps it on the account it is on, wherever a person moves it. */
  account_split?: "auto" | "manual";
  /** Whether a task that comes to an agent mid-turn on another goes to a seat of it: auto, the default, gives it to the agent's lowest idle seat, or a new one (the agent's name and a number, with its settings, role, lessons and memory folder, in a desk of its own), on a fresh conversation; the seat stays, idle, once the task is over. manual keeps the task waiting for the agent. A seat starts only where a start would: the load, agent_cap and account limits hold. */
  temp_workers?: "auto" | "manual";
  /** With it on, a task that serves the founder (in a founder request's tree, or the founder's own) is Ready for the founder when its assignee finishes it to its land_by agent, as a story's sub-task is; the founder's OK lands it and Redo sends it back. Off by default. */
  review_flow?: boolean;
  /** When an idle agent's account reaches this percent of its 5-hour window, or 98% of its week, and work is waiting for it, the agent moves to an account under both that has room, before the work starts a turn; 90 by default; 0 is off, and an agent moves only at the wall. */
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
  /** The ports a port lease (hl use port) is picked from, as lo-hi (e.g. 4000-4999) with 1024 <= lo <= hi <= 65535: a port no lease holds and that binds at grant time, given to the command as $HL_PORT; over the range an ask waits. Unset, the default, a port lease is any free port the system gives and none waits. */
  port_range?: string | null;
  /** Hosts the network listener serves, from connection.addresses, each on the -listen port: a door on each of their addresses, made again within seconds when the setting or the interfaces change (no restart). The web page is served on each. null (the default) serves -listen alone; [] serves none. A host the machine has no address for now is served when it has one again. A write answers unreachable_devices: the paired devices whose host is no longer served. */
  network_addresses?: string[] | null;
  /** The model a new claude agent gets when it is made without one: an agent.options model alias or a full model id, passed to the CLI as is. One word, not starting with -. Existing agents keep theirs. null puts back the default. */
  default_model_claude?: string;
  /** The effort a new claude agent gets when it is made without one. Existing agents keep theirs. null puts back the default. */
  default_effort_claude?: "low" | "medium" | "high" | "xhigh" | "max";
  /** The model a new codex agent (one hired by proposal; agent.create makes claude agents) gets when it is made without one, passed to codex as is. One word, not starting with -. Default "": codex's own default. Existing agents keep theirs. null puts back the default. */
  default_model_codex?: string;
  /** The effort a new codex agent gets when it is made without one; codex runs xhigh and max as high. Existing agents keep theirs. null puts back the default. */
  default_effort_codex?: "low" | "medium" | "high" | "xhigh" | "max";
  /** Free disk to keep on the volume the daemon home is on, in GB; 0 is off, the default. Under it the daemon removes free, unpinned desks of any project, oldest first, never one holding changes, until it is back over it. */
  disk_keep_free_gb?: number;
  /** Most memory the agents' processes may use together, in GB; 0 is off, the default. Over it the daemon closes the longest-idle sessions one at a time, never one mid-turn; their work stays and they wake from their handoff. */
  agents_memory_max_gb?: number;
  /** A lent Mac whose heartbeat says it went on battery is set to Off, and back to Run when it is plugged in again. Off by default. */
  node_battery_switch?: boolean;
  /** Seconds a lent machine may be offline before the master takes its sessions back to start them elsewhere. */
  node_reclaim_s?: number;
  /** A lent machine of the owner's, of the master's OS and architecture, on another Handline build takes the master's own build and starts again on it, so it can take work; its sessions go on. Off is Manual: its row says to update, and its Update does the same once. Another person's machine takes it only at its own person's Update. */
  node_build_auto?: boolean;
  /** The cluster's name, one line. A board without one gets two random words in Title Case ("Blue Harbor") at the daemon's start; the owner renames it. */
  cluster_name?: string;
  /** Refuse hl screen (a window captured through the daemon) for every agent. Off: an agent may capture a window. Per agent with `agent`: that agent's own switch, on over a global one that is off. */
  block_screen_capture?: boolean;
  /** Refuse a shell screencapture in an agent's Bash command and name the way through Handline instead: hl screen for a window, hl browser shot for a web page, hl sim screenshot for a phone app, node tools/app.mjs shot for Handline's own test app. Off: the command runs. */
  screencapture_via_handline?: boolean;
};

/** settings.set's changes: any of SettingsValues' keys; null removes one. */
export type SettingsChanges = {
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
  /** How long a background job runs before its agent is asked whether to stop it. Per agent. null removes the key (back to the default). */
  bg_check_s?: number | null;
  /** Seconds after the first waiting notice before the agent is typed one digest, at its prompt. Per agent. null removes the key (back to the default). */
  notice_digest_s?: number | null;
  /** Input price per million tokens by model-name pattern, laid over the built-in table; an entry that is not a positive number is ignored. null removes the key (back to the default). */
  token_prices?: { [key: string]: number } | null;
  /** NEW, no code today: seconds a person or device may hold a terminal's input without typing before the daemon takes it back (see terminal `holder` notice). null removes the key (back to the default). */
  input_hold_idle_s?: number | null;
  /** Tell a queued task's holder when other open tasks have waited on it (blocked by it, or its parent) for blocker_alarm_s, and the queued task's opener at blocker_alarm_opener_s (the founder when no agent opened it, or an hour later if it has not moved); once each per wait. */
  blocker_alarm?: boolean;
  /** Seconds the first open task waiting on a queued task may wait before its holder is told (blocker_alarm). null removes the key (back to the default). */
  blocker_alarm_s?: number | null;
  /** Seconds the first open task waiting on a queued task may wait before the queued task's opener is told (blocker_alarm); the founder hears when no agent opened it, or an hour later if it has not moved. null removes the key (back to the default). */
  blocker_alarm_opener_s?: number | null;
  /** How many times a task of a request may be sent back (reopened) before the request shows attention bounced. null removes the key (back to the default). */
  attention_sent_back?: number | null;
  /** Seconds a request's open tree may go with no activity before it shows attention quiet. null removes the key (back to the default). */
  attention_quiet_s?: number | null;
  /** Seconds an open task of a request may wait on full, wall or owner before the request shows attention waiting (RequestRow.attention). null removes the key (back to the default). */
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
  /** Notices only while no Handline window is in front; off, the default, holds back only a notice about the agent on screen in a focused window. The shell that raises notices reads this. null removes the key (back to the default). */
  push_only_away?: boolean | null;
  /** Whether a notice plays the system sound. The shell that raises notices reads this. null removes the key (back to the default). */
  push_sound?: boolean | null;
  /** Most agents mid-turn at once on this machine, over every account; 0 is no limit. Over it the daemon starts no new work (status reason load). null removes the key (back to the default). */
  host_active_cap?: number | null;
  /** Most 1-minute load per core, in percent (150 is 1.5 per core); 0 is off, the default (the limits wait for a limit based on what an agent's processes use). Over it the daemon starts no new work. null removes the key (back to the default). */
  load_per_core_pct?: number | null;
  /** Least free memory, in percent of all; 0 is off, the default. Under it the daemon starts no new work. null removes the key (back to the default). */
  ram_free_min_pct?: number | null;
  /** Least free disk on the volume the daemon home is on, in GB; 0 is off, the default. The disk needs-you item uses it as its floor when it is set, else 10 GB. null removes the key (back to the default). */
  disk_free_min_gb?: number | null;
  /** Where each account's pace line ends: its weekly usage, in percent, at the end of its week (its weekly reset, or its subscription end when that comes first). Under 100 because a reading lags by minutes. null removes the key (back to the default). */
  pace_target_pct?: number | null;
  /** How far, in points of the week, an account may run ahead of its pace line and read as ahead rather than over (Account.pace.state). It holds no start. null removes the key (back to the default). */
  pace_burst_pct?: number | null;
  /** Refuse a subagent (the Agent tool) when one more would take the agent's account to its 5-hour or weekly limit before that window resets (subagent.start). Off: every start is recorded and none is refused. Off by default until the founder turns it on. */
  subagent_pace_guard?: boolean;
  /** How agents are placed on the cluster's devices at each session's start: auto, the default, keeps a light agent on the master's machine and sends a heavy one (over 2 h of heavy commands or a 6 GB peak in its last 7 days) to the lent device with the most room that has all it needs; manual starts an agent pinned to a device (its device setting) there and places the others as auto does. Switching keeps the pins. null removes the key (back to the default). */
  agent_placement?: ("auto" | "manual") | null;
  /** How agents are spread over the accounts: auto, the default, places each new start and moves an agent off an account near its wall; manual never does: an agent stays on the account it is on, waits at its wall for the reset and goes on with the same conversation. A person's move (agent.move) goes in both. Applied at the next start or move, without a restart. Per agent with `agent`: that agent's own value, over the global one; manual keeps it on the account it is on, wherever a person moves it. null removes the key (back to the default). */
  account_split?: ("auto" | "manual") | null;
  /** Whether a task that comes to an agent mid-turn on another goes to a seat of it: auto, the default, gives it to the agent's lowest idle seat, or a new one (the agent's name and a number, with its settings, role, lessons and memory folder, in a desk of its own), on a fresh conversation; the seat stays, idle, once the task is over. manual keeps the task waiting for the agent. A seat starts only where a start would: the load, agent_cap and account limits hold. null removes the key (back to the default). */
  temp_workers?: ("auto" | "manual") | null;
  /** With it on, a task that serves the founder (in a founder request's tree, or the founder's own) is Ready for the founder when its assignee finishes it to its land_by agent, as a story's sub-task is; the founder's OK lands it and Redo sends it back. Off by default. */
  review_flow?: boolean;
  /** When an idle agent's account reaches this percent of its 5-hour window, or 98% of its week, and work is waiting for it, the agent moves to an account under both that has room, before the work starts a turn; 90 by default; 0 is off, and an agent moves only at the wall. null removes the key (back to the default). */
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
  /** The ports a port lease (hl use port) is picked from, as lo-hi (e.g. 4000-4999) with 1024 <= lo <= hi <= 65535: a port no lease holds and that binds at grant time, given to the command as $HL_PORT; over the range an ask waits. Unset, the default, a port lease is any free port the system gives and none waits. Another value is bad_request (field port_range). null removes the key (back to the default). */
  port_range?: string | null;
  /** Hosts the network listener serves, from connection.addresses, each on the -listen port: a door on each of their addresses, made again within seconds when the setting or the interfaces change (no restart). The web page is served on each. null (the default) serves -listen alone; [] serves none. A host the machine has no address for now is served when it has one again. A write answers unreachable_devices: the paired devices whose host is no longer served. A host this machine cannot be reached on is refused, gate address. A client connected through a host it drops is cut off before the answer; its retry with the same request_id reads it. null removes the key (back to the default). */
  network_addresses?: string[] | null;
  /** The battery switch (SettingsValues.node_battery_switch). null removes the key (back to the default). */
  node_battery_switch?: boolean | null;
  /** Seconds a lent machine may be offline before its sessions are taken back (SettingsValues.node_reclaim_s). null removes the key (back to the default). */
  node_reclaim_s?: number | null;
  /** Auto or Manual for a lent machine on another build (SettingsValues.node_build_auto). null removes the key (back to the default). */
  node_build_auto?: boolean | null;
  /** The cluster's new name (SettingsValues.cluster_name), trimmed; a blank one, one over 40 characters, more than one line or null is bad_request naming cluster_name: a cluster always has a name. */
  cluster_name?: string;
  /** Refuse hl screen (a window captured through the daemon) for every agent. Off: an agent may capture a window. Per agent with `agent`: that agent's own switch, on over a global one that is off. null removes the key (back to the default). */
  block_screen_capture?: boolean | null;
  /** Refuse a shell screencapture in an agent's Bash command and name the way through Handline instead: hl screen for a window, hl browser shot for a web page, hl sim screenshot for a phone app, node tools/app.mjs shot for Handline's own test app. Off: the command runs. null removes the key (back to the default). */
  screencapture_via_handline?: boolean | null;
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

/** How the person's agent list and dock show agents and workspaces, the same on every device. */
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
  /** The workspaces whose section the list shows folded, by key (Workspace.key). No key twice. */
  folded: string[];
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
  /** The workspaces whose section the list shows folded, by key (Workspace.key). No key twice. */
  folded?: string[];
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

/** Why a For you item is out of its group and when it returns: at the time `until`, or when the task `after_task` is done or discarded; exactly one is present. The founder's choice, kept in NavPrefs.later under the item's `kind:id`; the daemon reads it in the for_you count and the apps hide and return the item on their clock and on task.changed. */
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
  /** What the item wants of the founder. A task or comment: `review` when the task is tagged review, else `decide`. A draft queued work waits on, or one its agent left behind: `run`, since the first task began to wait or the agent moved on. A prompt: `allow` for a permission, else `decide`. An proposal: `allow`. The machine: `do`. */
  asks: "allow" | "decide" | "review" | "do" | "run";
  /** When it began to wait. */
  since: Timestamp;
  /** Only while the founder has the item deferred and the deferral is not over (NavPrefs.later). */
  later?: Later;
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

export type VendorStatus = {
  /** The vendor as account.add takes it: claude, codex, or another the daemon has a driver for. */
  vendor: string;
  /** The command the daemon runs for it, for example "codex". */
  binary: string;
  /** Whether the command is on the daemon's PATH, read at each call. */
  installed: boolean;
  /** A shell command that installs it, for a client to show and copy, for example "npm install -g @openai/codex". */
  fix: string;
  /** As MissingTool.install, while installed is false. */
  install?: ToolInstall;
};

export type ToolInstall = {
  /** The install window's handle, as tools.install answered it and terminal.attach takes it. */
  terminal: string;
  /** Whether its command failed (one that ends 0, the tool found, closes its window): false while it runs; git's runs until Apple's installer closes, and fails when the command line tools did not come. The window's last line says how it failed. */
  ended: boolean;
};

/** One thing search found, with what opens it: a task by its number, a comment by its task and its id, a message by its number and the chat that shows it. */
export type SearchHit = {
  /** task: a task, found by its title, its brief or what its end kept; comment: a comment on a task; message: a chat message. */
  kind: "task" | "comment" | "message";
  /** When the task was opened, or the comment or message written. */
  at: Timestamp;
  /** One line of the text around the first match, at most 160 characters, with … where it was cut. A mention is as stored (<@key>). */
  snippet: string;
  /** Only a task: where the text was found, the first of these that holds it. result is its done evidence or its report. */
  field?: "title" | "brief" | "result";
  /** A task: its number. A comment: the task it is on. A message: the first task it delivers, when it delivers one. */
  task?: number;
  /** The title of `task`. */
  title?: string;
  /** Only a comment: its id (Memo.id). */
  memo?: number;
  /** Only a message: its number; chat.locate gives the page around it. */
  request?: number;
  /** Only a message: the agent whose chat shows it. */
  agent?: Who;
  /** Only a message: which of that agent's chats shows it (as in chat.locate). */
  scope?: "mine" | "all";
  /** A comment: its author. A message: who sent it. */
  from?: Who;
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
  /** port, or the name of a shared run the agent takes (run.list). */
  kind: string;
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
  /** port, or the name of a shared run the agent takes (run.list). Empty or absent: every kind. */
  kind?: string;
}; result: {
    /** How many leases were given back. */
    released: number;
  } };
  "lease.extend": { params: {
  agent: Who;
  /** port, or the name of a shared run the agent takes (run.list). Empty or absent: every kind. */
  kind?: string;
}; result: {
    /** The leases extended. */
    extended: {
      lease: number;
      /** port or a shared run's scope:name. */
      kind: string;
      /** When the lease now reaches its max_hold. */
      max_hold_at: string;
    }[];
  } };
  "lease.get": { params: { [key: string]: unknown }; result: {
    kinds: {
      /** port, or a shared run's scope:name (mac:xcode, w3:xcode). */
      kind: string;
      /** A shared run's at_once; for port, the size of port_range, 0 when unset; 0 is no limit. */
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
        /** The wait in words, for the agent view: 'waiting for a turn at xcode, 2nd in line, held by a and b'. */
        says: string;
        /** The task the ask blocks (its gate waits on this lease); absent for a call held open. */
        task?: number;
      }[];
    }[];
  } };
  "run.list": { params: {
  agent?: Who;
}; result: {
    runs: {
      /** Its lease kind, scope:name (lease.get). */
      kind: string;
      name: string;
      /** mac, or the workspace's key (w3). */
      scope: string;
      /** A program (xcodebuild), a program and its first words after its flags (go test), a script path (tools/check.sh, also as ./, bash or an absolute path), or a preset. */
      cmds: string[];
      at_once: number;
      why: string;
      set_by: string;
      set_at: Timestamp;
      held: {
        agent: string;
        since: Timestamp;
        hold: boolean;
      }[];
      waiting: {
        agent: string;
        since: Timestamp;
        position: number;
        /** The task the ask blocks; absent for a call held open. */
        task?: number;
      }[];
    }[];
    /** The patterns a run may cover that name what Handline already knows: @build-test (a test or build run), @simulator, @browser. */
    presets: string[];
  } };
  "run.set": { params: {
  name: string;
  scope: "mac" | "workspace";
  workspace?: string;
  cmds: string[];
  at_once?: number;
  why?: string;
  request_id: RequestId;
}; result: Accepted };
  "run.remove": { params: {
  name: string;
  scope: "mac" | "workspace";
  workspace?: string;
  request_id: RequestId;
}; result: Accepted };
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
  "search": { params: {
  /** The text to find, 1 to 200 characters once trimmed. Letters match whatever their case (ASCII); % and _ are plain characters. */
  query: string;
  /** The most hits of each kind, default 10, at most 50. */
  limit?: number;
}; result: {
    /** The tasks, then the comments, then the messages; each kind newest first. */
    hits: SearchHit[];
    /** A kind had more hits than `limit`. */
    more: boolean;
  } };
  "operation.get": { params: {
  operation: string;
}; result: Operation & ReadCursor };
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
  "connection.addresses": { params: { [key: string]: unknown }; result: ConnectionAddresses };
  "settings.get": { params: {
  /** Return that agent's own overrides in `overrides`. */
  agent?: Who;
}; result: Settings };
  "settings.set": { params: {
  changes: SettingsChanges;
  /** Write the per-agent override of: park_timeout_s, ignore_cache_expiry, bg_notice_s, bg_grace_s, bg_check_s, notice_digest_s, handoff_prompt, account_split, block_screen_capture. Any other key with `agent` is bad_request. */
  agent?: Who;
  request_id: RequestId;
}; result: Accepted };
  "warning.dismiss": { params: {
  /** The warning's id: PendingRequest.id of a row whose source is machine. */
  id: string;
  request_id: RequestId;
}; result: Accepted };
  "screen.list": { params: {
  agent: Who;
}; result: {
    windows: {
      id: number;
      app: string;
      title: string;
    }[];
  } };
  "screen.shot": { params: {
  agent: Who;
  /** The app's name as screen.list shows it; its front window is taken. */
  app?: string;
  /** A window id from screen.list; wins over app. */
  window?: number;
}; result: {
    path: string;
    app: string;
    title: string;
    window: number;
  } };
  "screen.policy": { params: {
  agent: Who;
}; result: {
    via: boolean;
  } };
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
  "run.list": { role: "agent", callers: null, write: false, since: 1 },
  "run.set": { role: "agent", callers: null, write: true, since: 1 },
  "run.remove": { role: "agent", callers: null, write: true, since: 1 },
  "load.get": { role: "agent", callers: null, write: false, since: 1 },
  "events.feed": { role: "agent", callers: null, write: false, since: 1 },
  "search": { role: "member", callers: null, write: false, since: 1 },
  "operation.get": { role: "member", callers: null, write: false, since: 1 },
  "prefs.get": { role: "member", callers: ["member", "owner"], write: false, since: 1 },
  "prefs.set": { role: "member", callers: ["member", "owner"], write: true, since: 1 },
  "hook.record": { role: "agent", callers: ["agent"], write: true, since: 1 },
  "subagent.start": { role: "agent", callers: ["agent"], write: true, since: 1 },
  "connection.addresses": { role: "owner", callers: null, write: false, since: 1 },
  "settings.get": { role: "owner", callers: null, write: false, since: 1 },
  "settings.set": { role: "owner", callers: null, write: true, since: 1 },
  "warning.dismiss": { role: "owner", callers: null, write: true, since: 1 },
  "screen.list": { role: "agent", callers: ["agent"], write: false, since: 1 },
  "screen.shot": { role: "agent", callers: ["agent"], write: false, since: 1 },
  "screen.policy": { role: "agent", callers: ["agent"], write: false, since: 1 },
};

/** x-app-cache of each read (x-app in core.json). */
export const CORE_APP_CACHE: { [K in "hello" | "whoami" | "subscribe" | "unsubscribe" | "daemon.status" | "lease.take" | "lease.release" | "lease.extend" | "lease.get" | "run.list" | "load.get" | "events.feed" | "search" | "operation.get" | "prefs.get" | "connection.addresses" | "settings.get" | "screen.list" | "screen.shot" | "screen.policy"]: { resource: string; id?: string; feed: Topic | null } | false } = {
  "hello": false,
  "whoami": false,
  "subscribe": false,
  "unsubscribe": false,
  "daemon.status": {"resource": "status", "feed": "status"},
  "lease.take": false,
  "lease.release": false,
  "lease.extend": false,
  "lease.get": {"resource": "leases", "feed": null},
  "run.list": {"resource": "runs", "feed": null},
  "load.get": {"resource": "load", "feed": null},
  "events.feed": false,
  "search": {"resource": "search", "feed": null},
  "operation.get": {"resource": "operation", "id": "operation", "feed": "operations"},
  "prefs.get": {"resource": "prefs", "feed": "prefs"},
  "connection.addresses": {"resource": "addresses", "feed": null},
  "settings.get": {"resource": "settings", "feed": null},
  "screen.list": false,
  "screen.shot": false,
  "screen.policy": false,
};

/** x-app-write of each write (x-app in core.json). */
export const CORE_APP_WRITES: { [K in "daemon.pause" | "daemon.resume" | "run.set" | "run.remove" | "prefs.set" | "hook.record" | "subagent.start" | "settings.set" | "warning.dismiss"]: { kind: WriteKind; label: string } | false } = {
  "daemon.pause": {"kind": "decided", "label": "pause new work"},
  "daemon.resume": {"kind": "decided", "label": "resume new work"},
  "run.set": {"kind": "decided", "label": "keep a shared run"},
  "run.remove": {"kind": "decided", "label": "remove a shared run"},
  "prefs.set": {"kind": "authored", "label": "save view preferences"},
  "hook.record": false,
  "subagent.start": false,
  "settings.set": {"kind": "authored", "label": "save settings"},
  "warning.dismiss": {"kind": "decided", "label": "dismiss a warning"},
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
export const STATUS_REASONS = ["work", "restart", "retry", "launch", "turn", "watch", "handoff", "background", "you", "opener", "task", "owner", "clock", "move", "load", "paused", "wall", "full", "stalled", "none", "done", "discarded"] as const;

export const ROLE_ORDER: Role[] = ["anonymous", "agent", "member", "owner"];

export type WriteKind = "authored" | "decided" | "uncached";
