import { PROTOCOL } from '@handline/contract';
import type { AgentDetail, AgentView, InputHolder, MachineNeed, Methods, Notifications, Prompt, Row, Timestamp } from '@handline/contract';
import { BANNER, type AgentFixture } from '#src/fixtures.ts';
import { Failure, badRequest, refused } from '#src/errors.ts';
import { defaultSettings } from '#src/settings.ts';

export const ROWS = 24,
  COLS = 80;
/** A card's title by its kind, as the daemon gives one from a hook or the session file. */
const NEEDS_TITLE = { permission: 'Waiting for your OK', question: 'Asked you a question', dialog: 'Waiting at a dialog' } as const;
export const DAEMON_HOLDER: InputHolder = { kind: 'daemon', name: 'daemon', since: null };
export const row = (text: string): Row => ({ text });

/** A frame as a styled watcher gets it: the same rows with their colour, marked with the attach's format. */
export function styledFrame(n: Notifications['frame'], cols: number): Notifications['frame'] {
  return {
    ...n,
    format: 'styled',
    lines: n.lines?.map((r) => styledRow(r.text, cols)),
    changed: n.changed?.map((c) => ({ ...c, row: styledRow(c.row.text, cols) })),
  };
}

/** A row with colour, the way `coded` does it for bytes: the prompt marker cyan, the agent's bullet green. */
export function styledRow(text: string, cols = Infinity): Row {
  text = [...text].slice(0, cols).join('');
  if (text.startsWith('> ')) return { text, spans: [{ from: 0, to: 1, style: { fg: 6 } }] };
  if (text.startsWith('●')) return { text, spans: [{ from: 0, to: 1, style: { fg: 2, bold: true } }] };
  return { text };
}

export interface Session {
  fx: AgentFixture;
  state: string;
  since: Timestamp;
  sessionId: string | null;
  open: boolean;
  hasTerminal: boolean;
  lines: string[];
  seq: number;
  cols: number;
  rows: number;
  buffer: string; // typed, Enter not yet pressed
  composing: boolean; // the last line shows the buffer
  holder: InputHolder;
  unheld?: [number, number]; // the size before the viewers' first resize while nobody held the input
  prompt: Prompt | null;
  structured?: boolean; // played as a structured session by the transcript
  queue: (() => void)[]; // the scripted turn, one step per tick
  counter: number;
  settings: Record<string, string>; // agent.set's values
  strip?: Partial<Methods['agent.strip']['result']>; // what a test sets over the numbers agent.strip answers
  handle?: string; // a shell shell.open started: its terminal handle, with no agent session behind it
  received: { kind: 'text' | 'key'; value: string; actor: string }[]; // what reached the terminal
}

/** How one connection watches a terminal: the format it asked for, and its own notice counter when that is bytes. */
export interface Attach {
  format: 'text' | 'bytes' | 'styled';
  seq: number;
}

export const tid = (s: Session) => s.handle ?? `term:${s.fx.name}`;

/** The session whose terminal is `id`; not_found when none is. */
export function byTerminal(sessions: Map<string, Session>, id: unknown): Session {
  const s = [...sessions.values()].find((x) => tid(x) === id);
  if (!s) throw new Failure('not_found', `no terminal ${String(id)}`);
  return s;
}

export function newSession(fx: AgentFixture, since: Timestamp): Session {
  const open = fx.session !== null;
  return {
    fx,
    state: fx.state,
    since,
    sessionId: fx.session,
    open,
    hasTerminal: fx.state !== 'no_terminal',
    lines: open ? BANNER.map((l) => l.replace('{worktree}', fx.worktree)) : [],
    seq: 0,
    cols: COLS,
    rows: ROWS,
    buffer: '',
    composing: false,
    holder: DAEMON_HOLDER,
    prompt: null,
    queue: [],
    counter: 0,
    settings: defaultSettings(fx.model, fx.effort),
    received: [],
  };
}

/** The screen as `rows` lines, padded with blanks. */
export function visible(s: Session): string[] {
  const tail = s.lines.slice(-s.rows);
  return [...tail, ...Array(s.rows - tail.length).fill('')];
}
export const scrolledOff = (s: Session): number => Math.max(0, s.lines.length - s.rows);

export function view(s: Session): AgentView {
  return {
    name: s.fx.name,
    role: s.fx.role,
    model: s.fx.model,
    effort: s.fx.effort,
    state: s.state,
    state_since: s.since,
    state_detail: '',
    block: s.fx.wall ? { kind: 'wall', since: s.since } : null,
    wall_reason: s.fx.wall ? 'usage limit' : null,
    account: s.fx.account,
    task: s.fx.task,
    cache_until: s.open && s.state === 'idle' ? new Date(Date.parse(s.since) + 5 * 60_000).toISOString() : null,
    has_dialog: s.prompt !== null,
    needs_you: s.prompt
      ? {
          kind: s.prompt.kind === 'permission' || s.prompt.kind === 'question' ? s.prompt.kind : 'dialog',
          since: s.prompt.since,
          title: NEEDS_TITLE[s.prompt.kind === 'permission' || s.prompt.kind === 'question' ? s.prompt.kind : 'dialog'],
          text: s.prompt.reason ?? s.prompt.command ?? null,
          source: 'hook',
        }
      : null,
    parent: s.fx.parent,
    background: [],
    ...(s.structured && { structured: true }),
    ...s.fx.view,
  };
}

export function detail(s: Session): AgentDetail {
  return {
    ...view(s),
    worktree: { path: s.fx.worktree, branch: s.fx.branch, repo: s.fx.repo ?? null },
    cwd: s.fx.worktree,
    can: Object.fromEntries(
      ['terminal', 'wake', 'close', 'restart', 'park', 'fold', 'move'].map((k) => [k, { ok: true, why: null }]),
    ) as AgentDetail['can'],
    terminal: s.hasTerminal ? tid(s) : null,
    session_id: s.sessionId,
    session_open: s.open,
    input_holders: s.structured ? [] : [s.holder],
    account_pick: { account: s.fx.account, reason: 'fixture' },
    prompt: s.prompt,
    settings: s.settings as AgentDetail['settings'],
  };
}

/** accounts.list for a daemon given account names (each with the agents whose fixture names it); refused as unsupported without them. */
export function accountsReply(names: string[] | null, sessions: Session[]) {
  if (!names) throw refused('unsupported', 'the fake daemon does not serve accounts.list');
  const accounts = names.map((name) => ({
    name,
    label: name,
    vendor: 'claude',
    login_state: 'ok' as const,
    five_hour: null,
    weekly: null,
    agents: sessions.filter((s) => s.fx.account === name).map((s) => s.fx.name),
    moves: [],
    cap: null,
    cap_effective: 3,
    recover: false,
  }));
  return { accounts };
}

export function statusReply(
  sessions: Map<string, Session>,
  unread: number,
  startedAt: Timestamp,
  needs: readonly MachineNeed[] = [],
  keepAwake = 'off',
  paused: object | null = null,
) {
  const by_state: Record<string, number> = {};
  for (const s of sessions.values()) by_state[s.state] = (by_state[s.state] ?? 0) + 1;
  // the sleep lock is held under on, and under working while an agent runs; the fake has held it since it started
  const held = keepAwake === 'on' || (keepAwake === 'working' && (by_state.running ?? 0) > 0);
  return {
    alive: true,
    version: 'fake0000',
    protocol: PROTOCOL,
    schema: 1,
    started_at: startedAt,
    terminal_host: { name: 'fake', health: 'ok' },
    agents: { total: sessions.size, by_state },
    inbox: { unread },
    resources: fakeResources(sessions, startedAt),
    keep_awake: { mode: keepAwake, held, ...(held ? { since: startedAt } : {}) },
    ...(needs.length ? { needs_you: [...needs] } : {}),
    ...(paused ? { paused } : {}),
  };
}

const GB = 2 ** 30;

/** The daemon's measure of what each agent and the machine use, with fixed figures: a running agent works a core, the rest idle. */
function fakeResources(sessions: Map<string, Session>, at: Timestamp) {
  const agents: Record<string, { cpu_pct: number; rss_bytes: number }> = {};
  for (const [name, s] of sessions) agents[name] = { cpu_pct: s.state === 'running' ? 95 : 0.5, rss_bytes: 400 * 2 ** 20 };
  const machine = { ram_total_bytes: 32 * GB, ram_used_bytes: 20 * GB, ram_free_bytes: 12 * GB, load_1m: 3.2, cores: 10 };
  const all = Object.values(agents);
  return {
    at,
    agents,
    agents_total: { cpu_pct: all.reduce((n, a) => n + a.cpu_pct, 0), rss_bytes: all.reduce((n, a) => n + a.rss_bytes, 0) },
    start_hold: null,
    machine: { ...machine, load_per_core: 0.32, ram_free_pct: 37, disk_free_bytes: 120 * GB, disk_total_bytes: 500 * GB },
  };
}

type LoadParams = { minutes?: unknown; agent?: unknown };

/** load.get with fixed figures: each agent has a pane shell and a worker, a running agent's worker works a core, and one process is nobody's. */
export function loadReply(sessions: Map<string, Session>, at: Timestamp, p: LoadParams) {
  if (
    p.minutes !== undefined &&
    p.minutes !== null &&
    (!Number.isInteger(p.minutes) || (p.minutes as number) < 0 || (p.minutes as number) > 10080)
  )
    throw badRequest('minutes is a whole number from 0 to 10080', 'minutes');
  if (p.agent !== undefined && p.agent !== null && typeof p.agent !== 'string') throw badRequest('agent is a string', 'agent');
  const proc = (pid: number, agent: string | null, command: string, cpu: number, rss: number) => ({
    pid,
    ppid: agent ? 1 : 0,
    agent,
    found_by: agent ? ('env' as const) : null,
    command,
    started: at,
    cpu_pct: cpu,
    rss_bytes: rss,
    running: true,
    sampled: at,
  });
  const agents = [];
  const processes = [];
  let pid = 1000;
  for (const [name, s] of sessions) {
    if (typeof p.agent === 'string' && p.agent !== name) continue;
    const cpu = s.state === 'running' ? 95 : 0.5;
    const rows = [
      proc(pid++, name, 'claude', cpu, 400 * 2 ** 20),
      proc(pid++, name, 'pnpm test', s.state === 'running' ? 180 : 0, 200 * 2 ** 20),
    ];
    processes.push(...rows);
    agents.push({
      agent: name,
      cpu_pct: rows.reduce((n, r) => n + r.cpu_pct, 0),
      rss_bytes: rows.reduce((n, r) => n + r.rss_bytes, 0),
      processes: rows.length,
      found_by_env: rows.length,
      found_by_tree: 0,
      top: [...rows].sort((a, b) => b.cpu_pct - a.cpu_pct),
    });
  }
  const nobody = [proc(500, null, 'WindowServer', 12, 300 * 2 ** 20)];
  const all = agents;
  return {
    at,
    every_s: 5,
    machine: {
      cores: 10,
      load_1m: 3.2,
      load_per_core: 0.32,
      ram_total_bytes: 32 * GB,
      ram_used_bytes: 20 * GB,
      ram_free_bytes: 12 * GB,
      disk_free_bytes: 120 * GB,
      disk_total_bytes: 500 * GB,
    },
    total: {
      cpu_pct: all.reduce((n, a) => n + a.cpu_pct, 12),
      rss_bytes: all.reduce((n, a) => n + a.rss_bytes, 300 * 2 ** 20),
      processes: all.reduce((n, a) => n + a.processes, 1),
    },
    agents: agents.sort((a, b) => b.cpu_pct - a.cpu_pct),
    unowned: { cpu_pct: 12, rss_bytes: 300 * 2 ** 20, processes: 1, top: nobody },
    processes: [...processes, ...nobody],
    sampling: { samples: 12, mean_ms: 8.5 },
    ...(typeof p.minutes === 'number' && p.minutes > 0 ? { series: {} } : {}),
  };
}
