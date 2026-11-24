// More workspaces for the Town tab, enough to fill a screen: agents at work, resting, asleep, waiting, so each building looks different.
import type { FakeDaemon } from '#src/daemon.ts';
import { AGENTS } from '#src/fixtures.ts';
import { newSession } from '#src/session.ts';

type Seed = (d: FakeDaemon) => void;

const PEOPLE: [string, 'running' | 'idle' | 'closed' | 'waiting', string?][] = [
  ['handline', 'running', 'Plan the release notes'],
  ['handline.ui', 'running', 'Edit the settings sheet'],
  ['shop', 'running', 'Review the checkout change'],
  ['shop.web', 'running', 'Build the product page'],
  ['shop.api', 'running', 'Run the payment tests'],
  ['shop.design', 'idle'],
  ['shop.copy', 'waiting'],
  ['harbor', 'running', 'Read the import logs'],
  ['harbor.data', 'closed'],
  ['personal', 'idle'],
  ...crowd(),
];

/** Fifteen more small teams, enough to fill a screen: a name, its size and how many of it work. */
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
  ];
  const roles = ['web', 'api', 'test', 'review', 'deploy'];
  return teams.flatMap(([team, size, work]) =>
    Array.from({ length: size }, (_, k): (typeof PEOPLE)[number] => {
      const name = k === 0 ? team : `${team}.${roles[k - 1]}`;
      if (k < work) return [name, 'running', 'Ship the next change'];
      return [name, k === size - 1 && size > 2 ? 'waiting' : 'idle'];
    }),
  );
}

export const TOWN_SEEDS: Record<string, Seed> = {
  town: (d) => {
    const base = AGENTS.find((a) => a.name === 'handline.auditor')!;
    const at = new Date().toISOString();
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
      d.sessions.set(name, newSession({ ...base, name, state, wall: false, view }, at));
      d.structure(name);
      if (state === 'running') d.chat.setTurn(name, true);
    }
  },
};
