// More workspaces for the Town tab, enough to fill a screen: agents at work, resting, asleep, waiting, so each building looks different.
// The handline office has one of every state an office marks, and a task with a step count and a queue behind it. A doing line starts
// with its tool, as the daemon's does, so the people move by it: reading, writing or running something.
import type { FakeDaemon } from '#src/daemon.ts';
import { AGENTS } from '#src/fixtures.ts';
import { newSession } from '#src/session.ts';

type Seed = (d: FakeDaemon) => void;

/** What the people of a big team are doing, different from desk to desk so a room's lines can be told apart, each led by its tool so
 *  the room shows every motion. */
const DOINGS = [
  'Read Plan the next release',
  'Edit Fix the login redirect',
  'Write Add tests for the cart',
  'Agent Review the search change',
  'Bash Run the deploy script',
  'Grep Find the support thread',
  'Edit Draw the onboarding screens',
  'Bash Time the image upload',
  'Read Check the billing report',
  'Write Translate the help pages',
];
/** What those at work in the small teams do, in turn: one of each motion. */
const WORK = ['Read Look over the open issues', 'Edit Fix the failing check', 'Bash Run the tests', 'Agent Review the change'];

const PEOPLE: [string, 'running' | 'idle' | 'closed' | 'waiting', string?][] = [
  ['handline', 'running', 'Read the notes of the last release'],
  ['handline.ui', 'running', 'Edit the settings sheet'],
  ['shop', 'running', 'Grep Find every caller of the checkout'],
  ['shop.web', 'running', 'Write Add the product page'],
  ['shop.api', 'running', 'Bash Run the payment tests'],
  ['shop.design', 'idle'],
  ['shop.copy', 'waiting'],
  ['harbor', 'running', 'Read Look at the import logs'],
  ['harbor.data', 'closed'],
  ['personal', 'idle'],
  ...crowd(),
];

/** Fifteen more small teams, enough to fill a screen, and three big ones whose offices gain rows of desks: a name, its size and how
 *  many of it work. */
function crowd(): typeof PEOPLE {
  const teams: [string, number, number][] = [
    ['billing', 3, 2],
    ['docs', 2, 0],
    ['search', 4, 3],
    ['mobile', 5, 2],
    ['infra', 3, 1],
    ['growth', 2, 2],
    ['support', 4, 0],
    ['legal', 1, 0],
    ['data', 3, 3],
    ['ads', 2, 1],
    ['auth', 2, 1],
    ['ml', 6, 4],
    ['ops', 3, 0],
    ['brand', 2, 1],
    ['pay', 4, 2],
    ['lab', 8, 6],
    ['studio', 12, 9],
    ['factory', 24, 18],
  ];
  const roles = ['web', 'api', 'test', 'review', 'deploy', 'docs', 'data', 'infra', 'design', 'copy', 'ops', 'qa'];
  const more = ['perf', 'sec', 'mobile', 'search-index', 'ads', 'auth-gateway', 'payments-2', 'ml', 'growth', 'brand', 'legal-review'];
  const words = [...roles, ...more];
  return teams.flatMap(([team, size, work]) =>
    Array.from({ length: size }, (_, k): (typeof PEOPLE)[number] => {
      const name = k === 0 ? team : `${team}.${words[k - 1]}`;
      if (k < work) return [name, 'running', size > 6 ? DOINGS[k % DOINGS.length] : WORK[k % WORK.length]];
      return [name, k === size - 1 && size > 2 ? 'waiting' : 'idle'];
    }),
  );
}

/** The workspace whose office shows everything an office can; an agent's name there is this, a dot and its own word. */
const OFFICE = 'handline';
const inOffice = (word: string) => `${OFFICE}.${word}`;
/** In that office, one of each state an office marks: on hold at its usage limit waiting to switch account, stopped on a fault, asleep
 *  at an empty desk. */
const MARKED: {
  name: string;
  state: string;
  status: { state: string; reason: string; text: string };
  task?: { id: number; title: string };
  /** Waits at its account's limit for another account. */
  switches?: boolean;
}[] = [
  {
    name: inOffice('guide'),
    state: 'idle',
    status: { state: 'waiting', reason: 'wall', text: 'Usage limit, back at 14:00' },
    task: { id: 9817, title: 'Write the pairing guide' },
    switches: true,
  },
  {
    name: inOffice('checker'),
    state: 'stalled',
    status: { state: 'stopped', reason: 'stalled', text: 'Start failed: no session came up' },
  },
  { name: inOffice('sleeper'), state: 'closed', status: { state: 'off', reason: '', text: '' } },
];
/** Two tasks finished a little before now, so the list of what was done today has something in it. */
const DONE: { id: number; to: string; title: string; ago: number }[] = [
  { id: 9820, to: OFFICE, title: 'Agree on the release date', ago: 3 },
  { id: 9821, to: inOffice('guide'), title: 'Fix the links in the setup guide', ago: 1 },
];
/** The handline lead's task, three of its seven steps done, and what waits behind it: more than the papers on its desk name on hover. */
const LEAD_TASK = { id: 9810, title: 'Plan the release notes' };
const LEAD_QUEUE = [
  'Check the upgrade path from the last release',
  'Draft the changelog',
  'Collect screenshots of what changed',
  'Ask the team for known problems',
  'Write the migration notes',
  'Tag the release candidate',
];

export const TOWN_SEEDS: Record<string, Seed> = {
  town: (d) => {
    const base = AGENTS.find((a) => a.name === 'handline.auditor')!;
    const at = new Date().toISOString();
    for (const m of MARKED) {
      const wait = { cause: 'usage', from: base.account ?? 'beta', phase: 'waiting' as const, to: null, until: null, since: at };
      const view = { status: { ...m.status, until: null, since: at, tasks: [] }, ...(m.switches ? { account_switch: wait } : {}) };
      d.sessions.set(m.name, newSession({ ...base, name: m.name, state: m.state, wall: false, task: m.task ?? null, view }, at));
      d.structure(m.name);
      // A stalled start and a sleep leave a session to resume and none open, so Try again and Wake have one to start.
      if (m.state !== 'idle') d.sessions.get(m.name)!.open = false;
      if (m.task) d.tasks.seed({ ...m.task, from: 'human', to: m.name, state: 'doing' });
    }
    const todo = { done: 3, total: 7, current: 'List what changed for users', updated_at: at };
    d.tasks.seed({ ...LEAD_TASK, from: 'human', to: OFFICE, state: 'doing', todo });
    LEAD_QUEUE.forEach((title, i) => d.tasks.seed({ id: LEAD_TASK.id + 1 + i, from: 'human', to: OFFICE, title, state: 'queued' }));
    for (const { ago, ...t } of DONE) {
      const done = new Date(Date.now() - ago * 60_000).toISOString();
      d.tasks.seed({ ...t, from: 'human', state: 'done', done, evidence: 'Done.' });
    }
    for (const [name, state, doing] of PEOPLE) {
      const status = {
        state: state === 'closed' ? 'off' : state,
        reason: state === 'waiting' ? 'you' : '',
        text: '',
        until: null,
        since: at,
        tasks: [],
      };
      const view = { status, ...(doing ? { doing: { text: doing, at } } : {}) };
      const task = name === OFFICE ? LEAD_TASK : null;
      d.sessions.set(name, newSession({ ...base, name, state, wall: false, task, view }, at));
      d.structure(name);
      if (state === 'running') d.chat.setTurn(name, true);
    }
  },
};
