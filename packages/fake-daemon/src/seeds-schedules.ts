// Seeds for the list of schedules (Settings › Schedules, and an agent's settings): schedules that ran, one that is due, one that never
// ran, one whose cron has no sentence, and an agent whose stored value cannot be read. Each time one ran is a message from `schedule`
// in its agent's chat, so the list's jump to it has somewhere to land.
import type { FakeDaemon } from '#src/daemon.ts';
import type { FakeSchedule } from '#src/schedules.ts';

type Seed = (d: FakeDaemon) => void;

const ARCH = 'handline.architect'; // local-ref-ok
const AUD = 'handline.auditor'; // local-ref-ok
const BUILD = 'handline.term-build'; // local-ref-ok
const MIN = 60_000;

/** The nearest time after `from` (`dir` 1) or before it (-1) that is `hour`:`minute` on a day `days` takes, in this machine's zone. */
function near(hour: number, minute: number, days: (d: Date) => boolean, dir: 1 | -1, from = new Date()): Date {
  const t = new Date(from);
  t.setHours(hour, minute, 0, 0);
  while (dir * (t.getTime() - from.getTime()) <= 0 || !days(t)) t.setDate(t.getDate() + dir);
  return t;
}
const weekday = (d: Date) => d.getDay() >= 1 && d.getDay() <= 5;
const friday13 = (d: Date) => d.getDate() === 13 || d.getDay() === 5;

/** The times a schedule ran, newest first, each as a message in its agent's chat. */
function ran(d: FakeDaemon, agent: string, cron: string, message: string, times: Date[]): FakeSchedule['fires'] {
  return times.map((at) => {
    const request = d.chat.seed(agent, { from: 'schedule', to: agent, body: `[schedule ${cron}] ${message}`, at: at.toISOString() });
    return { at: at.toISOString(), request, ref: d.chat.find(request).ref ?? '' };
  });
}

export const SCHEDULE_SEEDS: Record<string, Seed> = {
  schedules: (d) => {
    const morning = { cron: '30 9 * * 1-5', message: 'Read the requests that are still open and say which ones wait on the founder.' };
    const audit = { cron: '0 9 13 * 5', message: "Audit the week's landed commits for missing tests." };
    const queue = { cron: '*/7 9-17 * * *', message: 'Check the build queue and restart a build that hangs.' };
    const tidy = {
      cron: '0 */2 * * *',
      message:
        'Prune the worktrees nothing uses any more. Keep one that holds uncommitted work, and say in one line which were removed. ' +
        'A worktree a task is still blocked on stays, whatever its age.',
    };
    const last = near(9, 30, weekday, -1);
    const before = near(9, 30, weekday, -1, last);
    const hour = new Date();
    hour.setHours(hour.getHours() + 2 - (hour.getHours() % 2), 0, 0, 0);
    d.schedules.rows = [
      {
        id: '4f1c2a9be0',
        agent: ARCH,
        ...morning,
        sentence: 'Every weekday at 09:30',
        message_more: false,
        next: near(9, 30, weekday, 1).toISOString(),
        due: false,
        fires: ran(d, ARCH, morning.cron, morning.message, [last, before, near(9, 30, weekday, -1, before)]),
      },
      {
        id: '9d3e7b1c44',
        agent: ARCH,
        ...queue,
        sentence: null,
        message_more: false,
        next: new Date(Date.now() - 3 * MIN).toISOString(),
        due: true,
        fires: ran(d, ARCH, queue.cron, queue.message, [new Date(Date.now() - 10 * MIN)]),
      },
      {
        id: 'c08a61f2d7',
        agent: AUD,
        ...audit,
        sentence: 'On the 13th of every month, and also every Friday, at 09:00',
        message_more: false,
        next: near(9, 0, friday13, 1).toISOString(),
        due: false,
        fires: ran(d, AUD, audit.cron, audit.message, [near(9, 0, friday13, -1)]),
      },
      {
        id: '2b77e5a013',
        agent: AUD,
        ...tidy,
        sentence: 'Every 2 hours from 00:00',
        message_more: true,
        next: hour.toISOString(),
        due: false,
        fires: [],
      },
    ];
  },
  /** An agent whose stored value has a key too many: the daemon sends none of its schedules, and the list says so. */
  'schedules-unreadable': (d) => void (d.schedules.unreadable = [{ agent: BUILD, why: 'schedule 2 is {cron, message} and nothing else' }]),
};
