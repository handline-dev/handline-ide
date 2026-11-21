// An agent that learned from Redos and now does well: its lessons, a record of 8 OK at first look in its last 10, and the founder's
// notice that it could be checked less (Requests, Settings › Agents › its page).
import type { FakeDaemon } from '#src/daemon.ts';

type Seed = (d: FakeDaemon) => void;

const ARCH = 'handline.architect'; // local-ref-ok
const FOUNDER = 'p1'; // the founder's person key, where lessons.list for 'human' reads
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

/** The founder's preferences, which every agent reads. */
const PREFERENCES: [string, number | undefined][] = [
  ['Anything the founder reads is in Korean.', 9204],
  ['Hover on a subtle row is a faint fill, never an underline.', 9204],
  ['Show mockups and get a pick before building a new screen.', undefined],
];

export const LEARNING_SEEDS: Record<string, Seed> = {
  learning: (d) => {
    d.tasks.lessons.set(
      ARCH,
      LESSONS.map(([text, task], i) => ({ id: i + 1, agent: ARCH, text, task, created_at: at(10 - i), created_by: ARCH })),
    );
    d.tasks.lessons.set(
      FOUNDER,
      PREFERENCES.map(([text, task], i) => ({ id: 101 + i, agent: FOUNDER, text, task, created_at: at(5 - i), created_by: ARCH })),
    );
    const record = (from: number, oks: boolean[]) => oks.map((ok_first, i) => ({ task: from + i, ok_first, at: at(10 - i) }));
    d.tasks.reviews.set(
      ARCH,
      new Map([
        ['contract', { level: 'every', record: record(9140, [true, true, false, true, true]) }],
        ['screen', { level: 'every', record: record(9150, Array<boolean>(10).fill(true)) }],
        ['daemon', { level: 'one_in_two', record: record(9170, [true, false, true, true]) }],
      ]),
    );
    d.tasks.warnings.push({
      task: null,
      title: "architect's screen results: look at 1 in 2?",
      kind: 'warning',
      source: 'machine',
      id: `review:${ARCH}:screen`,
      agent: null,
      ask: null,
      warning: {
        kind: 'review',
        agent: ARCH,
        work: 'screen',
        line: 'Its last 10 screen results were OK at first look.',
        level: 'one_in_two',
      },
      items: [],
      unread: 0,
      first_unread: null,
      last_at: at(0.01),
    });
  },
};
