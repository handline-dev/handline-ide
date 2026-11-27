// Seeds for the Requests view beyond the founder's turn.
import type { FakeDaemon } from '#src/daemon.ts';

type Seed = (d: FakeDaemon) => void;

export const REQUEST_SEEDS: Record<string, Seed> = {
  /** The architect holds a task that waits on two of term-build's: the sidebar's hourglass and its line. */
  'waits-on': (d) => {
    d.tasks.seed({ id: 1684, from: 'human', to: 'handline.architect', title: 'Web UI design', state: 'doing' });
    d.tasks.seed({ id: 1786, from: 'handline.architect', to: 'handline.term-build', title: 'Split the store', state: 'doing' });
    d.tasks.seed({ id: 1787, from: 'handline.architect', to: 'handline.term-build', title: 'Tidy the list', state: 'queued' });
    d.tasks.find(1684).blockers = [
      { task: 1786, reason: 'needs the store' },
      { task: 1787, reason: 'needs the list' },
    ];
  },
  /** Drafts only the founder can run: one two queued tasks wait on, one its agent left behind, and a founder's turn that holds a task up. */
  'draft-runs': (d) => {
    const ago = (m: number) => new Date(Date.now() - m * 60_000).toISOString();
    // The daemon's status line of a draft only the founder can run.
    const waits = (text: string) => ({ state: 'waiting', reason: 'run', text, until: null, since: ago(19 * 60), tasks: [] });
    d.tasks.seed({
      id: 3950,
      from: 'handline.architect',
      to: 'handline.term-build',
      title: 'Split the session store',
      state: 'draft',
      status: waits('waits for you to run it: T3951, T3952 wait on it'),
    });
    d.tasks.seed({
      id: 3951,
      from: 'handline.architect',
      to: 'handline.ui',
      title: 'Show the split sessions',
      state: 'queued',
      blockers: [{ task: 3950, reason: 'needs the split store' }],
    });
    d.tasks.seed({
      id: 3952,
      from: 'handline.architect',
      to: 'handline.ui',
      title: 'Session list filters',
      state: 'queued',
      blockers: [{ task: 3951, reason: 'needs the split list' }],
    });
    d.tasks.seed({
      id: 3953,
      from: 'handline.ui',
      to: 'handline.ui',
      title: 'Tidy the tab bar',
      state: 'draft',
      status: waits('<@handline.ui> left it for you to run'),
    });
    const run = (task: number, title: string, agent: string, holds: number[], at: string, ask: string | null) => ({
      task,
      title,
      kind: 'run' as const,
      source: 'task' as const,
      id: String(task),
      agent,
      ask,
      warning: null,
      items: [],
      holds,
      unread: 0,
      first_unread: null,
      last_at: at,
    });
    d.tasks.runs.push(
      run(3950, 'Split the session store', 'handline.architect', [3951, 3952], ago(19 * 60), 'Run this when the store layout is agreed.'),
      run(3953, 'Tidy the tab bar', 'handline.ui', [], ago(3 * 60), null),
    );
    const ask = 'Which goes first: the store or the list?';
    const turns = [
      {
        agent: 'p1',
        ask,
        comment: null,
        by: 'handline.architect',
        since: ago(90),
        on_pass: null,
        evidence: null,
        edited: null,
        earlier: [],
      },
    ];
    d.tasks.seed({ id: 3954, from: 'handline.architect', to: 'handline.lander', title: 'Pick the merge order', state: 'queued', turns });
    d.tasks.seed({
      id: 3955,
      from: 'handline.architect',
      to: 'handline.lander',
      title: 'Land the session split',
      state: 'queued',
      blockers: [{ task: 3954, reason: 'needs the merge order' }],
    });
    d.tasks.holds.set(3954, [3955]);
  },

  /** handline.term-build asks again for a command it was allowed before word for word: its card offers Always allow for it. */
  'always-allow': (d) => {
    const s = d.sessions.get('handline.term-build');
    if (!s) return;
    s.prompt = {
      id: 'always-seed',
      agent: 'handline.term-build',
      kind: 'permission',
      tool: 'Bash',
      command: 'pnpm --dir apps/web test',
      reason: null,
      lines: [],
      options: [
        { key: 'yes', label: 'Allow once', answer: 'yes', highlighted: true },
        {
          key: 'always',
          label: 'Always allow term-build Bash(pnpm --dir apps/web test)',
          answer: 'yes',
          rule: 'Bash(pnpm --dir apps/web test)',
        },
        { key: 'no', label: 'Deny', answer: 'no' },
      ],
      since: new Date().toISOString(),
    };
  },

  /** Three comments to the founder that ask nothing, on the founder's own tasks, one seen in its author's chat: Requests lists the other two as notes. */
  'founder-notes': (d) => {
    const titles = [
      'Story 4: several machines working as one',
      'Story 6: worker pool across accounts',
      'Story 9: the app tells me what to look at',
    ];
    titles.forEach((title, i) => {
      const at = new Date(Date.now() - (60 + i) * 60_000).toISOString();
      const author = i ? 'handline.ui' : 'handline.lander';
      const comments = [{ id: 5000 + i, task: 3890 + i, author, at, text: '<@human> Landed on main.' }];
      d.tasks.seed({ id: 3890 + i, from: 'human', to: 'human', title, state: 'queued', comments });
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
