// Seeds for the founder's turn on a task: asked, answered, asked again, and a note to them.
import type { FakeDaemon } from '#src/daemon.ts';

type Seed = (d: FakeDaemon) => void;

const ARCH = 'handline.architect';
const ME = 'human';

export const TURN_SEEDS: Record<string, Seed> = {
  /** The founder's turn on a task the architect holds, passed with its comment asking an A)/B) question, after a few comments. */
  'answer-turn': (d) => {
    const iso = (min: number) => new Date(Date.now() - min * 60_000).toISOString();
    const ask =
      'Which way should the bell count a pause?\nA) One line for each pause, newest first\nB) One line for each agent, its pauses folded under it';
    const memos = [
      { id: 1, at: iso(48), author: ARCH, text: 'Started on the bell redesign. Reading how pauses are counted today.' },
      {
        id: 2,
        at: iso(31),
        author: 'handline.ui',
        text: 'The bell now shows 14 for one agent that paused 14 times. That reads as 14 problems.',
      },
      {
        id: 3,
        at: iso(12),
        author: ARCH,
        text: 'Two ways to fix it, both drawn in the attached shots. They differ only in how a repeat pause is counted.',
      },
      { id: 4, at: iso(3), author: ARCH, text: ask },
    ].map((m) => ({ ...m, task: 8809 }));
    d.tasks.seed({
      id: 8809,
      from: ME,
      to: ARCH,
      title: 'Bell redesign: how a pause is counted',
      state: 'doing',
      created: iso(55),
      body: 'The bell counts every pause as its own line. Pick how repeats are shown.',
      memos,
      turnLog: [{ at: iso(3), who: `${ARCH} → ${ME}`, text: ask, turn: 'push', memo: 4 }],
      turns: [{ agent: ME, ask, memo: 4, by: ARCH, since: iso(3), on_pass: null, evidence: null, edited: null, earlier: [] }],
      acting: ME,
    });
    d.inbox.add({ kind: 'decision', agent: ARCH, title: 'Bell redesign: how a pause is counted', body: ask, task: 8809 });
  },

  /** A turn passed back and forth, each time with a comment: the architect asked, the founder answered, a note to the founder came
   * after with no turn, then it was asked again and a comment came after; and a task the architect assigned the founder with a note. */
  'turn-history': (d) => {
    const iso = (min: number) => new Date(Date.now() - min * 60_000).toISOString();
    const first = 'Should the bell count each pause, or each agent?';
    const again = 'Which way should the bell count a pause?\nA) One line for each pause, newest first\nB) One line for each agent';
    d.tasks.verbs = true; // the founder can answer or give the turn back
    d.tasks.seed({
      id: 8830,
      my_request: true,
      from: ME,
      to: ARCH,
      title: 'Bell: how a pause is counted',
      state: 'doing',
      created: iso(60),
      body: 'The bell counts every pause as its own line.',
      memos: [
        { id: 11, at: iso(50), author: ARCH, text: 'Reading how pauses are counted today.' },
        { id: 12, at: iso(40), author: ARCH, text: first },
        { id: 13, at: iso(30), author: ME, text: 'Each agent, but show me both first.' },
        { id: 14, at: iso(10), author: ARCH, text: '@human Both are drawn in the attached shots; picking one next.' },
        { id: 15, at: iso(3), author: ARCH, text: again },
        { id: 16, at: iso(1), author: 'handline.ui', text: 'B is the one the bell mock-up uses.' },
      ].map((m) => ({ ...m, task: 8830 })),
      turnLog: [
        { at: iso(40), who: `${ARCH} → ${ME}`, text: first, turn: 'push', memo: 12 },
        { at: iso(30), who: `${ME} → ${ARCH}`, text: '', turn: 'pop' },
        { at: iso(3), who: `${ARCH} → ${ME}`, text: again, turn: 'push', memo: 15 },
      ],
      turns: [{ agent: ME, ask: again, memo: 15, by: ARCH, since: iso(3), on_pass: null, evidence: null, edited: null, earlier: [] }],
      acting: ME,
    });
    // The same in the architect's chat, as task_comment items: its card there.
    d.chat.seed(ARCH, {
      from: ME,
      to: ARCH,
      kind: 'chat',
      tasks: [],
      body: 'Look at how the bell counts pauses.',
      at: iso(58),
      landing: 'delivered',
    });
    const lines = [
      [40, 12, 'turn', ARCH, first],
      [30, 13, 'answer', ME, 'Each agent, but show me both first.'],
      [10, 14, 'note', ARCH, '@human Both are drawn in the attached shots; picking one next.'],
      [3, 15, 'turn', ARCH, again],
    ] as const;
    for (const [min, memo, kind, who, text] of lines)
      d.chat.seedItem(ARCH, { at: iso(min), kind: 'task_comment', text, task_comment: { task: 8830, memo, kind, who, text } });
    d.tasks.seed({
      id: 8831,
      from: ARCH,
      to: ME,
      title: 'Pick the bell sound',
      state: 'doing',
      created: iso(20),
      body: 'The bell needs a sound for a pause.',
      memos: [{ id: 15, at: iso(5), author: ME, text: 'The soft one.', task: 8831 }],
      handoffLog: [{ at: iso(15), who: `${ARCH} → ${ME}`, text: 'Three sounds are in the attached file. Pick the one the bell plays.' }],
    });
  },
};
