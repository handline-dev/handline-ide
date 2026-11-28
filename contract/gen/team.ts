// Generated from contract/spec/ by contract/generate.py; do not edit.
/* eslint-disable */
import type { OperationStarted } from './agents.ts';
import type { Accepted, ReadCursor, RequestId, Role, Timestamp, Topic, Who, WriteKind } from './core.ts';

export type TeamAgent = {
  /** The agent's key (a17). */
  key: string;
  /** The word the caller uses for this agent: its name in the caller's workspace, another workspace's lead by that workspace's name, any other agent as name (workspace). */
  name: string;
  role: string;
};

export type Proposal = {
  id: number;
  /** The agent the proposal is about, as the card heads it `<kind> <agent>`. For a bundle: `<N> changes`, not an agent's name. */
  agent: string;
  /** One line, the card's head: `<kind> <agent>`; a bundle's is `bundle <N> changes`. Text from agents' names; draw as text. */
  title: string;
  /** What approving does, in words, one change per line. Not a unified diff. */
  diff: string;
  /** open: waits for the founder. applying: approved by the founder and held until each agent in `waiting_on` ends its turn; the daemon then closes those sessions, applies the change and restarts each agent that stays; it turns approved once the change is applied (an agent that stays and does not start at once is `follow_up`, not a failure), or apply_failed when the change itself fails; `settled` stays null and the founder is offered no action meanwhile. approved: decided and applied. rejected. apply_failed: approved by the founder but not applied (the apply failed, or never ran): still the founder's move, and proposal.decide approve again applies it. withdrawn: taken back by its author or the founder. An open set: a client draws a word it does not know as text, offers no action on it and does not count it. See proposal.list for the proposal file's own states. */
  state: string;
  /** Set only while state is applying: the agents whose running turn the change waits on, by their names before the change. A card names them. */
  waiting_on?: Who[];
  /** Set only while state is applying: when the proposal turned applying. With `waiting_on` it tells the card since when and why it waits (those agents' turns are running); proposal.apply_now ends the wait. */
  waiting_since?: Timestamp;
  /** Set on an approved proposal whose change is applied but an agent it renamed is not running yet. Absent when the change needs no start or the start went through; it is removed when the start succeeds. The change is applied either way: a client never draws it as not applied or offers approve again. */
  follow_up?: {
    agent: Who;
    /** waiting: the start is put off for machine load and the daemon makes it by itself when the load drops; the card shows no retry. failed: the start failed for another reason and only the founder's Start now (agent.force_start) retries it. An open set: draw a word you do not know as text. */
    state: string;
    /** Why, from the daemon (the held limit or the failure), one line. */
    text: string;
  };
  created: Timestamp;
  /** Digest of what the proposal changes as drawn; proposal.decide's approve hands it back. */
  subject: string;
  /** Who asked: an agent's key, or the person's for the founder's own change (a retire held for a running session, say); a client draws a person key as the person, never as an agent. Text from the daemon's record; optional. */
  requested_by?: string;
  /** Why it asks, as the agent wrote it. Text only, from an agent: a client draws it as plain text, never as markup, and drops control characters. Optional. */
  reason?: string;
  /** The founder's note on the decision (why it was rejected, say), as proposal.decide took it. Absent when the founder gave none or it is still open. */
  note?: string;
  /** The task its asking agent was doing when it asked, on any kind; task.get's timeline shows a run proposal there as a run row. Absent when the agent was doing none, or the founder asked. */
  task?: number;
  /** The fields the proposal asks for (vendor, model, effort, team, repo, to, then each `set <key>`). Every value is text from an agent (same rule as `reason`); optional, and a key it did not ask for is absent. Absent for kind bundle: its `diff` carries the changes. members, repos, relation, response and context texts are in `diff` only. */
  asked?: {
    vendor?: string;
    model?: string;
    team?: string;
    repo?: string;
    /** rename: the new name. */
    to?: string;
    /** `set <key>`: each agent setting asked for, key to value. */
    sets?: { [key: string]: string };
    effort?: string;
  };
  error?: string | null;
  settled?: Timestamp | null;
  /** Each change approving makes, in the order it is applied: the proposal itself, or each step of a bundle. A context step is one entry per file it replaces. proposal.list only: the proposals events leave it out. Optional: absent from a daemon that does not serve it. */
  changes?: {
    /** The step's kind, as Proposal.kind words it. */
    kind: string;
    /** The step's subject as the proposal names it, as Proposal.agent does: an agent's name, or for team and global the name they set. */
    agent: string;
    /** This step's lines of Proposal.diff. */
    diff: string;
    /** Context only: the instruction it replaces, role, base or rules. */
    file?: string;
    /** Context only: the file's whole text in force now, "" for none yet. Set only while the proposal waits (open, applying, apply_failed) and the agent exists; a client diffs it against `after`. */
    before?: string;
    /** Context only: the whole text approving sets, trimmed as it is kept. Text from an agent: draw as plain text. */
    after?: string;
  }[];
  /** What the proposal does: add, retire, rename, team, settings, context, schedule, sandbox (one entry for the agent's sandbox_allow; diff says it in plain words, `reach pypi.org`), run (a shell command the founder runs with the approve) or bundle (several changes decided once). An open set: a client draws a word it does not know as text. */
  kind?: string;
  /** Kind run only: the command and, once the approve started it, its run. The run's output goes to the proposing agent: a card says so before the press. */
  run?: {
    /** The command in full, as it will run; text from an agent: draw it as text, never cut. */
    command: string;
    /** The absolute folder it runs in. */
    cwd: string;
    /** Hints for reading, from the text alone: delete, overwrite, network, outside, outside_home, privilege. Never a gate, and none is not safe. An open set: draw a word you do not know as text. */
    risks: string[];
    /** Absent until the approve started it; then running, done (exit 0), failed (another exit) or stopped (its window closed first), as CommandRun.state. An open set. */
    state?: string;
    /** The run's window, as terminal.attach takes it; set once started. */
    terminal?: string;
    started?: Timestamp;
    ended?: Timestamp;
    /** The exit status once done or failed. */
    exit?: number;
  };
};

/** What the proposal changes, by kind (hl propose). Every kind needs name. */
export type ProposalArgs = {
  /** The agent the proposal is about (the new agent's name for add; a team's manager for team). */
  name: string;
  team?: string;
  repo?: string;
  model?: string;
  effort?: string;
  vendor?: string;
  /** rename: the new name. */
  to?: string;
  /** team: each a suffix (`sea` is `name.sea`) or a full name. */
  members?: string[];
  /** team: repo by member. */
  repos?: { [key: string]: string };
  /** settings: agent setting keys as AgentSettingValues names them. context: whole new texts keyed `role`, `base` or `rules`; empty text or control characters other than tab and newline is bad_request. sandbox: `kind` (host, command, write or cert) and `value` (empty for cert), checked as agent.set checks an entry of sandbox_allow; one the agent has already is bad_request. */
  sets?: { [key: string]: string };
  /** bundle: the steps, at most 40, each judged against the roster the earlier ones leave. name is then ignored (send the count, `3 changes`). */
  changes?: { [key: string]: unknown }[];
  /** schedule: add a schedule to `name`, edit one of its (give was_cron for which; cron and message are then the new ones, a missing one stays), or remove one (was_cron). */
  action?: "add" | "edit" | "remove";
  /** schedule add, edit: five cron fields; it may not fire more than once an hour (bad_request). */
  cron?: string;
  /** schedule add, edit: the text typed at the agent each time, at most 1000 characters. */
  message?: string;
  /** schedule edit, remove: the cron of the existing schedule. */
  was_cron?: string;
  /** schedule edit, remove: its message, when two of the agent's schedules share the cron. */
  was_message?: string;
  /** run: the shell command the founder's approve runs in a terminal window, 1 to 4000 characters, no NUL. One string, run by the window's shell: a pipeline or `&&` is one proposal. */
  command?: string;
  /** run: the folder it runs in; it must be a directory when proposed. A relative one is taken from the agent's worktree; kept absolute. */
  cwd?: string;
};

export type Person = {
  /** p<n>. */
  key: string;
  /** What the app shows for the person. */
  display_name: string;
  /** The word other people address them by in team mode; null in personal mode. */
  handle: string | null;
  role: "owner" | "member";
  /** The owner removed them (person.remove): kept so their past words still name them, with no device and no login. */
  removed: boolean;
};

export type Workspace = {
  /** w<n>. */
  key: string;
  name: string;
  /** Its default folder, the first of folders; "" when it has none. */
  folder: string;
  /** Its other folders, in the order they were added. */
  added_folders: string[];
  /** The key of its lead agent; null before it has one. */
  lead: string | null;
  /** The key of the agent that leads it once its lead is retired: the oldest other agent in it that is not a seat (a temp lent from an agent), by the lowest key number compared as a number (a9 before a10). null when there is none, and the workspace goes with the lead. */
  next_lead: string | null;
  /** Every folder of the workspace, the default first. Each agent works in one of them (agents.list folder) and reads the others. */
  folders: WorkspaceFolder[];
  /** How its agents' sessions run unless an agent's own session_mode says: terminal or chat. */
  session_mode: "terminal" | "chat";
};

/** A kept rule in force for a workspace (docs/design/people-workspaces.md "Workspace rules"): its value, and whether the workspace or the machine default sets it. */
export type WorkspaceRule = {
  /** The rule's key. */
  key: "accounts" | "agent_cap" | "tools_deny" | "mode_max" | "review_required" | "land_by" | "land_check";
  /** accounts and tools_deny: a list of names; agent_cap: a whole number; mode_max: plan, default, acceptEdits or auto; review_required: true or false; land_by: an agent's name; land_check: a shell command. 0, an empty list or name and false are no rule. */
  value: unknown;
  /** Whether the value is a rule, not 0, empty or false. */
  on: boolean;
  /** workspace: set on it, beating the machine default; machine: the machine default, the value each workspace gets (not a total across the machine). */
  source: "workspace" | "machine";
  /** The key of the person who set it. */
  set_by: string;
  /** When, unix seconds. */
  set_at: number;
};

export type WorkspaceFolder = {
  /** The folder, an absolute path: a git checkout's top, or a plain folder. */
  path: string;
  /** The folder's own name, the last part of its path. */
  name: string;
  /** Whether it is a git checkout: an agent there works in a worktree of it, on its own branch; in a plain folder it works in the folder itself. */
  git: boolean;
  /** Whether it is the workspace's default folder, where a new agent works unless told another. */
  default: boolean;
};

export interface TeamMethods {
  "proposal.propose": { params: {
  kind: "add" | "retire" | "rename" | "team" | "settings" | "context" | "schedule" | "run" | "sandbox" | "bundle";
  args: ProposalArgs;
  why?: string;
  request_id: RequestId;
}; result: Accepted & {
    proposal: number;
  } };
  "proposal.withdraw": { params: {
  proposal: number;
  request_id: RequestId;
}; result: Accepted };
  "people.list": { params: { [key: string]: unknown }; result: {
    people: Person[];
  } };
  "person.invite": { params: {
  display_name: string;
  /** member when left out. */
  role?: "owner" | "member";
  request_id: RequestId;
}; result: {
    accepted: true;
    /** The new person's key. */
    person: string;
  } };
  "person.set_role": { params: {
  /** The person's key, p<n>. */
  person: string;
  role: "owner" | "member";
  request_id: RequestId;
}; result: Accepted };
  "person.remove": { params: {
  /** The person's key, p<n>. */
  person: string;
  request_id: RequestId;
}; result: Accepted };
  "workspaces.list": { params: { [key: string]: unknown }; result: {
    workspaces: Workspace[];
  } };
  "workspace.create": { params: {
  /** The workspace's name: lowercase letters, digits and -, 1 to 32 long, starting with a letter, used by no workspace and no person's handle. A dot or a slash is bad_request, field name. Left out or blank: the folder's name made into one (else "workspace"), with -2, -3 and so on when taken. */
  name?: string;
  /** The workspace's first folder, its default: an absolute path (~ is the home folder), a git checkout or a plain folder. Left out or blank: a new folder ~/Handline/<name> (-2, -3 and so on past one that is there and not empty). */
  folder?: string;
  /** An account to pin the lead to: it runs that account's vendor and never moves off it. Needed for a codex account, which the pick never places an unpinned agent on. Omitted: the lead is placed as any agent. An unknown account is not_found, what account; one not logged in is accepted, and the lead waits for its login as any agent pinned to a logged-out account does. */
  account?: string;
  /** The lead's role, its first version, written by the caller before the lead starts, so its first session reads it (agent.context.set's text). Empty or with a control character but tab and newline: bad_request, field role. Omitted: the lead starts with the role it is seeded with. */
  role?: string;
  /** The lead's standing context, its first version, kept as role is. Empty or with a control character but tab and newline: bad_request, field base. */
  base?: string;
  request_id: RequestId;
}; result: OperationStarted };
  "workspace.set": { params: {
  /** The workspace's key (w3) or its name, as workspaces.list gives them. An unknown one is not_found, what workspace. */
  workspace: string;
  /** A new name: lowercase letters, digits and -, 1 to 32 long, starting with a letter. */
  name?: string;
  /** A folder to add, an absolute path: a git checkout (kept as its top) or a plain folder. */
  add_folder?: string;
  /** A folder to take away, as workspaces.list gives its path. */
  remove_folder?: string;
  /** How its agents' sessions run unless an agent says. A running session that then runs the other way is moved between turns: closed at its prompt and resumed in the same conversation. The same mode is no change. */
  session_mode?: "terminal" | "chat";
  request_id: RequestId;
}; result: Accepted };
  "workspace.delete": { params: {
  /** The workspace's key (w3) or its name, as workspaces.list gives them. An unknown one is not_found, what workspace. */
  workspace: string;
  /** The founder's answer for every worktree of the workspace's agents that holds work, as agent.retire's save and force. keep, which would keep an agent, is bad_request, field worktree, as is any other word. */
  worktree?: "save" | "force";
  request_id: RequestId;
}; result: Accepted };
  "workspace.rules": { params: {
  /** The workspace's key (w3) or its name, as workspaces.list gives them; absent for the machine default. An unknown one is not_found, what workspace. */
  workspace?: string;
  /** An agent's name, for the workspace it is in, when workspace is absent; an agent in none is given the machine default. */
  agent?: string;
}; result: {
    rules: WorkspaceRule[];
    /** The size of the workspace's rules text in bytes, 0 when it has none. Only with a workspace. */
    text_bytes?: number;
    /** The most bytes the rules text should hold. Only with a workspace. */
    limit_bytes?: number;
  } };
  "workspace.rule.set": { params: {
  /** The workspace's key (w3) or its name, as workspaces.list gives them; absent for the machine default. An unknown one is not_found, what workspace. */
  workspace?: string;
  /** The rule's key. */
  key: "accounts" | "agent_cap" | "tools_deny" | "mode_max" | "review_required" | "land_by" | "land_check";
  /** The value as WorkspaceRule.value says, or null to clear it, so the level below holds. */
  value: unknown;
  request_id: RequestId;
}; result: Accepted };
  "team.get": { params: { [key: string]: unknown }; result: {
    agents: TeamAgent[];
    people: {
      key: string;
      /** The word the caller uses for this person: human for its owner or itself, else the handle or display name. */
      name: string;
    }[];
    /** The workspaces other than the caller's own. */
    workspaces: {
      key: string;
      name: string;
      /** Its lead agent's key, which receives what is sent to the workspace. */
      lead: string | null;
    }[];
  } };
  "proposal.list": { params: {
  /** Only the proposals in that state; without it, the list below. */
  state?: string;
}; result: {
    proposals: Proposal[];
  } & ReadCursor };
  "proposal.decide": { params: {
  proposal: number;
  decision: "approve" | "reject";
  note?: string;
  subject?: string;
  request_id: RequestId;
}; result: Accepted };
  "proposal.apply_now": { params: {
  proposal: number;
  request_id: RequestId;
}; result: Accepted };
  "proposal.stop": { params: {
  proposal: number;
  request_id: RequestId;
}; result: Accepted };
}

export const TEAM_METHODS: { [K in keyof TeamMethods]: { role: Role; callers: Role[] | null; write: boolean; since: number } } = {
  "proposal.propose": { role: "agent", callers: null, write: true, since: 1 },
  "proposal.withdraw": { role: "agent", callers: null, write: true, since: 1 },
  "people.list": { role: "agent", callers: null, write: false, since: 1 },
  "person.invite": { role: "owner", callers: null, write: true, since: 1 },
  "person.set_role": { role: "owner", callers: null, write: true, since: 1 },
  "person.remove": { role: "owner", callers: null, write: true, since: 1 },
  "workspaces.list": { role: "agent", callers: null, write: false, since: 1 },
  "workspace.create": { role: "owner", callers: null, write: true, since: 1 },
  "workspace.set": { role: "owner", callers: null, write: true, since: 1 },
  "workspace.delete": { role: "owner", callers: null, write: true, since: 1 },
  "workspace.rules": { role: "agent", callers: null, write: false, since: 1 },
  "workspace.rule.set": { role: "owner", callers: null, write: true, since: 1 },
  "team.get": { role: "agent", callers: null, write: false, since: 1 },
  "proposal.list": { role: "agent", callers: null, write: false, since: 1 },
  "proposal.decide": { role: "owner", callers: null, write: true, since: 1 },
  "proposal.apply_now": { role: "owner", callers: null, write: true, since: 1 },
  "proposal.stop": { role: "owner", callers: null, write: true, since: 1 },
};

/** x-app-cache of each read (x-app in core.json). */
export const TEAM_APP_CACHE: { [K in "people.list" | "workspaces.list" | "workspace.rules" | "team.get" | "proposal.list"]: { resource: string; id?: string; feed: Topic | null } | false } = {
  "people.list": {"resource": "people", "feed": "people"},
  "workspaces.list": {"resource": "workspaces", "feed": "workspaces"},
  "workspace.rules": false,
  "team.get": {"resource": "team", "feed": null},
  "proposal.list": {"resource": "proposals", "feed": "proposals"},
};

/** x-app-write of each write (x-app in core.json). */
export const TEAM_APP_WRITES: { [K in "proposal.propose" | "proposal.withdraw" | "person.invite" | "person.set_role" | "person.remove" | "workspace.create" | "workspace.set" | "workspace.delete" | "workspace.rule.set" | "proposal.decide" | "proposal.apply_now" | "proposal.stop"]: { kind: WriteKind; label: string } | false } = {
  "proposal.propose": false,
  "proposal.withdraw": {"kind": "decided", "label": "withdraw"},
  "person.invite": {"kind": "decided", "label": "invite"},
  "person.set_role": {"kind": "decided", "label": "change role"},
  "person.remove": {"kind": "decided", "label": "remove"},
  "workspace.create": {"kind": "decided", "label": "create workspace"},
  "workspace.set": {"kind": "decided", "label": "rename workspace"},
  "workspace.delete": {"kind": "decided", "label": "delete workspace"},
  "workspace.rule.set": {"kind": "decided", "label": "set workspace rule"},
  "proposal.decide": {"kind": "decided", "label": "decide"},
  "proposal.apply_now": {"kind": "decided", "label": "apply now"},
  "proposal.stop": {"kind": "decided", "label": "stop"},
};
