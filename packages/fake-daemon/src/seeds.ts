// Seeds a UI check needs that the first-load fixtures leave out: switch them on by name, in a test or at serve time.
import type { FakeDaemon } from '#src/daemon.ts';
import { task } from '#src/seedkit.ts';
import { CHAT_SEEDS } from '#src/seeds-chat.ts';
import { PROPOSAL_SEEDS } from '#src/seeds-proposals.ts';
import { REQUEST_SEEDS } from '#src/seeds-requests.ts';
import { TURN_SEEDS } from '#src/seeds-turns.ts';

const ARCH = 'handline.architect';
const AUD = 'handline.auditor';
const ME = 'human';

type Seed = (d: FakeDaemon) => void;

const stall = (d: FakeDaemon, kind: 'start_failed' | 'no_session' | 'trust', title: string, text: string) => {
  const s = d.sessions.get(ARCH)!;
  s.state = 'stalled';
  const since = new Date(Date.now() - 5 * 60_000).toISOString();
  s.fx.view = {
    ...s.fx.view,
    status: { state: 'blocked', reason: 'stalled', text: `${title}: ${text}`, until: null, since, tasks: [] },
    needs_you: { kind, since, title, text, source: 'daemon' },
  };
};

export const SEEDS: Record<string, Seed> = {
  /** An inbox card the daemon raised after its recovery failed, with an agent, so it offers Open terminal. */
  'inbox-terminal': (d) =>
    void d.inbox.add({
      kind: 'block',
      agent: AUD,
      title: 'Recovery failed: the agent did not come back',
      body: 'Every step the daemon could take was taken; the terminal is the last one.',
      fix: 'open_terminal',
    }),

  /** A review: a task tagged review under the agent's own, a first comment naming two pictures, and a decision card asking for a look. */
  review: (d) => {
    const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64');
    const [before, after] = [d.uploads.add('before.png', png), d.uploads.add('after.png', png)];
    d.tasks.seed({ id: 9301, from: ARCH, to: ME, title: 'Review: wider margin on the board', state: 'doing', tags: ['review'] });
    d.tasks.memo(
      { task: 9301, text: `Margin widened.\n![before](attachment:${before.id})\n![after](attachment:${after.id})` },
      'add',
      ARCH,
    );
    d.inbox.add({ kind: 'decision', agent: ARCH, title: 'Review: wider margin on the board', body: 'Is this right?', task: 9301 });
  },

  /** A comment with five pasted pictures, two on one line and three each on their own: they fill a row and wrap (T9302). */
  pictures: (d) => {
    const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64');
    const ids = [1, 2, 3, 4, 5].map((i) => d.uploads.add(`shot-${i}.png`, png).id);
    const ref = (i: number) => `![shot ${i + 1}](attachment:${ids[i]})`;
    d.tasks.seed({ id: 9302, from: ARCH, to: ME, title: 'Five screenshots of the board', state: 'doing' });
    d.tasks.memo({ task: 9302, text: `Here they are.\n${ref(0)} ${ref(1)}\n\n${ref(2)}\n\n${ref(3)}\n${ref(4)}` }, 'add', ARCH);
  },

  /** An open task given to the person, with its card in the architect's chat: the card then offers Open and Done. */
  'task-card': (d) => {
    d.tasks.seed({ id: 1695, from: ARCH, to: ME, title: 'Confirm the chat plan', state: 'doing' });
    d.chat.seedItem(ARCH, {
      at: new Date().toISOString(),
      kind: 'task_event',
      task_event: { event: 'opened', task: 1695, request: null, evidence: null, who: ARCH },
    });
  },

  /** `handline.auditor` idle at its prompt with a shell, a monitor and two subagents running behind it, and two servers. */
  background: (d) => {
    const s = d.sessions.get(AUD)!;
    const ago = (min: number) => new Date(Date.now() - min * 60_000).toISOString();
    s.fx.view = {
      ...s.fx.view,
      status: { state: 'running', reason: 'background', text: 'tools/check.sh · Review the diff', until: null, since: ago(12), tasks: [] },
      background: [
        { kind: 'shell', label: 'tools/check.sh', since: ago(12) },
        { kind: 'monitor', label: 'Watch the deploy log for errors', since: ago(9) },
        { kind: 'subagent', label: 'Review the diff', since: ago(3), foreground: false },
        { kind: 'subagent', label: 'Explore', since: ago(0), foreground: true },
      ],
      servers: [
        { port: 5173, address: '127.0.0.1', command: 'node node_modules/.bin/vite', pid: 4101, since: ago(40) },
        { port: 8080, address: '0.0.0.0', command: 'python3 -m http.server 8080', pid: 4177, since: ago(6) },
      ],
    };
  },

  /** What waits on the founder, for the Requests view's first section and the bell: a decision, a question and a daemon warning. */
  'founder-turn': (d) => {
    d.tasks.seed({ id: 8635, from: ARCH, to: ME, title: 'Bell redesign: decision 2, a line for each pause', state: 'doing' });
    d.tasks.seed({ id: 8651, from: 'handline.ui', to: ME, title: 'Chat box in bash mode: which of two looks', state: 'doing' });
    d.inbox.add({ kind: 'decision', agent: ARCH, title: 'Bell redesign: decision 2, a line for each pause', body: 'A or B?', task: 8635 });
    d.inbox.add({ kind: 'ask', agent: 'handline.ui', title: 'Chat box in bash mode: which of two looks', body: 'Which one?', task: 8651 });
    d.tasks.warnings.push({
      task: null,
      title: 'gamma is not logged in: log in to put agents on it',
      kind: 'warning',
      source: 'machine',
      id: 'login:gamma',
      agent: null,
      ask: null,
      warning: { kind: 'login', account: 'gamma' },
      items: [],
      unread: 0,
      first_unread: null,
      last_at: new Date(Date.now() - 30 * 60_000).toISOString(),
    });
  },

  /** This machine is paused and holds T8642 from handline.auditor: its chat ends in the pause card. */
  paused: (d) => {
    d.admin.paused = { since: new Date(Date.now() - 20 * 60_000).toISOString(), until: null, machine: { id: 'm-fake', name: 'mac' } };
    d.tasks.seed({ id: 8642, from: ARCH, to: AUD, title: 'Audit the pause card', state: 'queued' });
    const s = d.sessions.get(AUD)!;
    s.fx.view = {
      ...s.fx.view,
      status: {
        state: 'waiting',
        reason: 'paused',
        text: 'Paused: new work waits',
        until: null,
        since: d.admin.paused.since,
        tasks: [8642],
      },
    };
  },

  /** handline.auditor waits on a question with several choices: its chat ends in the prompt card. */
  question: (d) => {
    const s = d.sessions.get(AUD)!;
    s.prompt = {
      id: 'q-seed',
      agent: AUD,
      kind: 'question',
      tool: 'AskUserQuestion',
      command: null,
      reason: null,
      lines: ['Which checks should run before the push?', '1. Lint', '2. Unit tests', '3. Contract tests'],
      options: [
        { key: '1', label: 'Lint' },
        { key: '2', label: 'Unit tests', highlighted: true },
        { key: '3', label: 'Contract tests' },
        { key: 'esc', label: 'Cancel' },
      ],
      multi: true,
      since: new Date().toISOString(),
    };
  },

  /** 200 tasks, so a list has to scroll and page. */
  'tasks-200': (d) => {
    const states = ['queued', 'doing', 'done', 'draft'];
    for (let i = 0; i < 200; i++) {
      task(d, 6000 + i, {
        id: 6000 + i,
        from: ARCH,
        to: i % 2 ? AUD : 'handline.term-build',
        title: `Seeded task ${i + 1}`,
        state: states[i % states.length]!,
        priority: ((i % 3) + 1) as 1 | 2 | 3,
      });
    }
  },

  /** The founder's requests: one waiting on them, one in review, one doing, a done one. */
  requests: (d) => {
    d.requests.add({
      title: 'Show the usage graph first',
      task: 6071,
      status: 'doing',
      agent: 'handline.pc',
      holders: ['handline.pc'],
      open: 2,
      done: 1,
    });
    d.requests.add({
      title: 'Open a new shell from the + menu',
      task: 6068,
      status: 'review',
      agent: 'handline.pc',
      holders: ['handline.pc'],
      open: 1,
      done: 2,
      prs: [630],
    });
    d.requests.add({
      title: 'Which build is live?',
      task: 6020,
      status: 'waiting_on_you',
      agent: 'handline',
      holders: ['handline'],
      open: 1,
      unread_comments: 2,
    });
    // The tasks behind the finished rows, so task.seen finds them.
    d.tasks.seed({ id: 5983, from: ME, to: 'handline.pc', title: 'Settings in one place', state: 'done' });
    d.tasks.seed({ id: 8613, from: ME, to: 'handline.ui', title: 'Coffee popover for keep-awake', state: 'done' });
    const ended = (min: number, seen: boolean) => ({
      outcome: 'done' as const,
      at: new Date(Date.now() - min * 60_000).toISOString(),
      seen,
    });
    d.requests.add({ title: 'Settings in one place', task: 5983, status: 'done', agent: 'handline.pc', done: 6, ended: ended(90, true) });
    d.requests.add({
      title: 'Coffee popover for keep-awake',
      task: 8613,
      status: 'done',
      agent: 'handline.ui',
      done: 2,
      ended: ended(20, false),
    });
  },

  /** The founder's open requests for the Tasks page's My requests chip: one waiting on them, one blocked, two being worked, one not taken yet. */
  'my-requests': (d) => {
    d.tasks.seed({ id: 7031, from: ME, to: 'handline.pc-shell', title: '[결정] 상태 줄 모양', state: 'doing' });
    d.tasks.seed({
      id: 7121,
      from: ME,
      to: 'handline.pc-shell',
      title: '상태 줄 모양 정하기',
      state: 'doing',
      blockers: [{ task: 7031, reason: '' }],
    });
    d.requests.add({ title: '설정 화면 목차를 좌측으로', task: 7102, status: 'waiting_on_you', open: 1, done: 4 });
    d.requests.add({
      title: '상태 줄 모양 정하기',
      task: 7121,
      open: 1,
      done: 0,
      now: 'handline.pc-chat',
      blocked: true,
      holders: ['handline.pc-chat'],
    });
    d.requests.add({
      title: '요청 화면 키보드로 처리',
      task: 7110,
      open: 2,
      done: 3,
      now: 'handline.daemon-core',
      holders: ['handline.daemon-core'],
      prs: [884],
    });
    d.requests.add({
      title: '업데이트 준비 중 로딩 표시',
      task: 7125,
      open: 2,
      done: 1,
      now: 'handline.pc-shell',
      holders: ['handline.pc-shell'],
    });
    d.requests.add({ title: '닫힌 태스크를 날짜별로 보기', task: 7133 });
  },

  /** The architect is held back for the machine's load: its chat and row say so, and agent.force_start starts it. */
  'agent-held': (d) => {
    const s = d.sessions.get(ARCH)!;
    const since = new Date(Date.now() - 4 * 60_000).toISOString();
    s.fx.view = {
      ...s.fx.view,
      status: {
        state: 'waiting',
        reason: 'load',
        text: 'Waiting: the computer is under heavy memory load right now. It starts by itself when the load drops.',
        until: null,
        since,
        tasks: [],
      },
    };
  },

  /** The architect stalled by the daemon: start_failed (long reason), no_session, trust. */
  'stall-start': (d) =>
    stall(
      d,
      'start_failed',
      "Won't start",
      'The last 3 starts failed: the claude binary exited with status 1 before it printed its prompt, after the vendor folder-trust screen timed out while the account was switching to beta, and the retry found the worktree locked by another process.',
    ),
  'stall-wake': (d) =>
    stall(d, 'no_session', "Woke but didn't start", 'It was woken 3 times and no session started: the terminal never came up.'),
  'stall-trust': (d) =>
    stall(
      d,
      'trust',
      'Waiting for folder trust',
      'Claude Code is asking whether to trust the files in /Users/ron/work/app. Only you can answer it, in the terminal.',
    ),

  /** The architect waits on another task (status reason task): its chat shows the blocked-by card with the task as a link. */
  'agent-blocked': (d) => {
    const s = d.sessions.get(ARCH)!;
    const since = new Date(Date.now() - 12 * 60_000).toISOString();
    s.fx.view = {
      ...s.fx.view,
      status: { state: 'waiting', reason: 'task', text: 'blocked by T1684 (human)', until: null, since, tasks: [1684] },
    };
  },

  /** The architect hit the session limit (needs_you kind usage): its chat shows the needs-you card. */
  'agent-usage': (d) => {
    const s = d.sessions.get(ARCH)!;
    const since = new Date(Date.now() - 3 * 60_000).toISOString();
    s.fx.view = { ...s.fx.view, needs_you: { kind: 'usage', since, title: 'Usage limit: choose what to do', text: null, source: 'hook' } };
  },

  /** The architect's account switch at each phase: what the working line draws from limit to the new session. */
  'switch-saving': (d) => switching(d, { phase: 'saving', to: null }),
  'switch-switching': (d) => switching(d, { phase: 'switching', to: 'beta' }),
  'switch-starting': (d) => switching(d, { phase: 'starting', to: 'beta' }),
  'switch-waiting': (d) => switching(d, { phase: 'waiting', to: null, until: inTwoHours() }),
  /** The same wait with no account to spare: the card offers beta, and agent.switch_now takes it. */
  'switch-offer': (d) => {
    switching(d, { phase: 'waiting', to: null, until: inTwoHours() });
    const s = d.sessions.get(ARCH)!;
    const since = new Date(Date.now() - 60_000).toISOString();
    s.fx.view = {
      ...s.fx.view,
      needs_you: {
        kind: 'switch_offer',
        since,
        title: 'No account has room to spare',
        text: 'beta is the closest, and may reach its limit again soon.',
        source: 'daemon',
        account: 'beta',
      },
    };
  },

  /** Three more accounts whose 5-hour number cannot be read as it stands: read 14 minutes ago, its window ended, no window open. */
  'usage-stale': (d) => {
    const h = (n: number) => new Date(Date.now() + n * 3_600_000).toISOString();
    const week = { used_pct: 40, resets_at: h(90), source: 'measured' };
    d.accounts.add({ name: 'delta', age_s: 14 * 60, five_hour: { used_pct: 23, resets_at: h(3), source: 'measured' }, weekly: week });
    d.accounts.add({ name: 'theta', age_s: 30, five_hour: { used_pct: 96, resets_at: h(-0.2), source: 'measured' }, weekly: week });
    d.accounts.add({ name: 'kappa', age_s: 30, five_hour: null, weekly: week });
  },

  /** The account beta stalled on a billing error: its row shows STALLED and a founder can Wake it. */
  'account-stalled': (d) =>
    void d.accounts.stall('beta', { reason: 'billing_error', detail: 'The turn ended: the organization has a billing problem.' }),
  /** A task the architect is working, with its session's to-do list: 3 of 5 done, on the fourth. */
  'task-progress': (d) => {
    const ago = (min: number) => new Date(Date.now() - min * 60_000).toISOString();
    task(d, 8871, {
      id: 8871,
      from: ME,
      to: ARCH,
      title: 'Tasks page: group by owner',
      body: 'Group the tasks page by owner, newest first in each group.',
      state: 'doing',
      created: ago(40),
      progress: { done: 3, total: 5, current: 'Write the grouping test', updated_at: ago(4) },
    });
  },
  ...TURN_SEEDS,
  ...PROPOSAL_SEEDS,
  ...REQUEST_SEEDS,
  ...CHAT_SEEDS,
};

const inTwoHours = () => new Date(Date.now() + 2 * 3_600_000).toISOString();

/** Puts the architect, on alpha at its 5-hour limit, in an account switch. */
function switching(d: FakeDaemon, sw: { phase: 'saving' | 'switching' | 'starting' | 'waiting'; to: string | null; until?: string }): void {
  const s = d.sessions.get(ARCH)!;
  const since = new Date(Date.now() - 20_000).toISOString();
  s.fx.account = sw.to === 'beta' && sw.phase === 'starting' ? 'beta' : 'alpha';
  s.fx.view = { ...s.fx.view, account_switch: { cause: 'usage', from: 'alpha', since, until: null, ...sw } };
}

/** Switches the named seeds on: a comma-separated list, or `all`. An unknown name is an error, not a silent no-op. */
export function applySeeds(d: FakeDaemon, names: string | string[]): string[] {
  const list = (Array.isArray(names) ? names : names.split(',')).map((n) => n.trim()).filter(Boolean);
  const on = list.includes('all') ? Object.keys(SEEDS) : list;
  for (const n of on) {
    const seed = SEEDS[n];
    if (!seed) throw new Error(`no seed "${n}"; known: ${Object.keys(SEEDS).join(', ')}, all`);
    seed(d);
  }
  return on;
}
