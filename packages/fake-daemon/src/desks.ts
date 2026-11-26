// The fake daemon's desks: a project's worktree folders, each kept by its agent, and their pins.
import type { Methods } from '@handline/contract';
import { invalidParams } from '#src/errors.ts';

type Desk = Methods['desks.list']['result']['desks'][number];

const REPO = '/work/handline';
const GB = 1 << 30;

/** The picture's ten desks: four held, one pinned to the founder, the rest free, the oldest unused for days. */
function seed(now: number): Desk[] {
  const ago = (min: number) => Math.floor(now / 1000) - min * 60;
  const desk = (n: number, more: Partial<Desk>): Desk => ({
    path: `${REPO}.worktrees.noindex/desk-${n}`,
    name: `desk-${n}`,
    repo: REPO,
    holder: '',
    last_agent: '',
    last_used: ago(60),
    pinned_to: '',
    size_bytes: Math.round((1.6 + (n % 3) * 0.2) * GB),
    task: null,
    removed_tonight: false,
    ...more,
  });
  return [
    desk(10, { last_agent: 'handline.auditor', last_used: ago(3 * 24 * 60 + 30) }),
    desk(9, { last_agent: 'handline.architect', last_used: ago(2 * 24 * 60 + 10) }),
    desk(8, { last_agent: 'handline.term-build', last_used: ago(2 * 24 * 60) }),
    desk(7, { last_agent: 'handline.auditor', last_used: ago(2 * 24 * 60 - 30) }),
    desk(6, { last_agent: 'handline.architect', last_used: ago(20) }),
    desk(5, { last_agent: 'founder', pinned_to: 'founder', last_used: ago(10) }),
    desk(4, { holder: 'handline.auditor', last_agent: 'handline.auditor', task: 1690 }),
    desk(3, { holder: 'handline.architect', last_agent: 'handline.architect' }),
    desk(2, { holder: 'handline.term-build', last_agent: 'handline.term-build', task: 1688 }),
    desk(1, { holder: 'handline.architect', last_agent: 'handline.architect', task: 1684 }),
  ];
}

export class FakeDesks {
  desks: Desk[];

  constructor(now = Date.now()) {
    this.desks = seed(now);
  }

  call(method: string, p: Record<string, unknown>): unknown {
    switch (method) {
      case 'desks.list': {
        const repo = typeof p.repo === 'string' && p.repo ? p.repo : null;
        const desks = this.desks.filter((d) => !repo || d.repo === repo).sort((a, b) => a.last_used - b.last_used);
        return { desks, kept: null };
      }
      case 'desk.pin': {
        const d = this.desks.find((x) => x.path === p.desk || x.name === p.desk);
        if (!d) throw invalidParams(`${String(p.desk)} is not a desk`, 'desk');
        d.pinned_to = typeof p.to !== 'string' || !p.to ? '' : p.to.startsWith('p') ? 'founder' : p.to;
        return { accepted: true, said: d.pinned_to ? `${d.name} is kept for ${d.pinned_to}` : `${d.name} goes to any agent` };
      }
    }
    return undefined;
  }
}
