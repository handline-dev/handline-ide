// A second cluster, for a page that joined one: tools/app.mjs --fake serves a daemon with this seed beside the first.
import type { FakeDaemon } from '#src/daemon.ts';
import { AGENTS } from '#src/fixtures.ts';
import { newSession } from '#src/session.ts';

type Seed = (d: FakeDaemon) => void;

const ME = 'human';

export const CLUSTER_SEEDS: Record<string, Seed> = {
  /** Another cluster, Amber Fox: its own agents in a shop workspace, in place of the handline ones, and one of them asking the founder. */
  'second-cluster': (d) => {
    d.admin.rename('Amber Fox');
    d.sessions.clear();
    d.inbox.clear(); // the first-load items name the handline agents
    for (const name of ['shop.backend', 'shop.web'])
      d.sessions.set(
        name,
        newSession(
          { ...AGENTS[1]!, name, task: null, session: null, worktree: `/work/${name}`, branch: `yourname/${name}`, repo: 'acme/shop' },
          d.iso(),
        ),
      );
    const at = new Date(Date.now() - 5 * 60_000).toISOString();
    const ask = 'Rotate the API key?';
    d.tasks.seed({
      id: 41,
      from: ME,
      to: 'shop.backend',
      title: 'Rotate the payment API key',
      state: 'doing',
      created: at,
      body: 'The old key was in a log.',
      memos: [{ id: 1, at, author: 'shop.backend', text: ask, task: 41 }],
      turnLog: [{ at, who: `shop.backend → ${ME}`, text: ask, turn: 'push', memo: 1 }],
      turns: [{ agent: ME, ask, memo: 1, by: 'shop.backend', since: at, on_pass: null, evidence: null, edited: null, earlier: [] }],
      acting: ME,
    });
    d.inbox.add({ kind: 'decision', agent: 'shop.backend', title: 'Rotate the payment API key', body: ask, task: 41 });
    d.admin.prefs.pin(['shop.web']);
  },
  /** A Mac joined to a cluster with no workspace of its own (HANDLINE_FAKE_SEEDS=empty-board beside --cluster). */
  'empty-board': (d) => {
    d.sessions.clear();
    d.inbox.clear();
  },
};
