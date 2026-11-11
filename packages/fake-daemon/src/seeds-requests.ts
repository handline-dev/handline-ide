// Seeds for the Requests view beyond the founder's turn.
import type { FakeDaemon } from '#src/daemon.ts';

type Seed = (d: FakeDaemon) => void;

export const REQUEST_SEEDS: Record<string, Seed> = {
  /** Three unread `@human` comments that ask nothing, on the founder's own tasks: Requests lists them as notes, its count leaves them out. */
  'founder-notes': (d) => {
    const titles = [
      'Story 4: several machines working as one',
      'Story 6: worker pool across accounts',
      'Story 9: the app tells me what to look at',
    ];
    titles.forEach((title, i) => {
      const at = new Date(Date.now() - (60 + i) * 60_000).toISOString();
      const memos = [{ id: 5000 + i, task: 3890 + i, author: 'handline.lander', at, text: '@human Landed on main.' }];
      d.tasks.seed({ id: 3890 + i, from: 'human', to: 'human', title, state: 'queued', memos });
    });
  },
};
