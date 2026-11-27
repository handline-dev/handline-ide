// How the chat's lines become what is drawn: task notices, task comments and task changes heard live gathered into task cards, each task
// in one card only; and which lines draw nothing at all.
import { isPerson, unblockedStart, type TimelineEntry, type TurnRecord } from '#src/chat.ts';
import { isSystem } from '#src/task-people.ts';

/** `start`: a task this agent was given, by its maker or handed on by its holder; `comment`: a comment on it, its detail "Who: first line";
 * `alongside`: work on it beside the agent's own (a seat of it), its detail `started`, or how it ended (`done`, `handed_off`, `discarded`). */
export type TaskEventKind = 'add' | 'edit' | 'done' | 'handoff' | 'discard' | 'start' | 'comment';
export type TaskEvent = {
  key: string;
  kind: TaskEventKind;
  id: string;
  title: string;
  detail: string;
  before?: string;
  after?: string;
  /** The chat line it was drawn from (`item:<id>`), when it is not a message's. */
  line?: string;
  /** When it happened: shown under the card on hover. */
  at?: string;
  /** Who the task was given or handed to, for a made or handed task: matches a live change with its notice. */
  to?: string;
};

/** A run of task rows as one card. `id`, its React key, names the line before it, so the card changes in place as rows join or give way to
    their notice; `key` names its newest notice, so the reader's line is found again when an older page lands. */
export type TaskCard = { id: string; key: string; events: TaskEvent[] };
export type ChatRow = TimelineEntry | TurnRecord | TaskCard;

/** Who a message the daemon typed on a schedule is from. */
export const SCHEDULE_SENDER = 'schedule';
/** A scheduled message's own words: the daemon types `[schedule <cron>] <message>`, and the chat shows the message. */
export const scheduledText = (body: string): string => body.replace(/^\[schedule [^\]]*\]\s*/, '');

/** Whether a line draws nothing (what others said to the agent, a session a move started, an empty item): the chat line's own rules, so a
 * run of task cards is not cut by a line nobody sees. */
export function drawsNothing(e: TimelineEntry, agent: string, me: string): boolean {
  if (e.kind === 'item')
    return (
      e.item.session?.cause === 'move' || // the move's own line says it
      (!e.item.session && !e.item.text && e.item.kind !== 'bash' && !e.item.account_moved && !e.item.move_failed && !e.item.machine_moved)
    );
  if (e.kind === 'proposal') return true;
  // A message typed on a schedule is a quiet line: it is why the agent took a turn nobody asked for, and the list of schedules opens on it.
  if (e.message.from === SCHEDULE_SENDER) return !scheduledText(e.message.body).trim();
  // The daemon's own bookkeeping (sys digests and nudges) is not shown anywhere.
  if (isSystem(e.message.from)) return true;
  if (!isPerson(e.message.from, me) && !isPerson(e.message.to, me)) return e.message.from !== agent;
  const text = e.message.parts ? e.message.parts.some((p) => 'text' in p && p.text.trim()) : !!e.message.body;
  return !text && !(e.message.attachments ?? []).some((a) => a.mime.startsWith('image/'));
}

/** A task this agent gave or handed another agent, or one another agent gave or handed it, said as a message (kind task): drawn as the task, not as talk. */
const taskNotice = (e: TimelineEntry | TurnRecord, agent: string, me: string) =>
  e.kind === 'message' &&
  e.message.kind === 'task' &&
  e.message.tasks.length > 0 &&
  (e.message.from === agent ? !isPerson(e.message.to, me) : e.message.to === agent && !isPerson(e.message.from, me));

/** Whether a notice says a task was handed on, or made and given. */
const noticeKind = (body: string) => (/^T\d+\s+handed\b/i.test(body) ? 'handoff' : 'add');

/** A comment's first line; a mention in it reads as a name, or @You, where it is drawn. */
const firstLine = (body: string) => body.split('\n').find((l) => l.trim()) ?? '';

/** The entries, with each run of task notices gathered into one card's rows. */
export function taskRuns(
  entries: (TimelineEntry | TurnRecord)[],
  agent: string,
  me: string,
  nameOf: (key: string) => string,
  live: TaskEvent[] = [],
): ChatRow[] {
  const out: ChatRow[] = [];
  // Task changes heard while the chat is open go in by their time, joining a card next to them, so they scroll up as talk goes on.
  // A task made and given, or handed on, arrives twice, live and as the notice message: the notice's row is the one kept.
  const noticed = new Set(
    entries.flatMap((e) =>
      e.kind === 'message' && taskNotice(e, agent, me)
        ? e.message.tasks.map((t) => `${noticeKind(e.message.body)}:T${t.id}:${e.message.to}`)
        : [],
    ),
  );
  const heard = live
    .filter((ev) => !noticed.has(`${ev.kind}:${ev.id}:${ev.to}`))
    .sort((a, b) => Date.parse(a.at ?? '') - Date.parse(b.at ?? ''));
  const add = (rows: TaskEvent[], key: string) => {
    const last = out.at(-1);
    if (last && 'events' in last) {
      last.events.push(...rows);
      last.key = key;
    } else out.push({ id: `tasks:after:${last ? `${last.kind}:${last.id}` : ''}`, key, events: rows });
  };
  const until = (at: string) => {
    while (heard.length && Date.parse(heard[0].at ?? '') <= Date.parse(at)) {
      const ev = heard.shift()!;
      add([ev], `tasks:${ev.key}`);
    }
  };
  for (const e of entries) {
    until(e.at);
    // A comment on a task is that task's row, at its time, with its first line and who said it.
    const said = e.kind === 'item' ? e.item.task_comment : undefined;
    if (e.kind === 'item' && said) {
      const who = said.kind === 'answer' || isPerson(said.who, me) ? 'You' : nameOf(said.who);
      add(
        [
          {
            key: `c${e.id}`,
            kind: 'comment',
            id: `T${said.task}`,
            title: '',
            detail: `${who}: ${firstLine(said.text)}`,
            at: e.at,
            line: `item:${e.id}`,
          },
        ],
        `tasks:c${e.id}`,
      );
      continue;
    }
    const freed = e.kind === 'item' ? unblockedStart(e.item) : null;
    if (freed) {
      // The title is read with the task's state (TaskActivity).
      add(
        [{ key: `u${e.id}`, kind: 'start', id: `T${freed.task}`, title: '', detail: freed.detail, at: e.at, line: `item:${e.id}` }],
        `tasks:u${e.id}`,
      );
      continue;
    }
    if (!taskNotice(e, agent, me) || e.kind !== 'message') {
      // A line that draws nothing is kept out of the way: it goes before the card, so the run goes on.
      const last = out.at(-1);
      if (last && 'events' in last && e.kind !== 'record' && drawsNothing(e, agent, me)) out.splice(out.length - 1, 0, e);
      else out.push(e);
      continue;
    }
    const m = e.message;
    // A task handed on from another holder, or a new one made and given: said in words, its icon telling which.
    const handed = noticeKind(m.body) === 'handoff';
    // Given to this agent: where its work starts, from whom.
    const got = m.to === agent;
    const rows = m.tasks.map((t) => ({
      key: `${m.id}:${t.id}`,
      kind: got ? ('start' as const) : handed ? ('handoff' as const) : ('add' as const),
      id: `T${t.id}`,
      title: t.title,
      detail: got
        ? `${handed ? 'Handed' : 'Assigned'} by ${nameOf(m.from)}`
        : handed
          ? `Handed to ${nameOf(m.to)}`
          : `Created and assigned to ${nameOf(m.to)}`,
      at: m.at,
    }));
    add(rows, `tasks:${m.id}`);
  }
  until(new Date(8.64e15).toISOString());
  return out;
}

/** Task cards with each task in one card only, the first it is in, which takes its later changes: a row stays where it came up and
 * changes in place; a later card left with no task goes. */
export function onePerTask<R extends object>(rows: R[]): R[] {
  const eventsOf = (r: R): TaskEvent[] | null => ('events' in r && Array.isArray(r.events) ? r.events : null);
  const firstIn = new Map<string, number>();
  rows.forEach((r, i) => eventsOf(r)?.forEach((e) => firstIn.has(e.id) || firstIn.set(e.id, i)));
  const moved = new Map<number, TaskEvent[]>();
  for (const r of rows)
    for (const e of eventsOf(r) ?? []) {
      const to = firstIn.get(e.id)!;
      moved.set(to, [...(moved.get(to) ?? []), e]);
    }
  return rows.flatMap((r, i) => {
    if (!eventsOf(r)) return [r];
    const events = moved.get(i);
    return events ? [{ ...r, events }] : [];
  });
}

/** The lines each row draws, by key (`message:<id>`, `item:<id>`): what the reader has already seen is found by them. */
const linesOf = (r: ChatRow): string[] =>
  'events' in r
    ? r.events.map((e) => e.line ?? `message:${e.key.split(':')[0]}`)
    : r.kind === 'record'
      ? r.items.map((i) => `item:${i.id}`)
      : [`${r.kind}:${r.id}`];

/** The index of the last row already drawn, given the line that was last before this render (`before`); rows after it are new and fade
 * in. Infinity when that line is not among the entries (all is old), -1 when nothing is. */
export function knownUpTo(rows: ChatRow[], entries: TimelineEntry[], before: string | null): number {
  const at = rows.map(linesOf);
  for (let i = at.length - 1; i >= 0; i--) if (at[i].includes(before ?? '')) return i;
  // Not drawn: the line joined an earlier card of its task (onePerTask), so the rows that start above that line are the known ones.
  const pos = new Map(entries.map((e, n) => [`${e.kind}:${e.id}`, n]));
  const was = pos.get(before ?? '');
  if (was === undefined) return Infinity;
  for (let i = at.length - 1; i >= 0; i--) if (Math.min(...at[i].map((k) => pos.get(k) ?? Infinity)) <= was) return i;
  return -1;
}
