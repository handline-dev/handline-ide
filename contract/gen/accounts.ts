// Generated from contract/spec/ by contract/generate.py; do not edit.
/* eslint-disable */
import type { Accepted, ReadCursor, RequestId, Role, Timestamp, Topic, Who, WriteKind } from './core.ts';

export type Window = {
  used_pct: number;
  resets_at: Timestamp | null;
  /** Where the number comes from, e.g. measured or estimated. */
  source: string;
};

export type Account = {
  name: string;
  login_state: "ok" | "needs_login" | "unknown";
  five_hour: Window | null;
  weekly: Window | null;
  /** The agents on this account (AgentView.account): running on it, or at rest with their prompt cache alive. One past its cache life is on none. */
  agents: Who[];
  moves: {
    at: Timestamp;
    agent: Who;
    from: string | null;
    to: string;
    reason: string;
  }[];
  /** The name screens show; the account's own name until an alias is set. */
  label: string;
  /** The tool the account is for (`claude`, `opencode`, ...), as account.add set it. */
  vendor: string;
  /** The account's own cap: most agents mid-turn at once; null follows active_cap. */
  cap: number | null;
  /** The cap in force: cap, else the active_cap setting; with launch_pace_hold on, shrunk where the agents mid-turn would reach the week's block before it ends, and by the account's pace (Account.pace). */
  cap_effective: number;
  /** Whether a dead usage read may spend one real turn to fix itself (hl account recover). */
  recover: boolean;
  /** Paused by the founder (account.set paused): no agent starts on it (cap_effective 0), and its agents move off at their next turn when another account has room. Absent: not paused. */
  paused?: boolean;
  /** The Anthropic account's email, kept from its last login so it shows after the login drops; null when none was kept. Given to the founder's app only, never to an agent. */
  email?: string | null;
  cycle: {
    /** One-month subscription; off is renewing. */
    on: boolean;
    end: string | null;
    /** No end day is typed: end is null, and the account ends only when its login drops. */
    end_is_default: boolean;
  };
  /** Only with accounts.list `roster: true`. Agents on this account (Account.agents), running or at rest with their prompt cache alive (the figure `hl account list` prints). */
  agents_assigned?: number;
  /** Only with accounts.list `roster: true`. Agents mid-turn on this account: the count cap_effective holds to. */
  agents_running?: number;
  /** Only with accounts.list `roster: true`, and absent when the daemon runs no supervisor. Agents on this account at their prompt whose session still runs background work; never in agents_running. */
  agents_background?: number;
  /** Only with accounts.list `roster: true`. Agents whose config cannot be read and so count on no account; the same on every account, and every figure above is a floor while it is above 0. */
  agents_unplaced?: number;
  /** The login in progress or the last one to end; null when none was started. Reported on the accounts topic as account.changed. Never carries the authorize URL: only the founder's account.login reply has it. */
  login?: {
    login_id: string;
    state: "waiting" | "code_sent" | "done" | "failed" | "cancelled";
    /** Why the login ended failed, one plain line (e.g. logged in to an Anthropic account other than the one recorded for this account); absent unless it failed. */
    reason?: string;
  } | null;
  /** Only with accounts.list `usage: true`. The plan claude reports (`max`, `pro`), null when not recorded. */
  plan?: string | null;
  /** Only with accounts.list `usage: true`. The shortest and, when the vendor has one, the longest window: the figure read, the one that may be acted on, and where it came from. */
  windows?: {
    window: string;
    minutes: number;
    /** The figure as read. */
    pct: number | null;
    /** The figure the daemon acts on. */
    effective: number | null;
    /** Where the figure comes from, or why it is not trusted. */
    source: string;
    /** Unix seconds the window resets. */
    reset: number | null;
  }[];
  /** Only with accounts.list `usage: true`. Context used by the account's last session, percent. */
  ctx_pct?: number | null;
  /** Only with accounts.list `usage: true`. Seconds since the usage was read. */
  age_s?: number | null;
  /** Only with accounts.list `usage: true`. How long the account lasts at its recent burn, in words (`~10m to wall`), as `hl account list` prints it. */
  runway?: string;
  /** Only with accounts.list `usage: true`. Whether one more agent may start on it. */
  assignable?: boolean;
  /** Only with accounts.list `usage: true`. Why assignable is what it is. */
  assign_reason?: string;
  /** Only with accounts.list `usage: true`. Why the daemon's pick for its vendor would not choose it now; null while it is a candidate. When the pick chooses none, a start may still fall back to a refused account. assign_reason says whether one more agent may start on it; not_picked says why the pick passes it over. A client shows not_picked. */
  not_picked?: {
    /** What keeps new work off it: its subscription end day has passed, the founder paused it, its weekly or 5-hour window is at the wall, it is not logged in (which wins over the rest), the agent cap or the launch pace hold, or something else (a stale, lost or unreadable reading). */
    kind: "subscription_ended" | "paused" | "week_cap" | "five_hour_cap" | "login" | "held" | "other";
    /** The daemon's reason in words, as `hl status` prints it. An ended account that is logged in with a fresh reading adds that its end day may be out of date. */
    text: string;
  } | null;
  /** Set while the account is STALLED: a turn on it ended on an error that says the account itself cannot be used (a refused login, an org that disallows it, a hold or billing problem), or on one error the daemon does not name four times with no good turn between. No agent is placed on it; account.wake, a login or a good turn there ends it. null while it is usable. Absent from a daemon that does not send it. */
  stalled?: {
    /** The line a row shows beside STALLED, as an agent's stall does: `login expired` for authentication_failed, else the error itself. */
    reason: string;
    /** The StopFailure error that stalled it, as the CLI gave it. */
    error: string;
    /** The turn that showed it, in words. */
    detail: string;
    since: Timestamp;
  } | null;
  /** Only with accounts.list `usage: true`. The account's weekly pace (settings launch_pace_hold, pace_target_pct, pace_burst_pct), judged on its last weekly reading, which is kept until its weekly reset even when it is stale. null when it has no weekly reading or its reset is not known; a null pace does not shrink the cap. back_at: for state over, when the line will have caught up to within pace_burst_pct of used_pct at no further use, or the end of its week when the line never gets there; else null. */
  pace?: {
    /** behind: used_pct is at or under line_pct, and the cap is not shrunk for pace. ahead: over the line by at most pace_burst_pct points; the pace cap is ceil(C x (line_pct + pace_burst_pct - used_pct) / pace_burst_pct), at least 1, where C is cap, else the active_cap setting. over: further ahead; the pace cap is 0 and the account starts no new turn. off: launch_pace_hold is off; the figures are still given. cap_effective is the smallest of C, the wall-cutoff cap and the pace cap. */
    state: "behind" | "ahead" | "over" | "off";
    /** Where the line is now: pace_target_pct x (now - start) / (end - start), where start is its weekly reset less 7 days and end is that reset or the subscription end, whichever comes first. */
    line_pct: number;
    /** The weekly window's used percent the pace is judged on: the last weekly reading, kept until its reset. */
    used_pct: number;
    /** Points of the week per hour the account may use from now to reach pace_target_pct at the end of its week: (pace_target_pct - used_pct) / hours left; 0 at or past the target. */
    rate_pct_h: number;
    back_at: Timestamp | null;
  } | null;
  /** Only with accounts.list `usage: true`. What a point of each window costs on this account in API-equivalent dollars, learned from stretches of live readings over which the figure rose 5 points or more: the account's priced token calls in the stretch over the rise. Recomputed every 10 minutes from the last 7 days. null before the daemon has read the account. */
  factors?: {
    /** Dollars per point of the 5-hour window. */
    short: {
      /** API-equivalent dollars per point of the window; 0 when source is none. */
      usd_per_pct: number;
      /** own: the median of the account's own recent stretches. pooled: the median of every account's, in Pro units, times plan_scale. none: no account has a stretch yet. */
      source: "own" | "pooled" | "none";
      /** The account's own stretches; it switches to own at 5. */
      samples: number;
    };
    /** Dollars per point of the week. */
    long: {
      /** API-equivalent dollars per point of the window; 0 when source is none. */
      usd_per_pct: number;
      /** own: the median of the account's own recent stretches. pooled: the median of every account's, in Pro units, times plan_scale. none: no account has a stretch yet. */
      source: "own" | "pooled" | "none";
      /** The account's own stretches; it switches to own at 5. */
      samples: number;
    };
    /** The plan's size relative to Pro (Pro 1, Max 5x 5, Max 20x 20), from the recorded subscription type and rate-limit tier. */
    plan_scale: number;
  } | null;
};

export type Stats = {
  window: "24h" | "7d";
  since: Timestamp;
  until: Timestamp;
  tasks_done: number;
  /** Each agent's use of each account in the window, largest cost first; an agent on two accounts has two rows. */
  use: AgentAccountUse[];
  moves: number;
  handoffs: number;
  blocks: number;
  per_account: {
    account: string;
    five_hour_used_pct: number | null;
    weekly_used_pct: number | null;
  }[];
};

/** One agent's use of one account. tokens and cost are measured; week_pct is an estimate of the split, not of the account's rise. */
export type AgentAccountUse = {
  /** Empty for the calls of no agent's worktree (the founder's own sessions, the usage poll); drawn as Other. */
  agent: string;
  account: string;
  /** Input, cache and output tokens. */
  tokens: number;
  /** List price, USD. */
  cost: number;
  /** This row's cost over the account's cost in the window, 0 to 1. */
  share: number;
  /** null when the account has no reading in the window or its calls had no list price. */
  week_pct: number | null;
};

export type Outlook = {
  range: "12h" | "24h" | "3d" | "7d" | "14d" | "30d";
  now: Timestamp;
  /** How many accounts have a reading. */
  accounts: number;
  /** What usage samples recorded, oldest first. */
  past: {
    at: Timestamp;
    left_pct: number | null;
  }[];
  /** A guess. With launch_pace_hold on, each account's week goes at the smaller of its pace (Account.pace rate_pct_h, which ends at pace_target_pct) and its use over the last day; with pacing off, at its use over the last day. A week that resets starts again from nothing used. An account whose last reading is stale or at a wall keeps that reading until its reset and is not left out. An account that cannot be used now (its login gone or expired) counts as 0 ahead. Kept apart from `past`, never joined to it. */
  future: {
    at: Timestamp;
    left_pct: number | null;
  }[];
  runway: {
    account: string;
    runs_out_in_seconds: number | null;
    week_resets_at: Timestamp | null;
  }[];
  cost_per_day: DayCosts;
  /** What each agent used over the last `range` (the span of `past`), largest cost first, ties by name. A client groups agents by the first part of the dotted name; a group's numbers are the sums of its members' (est_pct null if any member's is). Absent from a daemon that does not send it. */
  agent_use?: {
    agent: string;
    /** Tokens the agent used in the span. */
    tokens: number;
    /** Its cost in US dollars at list price in the span, from the token log. */
    cost: number;
    /** Its cost as a fraction (0 to 1) of everyone's cost in the span; 0 when nothing was spent. */
    share: number;
    /** An estimate of the points of the combined week the agent used in the span: its cost share of each account times the points that account used, averaged over the accounts. An estimate, because what the plan charges is not what the price list says. Null when no account has a reading. */
    est_pct: number | null;
    /** Its cost share of each account in the span, by account name (a fraction, 0 to 1). An account on which nothing at all was spent is absent; one on which others spent but none of this agent's tokens was priced is 0. */
    fracs: { [key: string]: number };
  }[];
  /** Only with `agent`: the points of the combined week the pick had used by each time of `past`, one number per `past` entry in the same order: its cost share of each account times what that account had used so far, averaged over the accounts. Empty when the pick used nothing in the span or is not an agent or group. Absent from a daemon that does not send it. */
  overlay?: number[];
  /** Each account that has a reading in the span, by name: its windows and its pace line, for a chart per account. Absent from a daemon that does not send it. */
  by_account?: {
    account: string;
    /** What usage samples recorded, at the same points as `past`. */
    past: {
      at: Timestamp;
      five_left: number | null;
      week_left: number | null;
      line_left: number | null;
    }[];
    /** The guess of `future` for this account, at the same points. The 5h window starts again from nothing at each reset, so its line is a saw; it goes at its use over the last day, and with launch_pace_hold on at no more than the paced week rate converted to the 5h window by the account's factors (Account.factors: its 5-hour factor over its week factor). */
    future: {
      at: Timestamp;
      five_left: number | null;
      week_left: number | null;
      line_left: number | null;
    }[];
    /** Resets in the span, oldest first: those samples recorded, then those guessed. */
    resets: {
      at: Timestamp;
      window: "5h" | "7d";
      /** Only on a weekly (7d) reset the samples recorded: the percent of that week left unused at the last sample before it. Absent for a 5h reset, a guessed one, or a week with no sample. */
      unused_pct?: number;
    }[];
  }[];
};

/** The cost of one founder's task: its wall time, and the tokens spent inside the doing windows of every task in its subtree, per agent. */
export type Retro = {
  task: number;
  /** From the task's creation to its done or discard time; to now while it is open. */
  wall_seconds: number;
  /** All agents together: inp, cc, cr and out (thinking is a part of out). */
  tokens: number;
  /** USD at list price, all agents together. */
  cost: number;
  /** One row per agent that was doing a task of the subtree, most tokens first, ties by agent. */
  agents: {
    agent: string;
    /** Time it held tasks of the subtree in doing, waits on a blocker included. */
    doing_seconds: number;
    tokens: number;
    cost: number;
    /** The subtree tasks it did, by id. */
    tasks: number[];
  }[];
};

/** What the agents cost per local calendar day, oldest first; a day with no calls is absent. */
export type DayCosts = {
  /** Local calendar day, YYYY-MM-DD. */
  day: string;
  /** List-price cost in US dollars of each agent that day. */
  agents: { [key: string]: number };
}[];

export type CostPerDay = {
  /** The range asked for. */
  range: "12h" | "24h" | "3d" | "7d" | "14d" | "30d";
  cost_per_day: DayCosts;
};

export interface AccountsMethods {
  "accounts.list": { params: {
  /** Add each account's agents_assigned, agents_running, agents_background and agents_unplaced. Default false. */
  roster?: boolean;
  /** Add each account's plan, windows, ctx_pct, age_s, runway, assignable and assign_reason (what `hl account list` prints beside the windows), pace and not_picked. Default false. */
  usage?: boolean;
}; result: {
    accounts: Account[];
  } & ReadCursor };
  "status.pick": { params: { [key: string]: unknown }; result: {
    /** The account to place the next agent on; null when none may take work (fail-closed). */
    picked: string | null;
    candidates: {
      account: string;
      reason: string;
    }[];
    /** One line per agent kind: its price per minute of the 5h window and what the price rests on. */
    agent_costs: string[];
    /** What one agent spends per minute of work, by model, pooled across accounts, then the whole pool. */
    agent_pace_usd?: {
      /** The model, or "all agents" for the whole pool. */
      model: string;
      /** Median API-equivalent dollars per minute of work of its latest samples. */
      usd_per_min: number;
      /** What the figure rests on: the model, "all agents" or "starting value". */
      source: string;
      /** Samples behind it; 0 for the starting value. */
      samples: number;
    }[];
  } };
  "account.set": { params: {
  account: string;
  /** Alias, one plain line; empty clears. */
  label?: string;
  cap?: number;
  recover?: boolean;
  /** Pause or resume the account. */
  paused?: boolean;
  /** The one-month subscription switch (hl account cycle on|off); off keeps the end day. */
  cycle?: boolean;
  /** Last usable day YYYY-MM-DD; a date alone turns the cycle on, as `hl account cycle --end` does; null clears it. */
  cycle_end?: string | null;
  request_id: RequestId;
}; result: Accepted };
  "account.wake": { params: {
  account: string;
  request_id: RequestId;
}; result: Accepted };
  "account.read_usage": { params: {
  account: string;
  request_id: RequestId;
}; result: Accepted };
  "account.add": { params: {
  /** The new account's name: one plain line, no slash, not starting with a dot. Omitted: account<n>, n the number of accounts plus one, skipping a name in use. */
  account?: string;
  /** The tool the account is for: claude, codex, or another the daemon has a driver for; omitted means claude. */
  vendor?: string;
  request_id: RequestId;
}; result: Accepted & {
    /** The name as registered. */
    account: string;
  } };
  "account.login": { params: {
  account: string;
  request_id: RequestId;
}; result: Accepted & {
    login_id: string;
    state: "waiting";
    /** Claude: the authorize URL the CLI printed; the person opens it and brings the code back with account.login_code. Codex: the device page; the person opens it and types device_code there, and nothing comes back. */
    url: string;
    /** Codex only: the one-time code the CLI printed, for the person to type at url. Absent for claude. Kept in memory for the login only: never in an event, a log, the login row or the kept reply a retry gets. */
    device_code?: string;
  } };
  "account.login_cancel": { params: {
  login_id: string;
  request_id: RequestId;
}; result: Accepted };
  "account.login_code": { params: {
  login_id: string;
  /** One-time; written to the CLI's stdin and nowhere else. */
  code: string;
  request_id: RequestId;
}; result: Accepted };
  "accounts.discover": { params: { [key: string]: unknown }; result: {
    /** One row per folder that exists, claude first; empty when there is none. */
    found: {
      /** claude or codex. */
      vendor: string;
      /** The folder the login lives in, as ~/.codex; what account.adopt takes. */
      source: string;
      /** Whether the vendor's own check says it is logged in; null when it cannot be told. */
      logged_in: boolean | null;
      /** Whether account.adopt takes it: true for codex. A Claude login is keyed by its config folder, so a client shows it as found and the account logs in again. */
      adoptable: boolean;
    }[];
  } };
  "account.adopt": { params: {
  /** codex. Any other is bad_request, field vendor: a Claude login cannot be shared, so the account is added and logs in. */
  vendor: string;
  /** A source accounts.discover answered for that vendor. Another is not_found, what source. */
  source: string;
  /** The new account's name, as account.add takes it; omitted: account<n>. */
  account?: string;
  request_id: RequestId;
}; result: Accepted & {
    /** The name as registered. */
    account: string;
  } };
  "account.remove": { params: {
  account: string;
  /** Change nothing: answer `plan` and `would_remove`. A dry run needs no `confirm`. */
  dry_run?: boolean;
  /** The account's name, as the founder typed it; required unless dry_run. */
  confirm?: string;
  request_id: RequestId;
}; result: Accepted & {
    /** Dry run only: whether the removal would go ahead. */
    would_remove?: boolean;
    /** Dry run only: what it would do, line by line, or the one line that says why it would refuse. */
    plan?: string[];
  } };
  "stats.get": { params: {
  window: "24h" | "7d";
}; result: Stats };
  "stats.outlook": { params: {
  /** Headroom: that long back and as long ahead. Cost per day: the last that many days (under 1 day counts as 1). */
  range: "12h" | "24h" | "3d" | "7d" | "14d" | "30d";
  /** Also give `overlay` for this pick: an agent's name, or `grp:<name>` for the group of agents whose dotted names start with `<name>.` (and the agent `<name>` itself), the key of a row of the agents table a client draws. It rides on this call and not on a method of its own because the curve is read against `past` and must be sampled at the same moment as it; one pick costs one array, not one per agent. */
  agent?: Who;
}; result: Outlook };
  "stats.cost": { params: {
  /** The last that many days (under 1 day counts as 1), as stats.outlook counts them. */
  range: "12h" | "24h" | "3d" | "7d" | "14d" | "30d";
}; result: CostPerDay };
  "stats.retro": { params: {
  task: number;
}; result: Retro };
}

export const ACCOUNTS_METHODS: { [K in keyof AccountsMethods]: { role: Role; callers: Role[] | null; write: boolean; since: number } } = {
  "accounts.list": { role: "agent", callers: null, write: false, since: 1 },
  "status.pick": { role: "agent", callers: null, write: false, since: 1 },
  "account.set": { role: "owner", callers: null, write: true, since: 1 },
  "account.wake": { role: "owner", callers: null, write: true, since: 1 },
  "account.read_usage": { role: "owner", callers: null, write: true, since: 1 },
  "account.add": { role: "owner", callers: null, write: true, since: 1 },
  "account.login": { role: "owner", callers: null, write: true, since: 1 },
  "account.login_cancel": { role: "owner", callers: null, write: true, since: 1 },
  "account.login_code": { role: "owner", callers: null, write: true, since: 1 },
  "accounts.discover": { role: "owner", callers: null, write: false, since: 1 },
  "account.adopt": { role: "owner", callers: null, write: true, since: 1 },
  "account.remove": { role: "owner", callers: null, write: true, since: 1 },
  "stats.get": { role: "member", callers: null, write: false, since: 1 },
  "stats.outlook": { role: "member", callers: null, write: false, since: 1 },
  "stats.cost": { role: "member", callers: null, write: false, since: 1 },
  "stats.retro": { role: "member", callers: null, write: false, since: 1 },
};

/** x-app-cache of each read (x-app in core.json). */
export const ACCOUNTS_APP_CACHE: { [K in "accounts.list" | "status.pick" | "accounts.discover" | "stats.get" | "stats.outlook" | "stats.cost" | "stats.retro"]: { resource: string; id?: string; feed: Topic | null } | false } = {
  "accounts.list": {"resource": "accounts", "feed": "accounts"},
  "status.pick": false,
  "accounts.discover": false,
  "stats.get": {"resource": "stats", "feed": null},
  "stats.outlook": {"resource": "outlook", "feed": null},
  "stats.cost": {"resource": "cost", "feed": null},
  "stats.retro": {"resource": "retro", "feed": null},
};

/** x-app-write of each write (x-app in core.json). */
export const ACCOUNTS_APP_WRITES: { [K in "account.set" | "account.wake" | "account.read_usage" | "account.add" | "account.login" | "account.login_cancel" | "account.login_code" | "account.adopt" | "account.remove"]: { kind: WriteKind; label: string } | false } = {
  "account.set": {"kind": "authored", "label": "save account"},
  "account.wake": {"kind": "authored", "label": "wake"},
  "account.read_usage": {"kind": "decided", "label": "read usage"},
  "account.add": {"kind": "decided", "label": "add account"},
  "account.login": {"kind": "decided", "label": "log in"},
  "account.login_cancel": {"kind": "decided", "label": "cancel login"},
  "account.login_code": {"kind": "decided", "label": "send code"},
  "account.adopt": {"kind": "decided", "label": "use this login"},
  "account.remove": {"kind": "decided", "label": "remove"},
};
