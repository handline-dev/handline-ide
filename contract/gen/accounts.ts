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
  /** Whether a dead usage read may spend one real turn to fix itself (hl account recover). */
  recover: boolean;
  /** Paused by the founder (account.set paused): no agent starts on it, and its agents move off at their next turn when another account has room. Absent: not paused. */
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
  /** Only with accounts.list `roster: true`. Agents mid-turn on this account. */
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
  /** Only with accounts.list `usage: true`. The plan as Anthropic names it (`Ultra`, `Max`, `Pro`), from the subscription and rate-limit tier claude reports; null when not recorded. */
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
    /** What keeps new work off it: its subscription end day has passed, the founder paused it, its weekly or 5-hour window is at the wall, it is not logged in (which wins over the rest), it may take work but another account fits a new start better, or something else (a stale, lost or unreadable reading). */
    kind: "subscription_ended" | "paused" | "week_cap" | "five_hour_cap" | "login" | "unfit" | "other";
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
  /** Only with accounts.list `usage: true`. The account's weekly pace (settings pace_target_pct, pace_burst_pct), shown only: it holds no start. Judged on its last weekly reading, which is kept until its weekly reset even when it is stale. null when it has no weekly reading or its reset is not known. */
  pace?: {
    /** behind: used_pct is at or under line_pct. ahead: over the line by at most pace_burst_pct points. over: further ahead. The pace holds no start. */
    state: "behind" | "ahead" | "over";
    /** Where the line is now: pace_target_pct x (now - start) / (end - start), where start is its weekly reset less 7 days and end is that reset or the subscription end, whichever comes first. */
    line_pct: number;
    /** The weekly window's used percent the pace is judged on: the last weekly reading, kept until its reset. */
    used_pct: number;
    /** Points of the week per hour the account may use from now to reach pace_target_pct at the end of its week: (pace_target_pct - used_pct) / hours left; 0 at or past the target. */
    rate_pct_h: number;
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
  /** The median seconds from a turn put on a person to that person giving it back, over turns given back in the window; null when none was. */
  your_wait_s: number | null;
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
  /** A guess. Each account's week goes at its pace since that week began. A week that resets starts again from nothing used. An account whose last reading is stale or at a wall keeps that reading until its reset and is not left out. An account that cannot be used now (its login gone or expired) counts as 0 ahead. Kept apart from `past`, never joined to it. */
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
  /** What each agent used over the last `range` (the span of `past`), largest cost first, ties by name; plan money of a week's rise with no priced call comes last as a row of (unattributed) with no tokens. Absent from a daemon that does not send it. */
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
    /** Its plan money in the span, US dollars: each account's week rise in the span times that account's weekly budget, split by its cost share of that account (`fracs`). Token prices only split it; they never size it. */
    plan_cost?: number;
  }[];
  /** Only with `agent`: the points of the combined week the pick had used by each time of `past`, one number per `past` entry in the same order: its cost share of each account times what that account had used so far, averaged over the accounts as `past` is. Empty when the pick used nothing in the span or is not an agent or group. Absent from a daemon that does not send it. */
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
    /** The guess of `future` for this account, at the same points: the week at its pace since it began, the 5h window at its use over the last day, starting again from nothing at each reset, so its line is a saw. */
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
    /** Whether its readings carry a 5-hour figure. One with none (a verified Ultra has no 5-hour limit) has five_left null everywhere, no 5h resets and no five_runs_out_at. */
    five_hour: boolean;
    /** What its week is guessed to leave unused at its next reset (the one after, when the reset passed since its last reading); null when no reset is known, its last reading is older than two hours, it cannot be used now, or it ends before it. */
    forecast: {
      /** Its next weekly reset. */
      at: Timestamp;
      /** Percent of the week guessed left unused at `at`, going at its pace since the week began. */
      unused_pct: number;
      /** That share of its weekly plan budget (budget.accounts[].week_usd), US dollars; null for an account with no budget here (unmetered, or no longer listed). */
      unused_usd: number | null;
    } | null;
    /** When its 5-hour window runs out at its use over the last day, counted on from its last reading; null when it lasts to its reset, is not in use, its reading is stale, or it has no 5-hour window. */
    five_runs_out_at: Timestamp | null;
  }[];
  plan_cost_per_day?: PlanDayCosts;
  /** The week in plan money: each account's monthly fee turned into a weekly budget, of which its week's percent used is spent. Absent from a daemon that does not send it. */
  budget?: {
    /** Each metered account, by name. */
    accounts: {
      account: string;
      /** The plan as Anthropic names it (Pro, Max, Ultra); null when none is recorded. */
      plan: string | null;
      /** Its monthly fee in US dollars: the one set on the account (account.set plan_fee), else its plan's listed one (Pro 20, Max 100, Ultra 200); 0 for a plan with none. */
      fee_usd: number;
      /** Its weekly budget: fee_usd x 12 / 52. */
      week_usd: number;
      /** Percent of its week used now; null with no reading. A week whose reset has passed is 0. */
      used_pct: number | null;
      /** used_pct of week_usd; null with no reading. */
      spent_usd: number | null;
      /** week_usd less spent_usd; null with no reading. */
      left_usd: number | null;
      /** Whether its reading carries a 5-hour figure: an Ultra account has no 5-hour limit, so none. */
      five_hour: boolean;
      /** Its extra usage; null when its last reading named none (unknown, never zero). */
      extra: ExtraUsage | null;
    }[];
    /** The weekly budgets together, of the accounts with a fee and a reading. */
    week_usd: number;
    /** What those accounts spent of them this week. */
    spent_usd: number;
    /** week_usd less spent_usd. */
    left_usd: number;
    /** Percent of week_usd left: the accounts' weeks weighted by their budgets, not averaged. null when no account counts. */
    left_pct: number | null;
    /** Extra usage spent this billing month by the accounts whose reading names an amount, per currency (USD: dollars). Never part of spent_usd. */
    extra_spent: { [key: string]: number };
  };
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

/** The plan money the agents used per local calendar day of the range, oldest first; a day nothing rose and no extra usage was spent is absent. Absent from a daemon that does not send it. */
export type PlanDayCosts = {
  /** Local calendar day, YYYY-MM-DD. */
  day: string;
  /** Plan money in US dollars of each agent that day: each account's week rises seen that day (dated by the reading that shows them) times its weekly budget, split by the agents' list-price cost on that account that day; money of a rise with no priced call is under (unattributed). */
  agents: { [key: string]: number };
  /** Extra usage spent that day, per currency: the rise of each account's spent between its readings, dated by the later one; a drop is its billing month starting over, counted from 0. */
  extra: { [key: string]: number };
}[];

/** An account's extra usage as its last reading named it: money paid on top of the plan in its billing month. It never enters the plan money. */
export type ExtraUsage = {
  /** Whether extra usage is turned on: the account may keep working past its limits, paid on top of its plan. */
  on: boolean;
  /** Its monthly cap, in major units of currency; null when none is named. */
  cap: number | null;
  /** What it spent this billing month, in major units of currency; null when none is named. */
  spent: number | null;
  /** The currency the amounts are in (USD); null when none is named. */
  currency: string | null;
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
  "account.set": { params: {
  account: string;
  /** Alias, one plain line; empty clears. */
  label?: string;
  recover?: boolean;
  /** Pause or resume the account. */
  paused?: boolean;
  /** The one-month subscription switch (hl account cycle on|off); off keeps the end day. */
  cycle?: boolean;
  /** Last usable day YYYY-MM-DD; a date alone turns the cycle on, as `hl account cycle --end` does; null clears it. */
  cycle_end?: string | null;
  /** The account's monthly fee in US dollars when it is not its plan's listed one; null goes back to the plan's. */
  plan_fee?: number | null;
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
  /** Also give `overlay` for this pick: an agent, the key of a row of the agents table a client draws. It rides on this call and not on a method of its own because the curve is read against `past` and must be sampled at the same moment as it; one pick costs one array, not one per agent. */
  agent?: Who;
}; result: Outlook };
  "stats.retro": { params: {
  task: number;
}; result: Retro };
}

export const ACCOUNTS_METHODS: { [K in keyof AccountsMethods]: { role: Role; callers: Role[] | null; write: boolean; since: number } } = {
  "accounts.list": { role: "agent", callers: null, write: false, since: 1 },
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
  "stats.retro": { role: "member", callers: null, write: false, since: 1 },
};

/** x-app-cache of each read (x-app in core.json). */
export const ACCOUNTS_APP_CACHE: { [K in "accounts.list" | "accounts.discover" | "stats.get" | "stats.outlook" | "stats.retro"]: { resource: string; id?: string; feed: Topic | null } | false } = {
  "accounts.list": {"resource": "accounts", "feed": "accounts"},
  "accounts.discover": false,
  "stats.get": {"resource": "stats", "feed": null},
  "stats.outlook": {"resource": "outlook", "feed": null},
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
