// Seeds for the Requests view beyond the founder's turn.
import type { FakeDaemon } from '#src/daemon.ts';

type Seed = (d: FakeDaemon) => void;

export const REQUEST_SEEDS: Record<string, Seed> = {
  /** Three `@human` comments that ask nothing, on the founder's own tasks, one seen in its author's chat: Requests lists the other two as notes. */
  'founder-notes': (d) => {
    const titles = [
      'Story 4: several machines working as one',
      'Story 6: worker pool across accounts',
      'Story 9: the app tells me what to look at',
    ];
    titles.forEach((title, i) => {
      const at = new Date(Date.now() - (60 + i) * 60_000).toISOString();
      const author = i ? 'handline.ui' : 'handline.lander';
      const memos = [{ id: 5000 + i, task: 3890 + i, author, at, text: '@human Landed on main.' }];
      d.tasks.seed({ id: 3890 + i, from: 'human', to: 'human', title, state: 'queued', memos });
    });
    // The founder read the lander's chat after its note: that note is read; handline.ui's two are not.
    const seen = d.chat.seed('handline.lander', {
      from: 'handline.lander',
      to: 'human',
      body: 'T3890 is on main.',
      at: new Date(Date.now() - 30 * 60_000).toISOString(),
      landing: 'delivered',
    });
    d.chat.seen({ agent: 'handline.lander', up_to: seen });
  },
};
