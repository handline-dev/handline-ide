// An agent that learned from Redos and now does well: its lessons, a record of 8 OK at first look in its last 10, and the founder's
// notice that it could be checked less (Requests, Settings › Agents › its page).
import type { FakeDaemon } from '#src/daemon.ts';

type Seed = (d: FakeDaemon) => void;

const ARCH = 'handline.architect'; // local-ref-ok
const DAY = 86_400_000;
const at = (daysAgo: number) => new Date(Date.now() - daysAgo * DAY).toISOString();

const LESSONS: [string, number][] = [
  ['Look at real screenshots of 10+ apps before drawing a new screen.', 8851],
  ['Lists are flat rows with hairlines, never cards; section headers stick.', 9068],
  ['A choice shows buttons first; inputs open in a modal. Primary button bottom-right.', 8851],
  ['Status-bar controls open popovers, not pages.', 8635],
  ['Show before and after side by side.', 8816],
  ['Put every state in the pictures: empty, loading, error, many.', 9132],
];

export const LEARNING_SEEDS: Record<string, Seed> = {
  learning: (d) => {
    d.tasks.lessons.set(
      ARCH,
      LESSONS.map(([text, task], i) => ({ id: i + 1, agent: ARCH, text, task, created_at: at(10 - i), created_by: ARCH })),
    );
    const verdicts = [false, true, true, true, false, true, true, true, true, true];
    d.tasks.reviews.set(ARCH, { level: 'every', record: verdicts.map((ok_first, i) => ({ task: 9150 + i, ok_first, at: at(10 - i) })) });
    d.tasks.warnings.push({
      task: null,
      title: 'architect has learned enough to be checked less',
      kind: 'warning',
      source: 'machine',
      id: `review:${ARCH}`,
      agent: null,
      ask: null,
      warning: { kind: 'review', agent: ARCH, line: '6 lessons. 8 of its last 10 tasks were OK at first look.', level: 'one_in_three' },
      items: [],
      unread: 0,
      first_unread: null,
      last_at: at(0.01),
    });
  },
};
