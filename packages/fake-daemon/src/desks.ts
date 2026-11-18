// The fake daemon's desks: a project's worktree folders that agents take and give back, how many it keeps, and their pins.
import type { Methods } from '@handline/contract';
import { Failure, invalidParams } from '#src/errors.ts';

type Desk = Methods['desks.list']['result']['desks'][number];
type Kept = { mode: 'auto' | 'manual'; value: number };

const REPO = '/work/handline';
const GB = 1 << 30;
/** What auto keeps for the seeded project: its week's busiest days averaged, plus one. */
const AUTO = 9;

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
  kept: Kept = { mode: 'auto', value: AUTO };

  constructor(now = Date.now()) {
    this.desks = seed(now);
  }

  /** The free, unpinned desks beyond the kept number unused for three days by tonight, the oldest first. */
  private tonight(keep: number): Set<string> {
    const cutoff = Math.floor(Date.now() / 1000) + 24 * 3600 - 3 * 24 * 3600;
    const free = this.desks.filter((d) => !d.holder && !d.pinned_to && d.last_used < cutoff).sort((a, b) => a.last_used - b.last_used);
    return new Set(free.slice(0, Math.max(0, this.desks.length - keep)).map((d) => d.path));
  }

  call(method: string, p: Record<string, unknown>): unknown {
    switch (method) {
      case 'desks.list': {
        const repo = typeof p.repo === 'string' && p.repo ? p.repo : null;
        const keep = this.kept.mode === 'manual' ? this.kept.value : AUTO;
        const gone = repo ? this.tonight(keep) : new Set<string>();
        const desks = this.desks
          .filter((d) => !repo || d.repo === repo)
          .sort((a, b) => a.last_used - b.last_used)
          .map((d) => ({ ...d, removed_tonight: gone.has(d.path) }));
        return { desks, kept: repo ? { mode: this.kept.mode, value: keep, auto_value: AUTO } : null };
      }
      case 'desk.pin': {
        const d = this.desks.find((x) => x.path === p.desk || x.name === p.desk);
        if (!d) throw invalidParams(`${String(p.desk)} is not a desk`, 'desk');
        d.pinned_to = typeof p.to !== 'string' || !p.to ? '' : p.to.startsWith('p') ? 'founder' : p.to;
        return { accepted: true, said: d.pinned_to ? `${d.name} is kept for ${d.pinned_to}` : `${d.name} goes to any agent` };
      }
      case 'desks.keep': {
        if (p.repo !== REPO)
          throw new Failure('not_found', `"${String(p.repo)}" is no repo an agent works in (repos.list)`, { what: 'repo', name: p.repo });
        if (p.mode === 'auto') {
          this.kept = { mode: 'auto', value: AUTO };
          return { accepted: true, said: 'handline keeps its desks by Auto' };
        }
        if (p.mode !== 'manual') throw invalidParams('mode is auto or manual', 'mode');
        if (!Number.isInteger(p.value) || (p.value as number) < 1 || (p.value as number) > 50)
          throw invalidParams('value is 1 to 50 desks', 'value');
        this.kept = { mode: 'manual', value: p.value as number };
        return { accepted: true, said: `handline keeps ${String(p.value)} desks` };
      }
    }
    return undefined;
  }
}
