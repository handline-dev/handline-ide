// A task's past: the history rows a patch to it makes (one per field it changes, as the daemon keeps them), and task.time's segments and box.
import type { HistoryRow, Methods, Task, TaskClock, TimeSegment } from '@handline/contract';
import { invalidParams } from '#src/errors.ts';

type Fields = Task & { body: string; evidence?: string | null; blockers: { task: number }[]; estimate?: number | null };

export function changes(t: Fields, patch: Partial<Fields>, at: string, by: string): HistoryRow[] {
  const rows: HistoryRow[] = [];
  const log = (field: HistoryRow['field'], was: unknown, now: unknown) => {
    if (String(was ?? '') !== String(now ?? '')) rows.push({ at, by, field, was: String(was ?? ''), now: String(now ?? '') });
  };
  if ('title' in patch) log('title', t.title, patch.title);
  if ('to' in patch) log('assignee', t.to, patch.to);
  if ('body' in patch) log('body', t.body, patch.body);
  if ('evidence' in patch) log('evidence', t.evidence, patch.evidence);
  if ('priority' in patch) log('priority', t.priority ? `P${t.priority}` : '', patch.priority ? `P${patch.priority}` : '');
  if ('estimate' in patch) log('estimate', t.estimate ? `${t.estimate} points` : '', patch.estimate ? `${patch.estimate} points` : '');
  if ('time_box' in patch) log('timeout', t.time_box, patch.time_box);
  if ('state' in patch) log('state', t.state, patch.state);
  if (patch.blockers && patch.blockers.length > t.blockers.length)
    log('blocked_by', '', `T${patch.blockers.find((b) => !t.blockers.some((o) => o.task === b.task))?.task}`);
  return rows;
}

/** The plain fields a task.edit sets, as a patch: only those it names. */
export function fieldsOf(
  p: Record<string, unknown>,
): Partial<Pick<Fields, 'title' | 'body' | 'evidence' | 'time_box' | 'priority' | 'estimate'>> {
  return {
    ...(p.title !== undefined ? { title: String(p.title) } : {}),
    ...(p.body !== undefined ? { body: String(p.body) } : {}),
    ...(p.evidence !== undefined ? { evidence: String(p.evidence) } : {}),
    ...(p.time_box !== undefined ? { time_box: String(p.time_box) } : {}),
    ...(p.priority !== undefined ? { priority: p.priority as 1 | 2 | 3 } : {}),
    ...(p.estimate !== undefined ? { estimate: p.estimate as number | null } : {}),
  };
}

// task.time: queued until the task was delivered, then working, to its done or now. Its expectation is what the fake knows without
// history or accounts: an open task starts when it was delivered (now when not yet), with no duration to end it by.
interface Tasks {
  find(id: unknown): Pick<Task, 'id' | 'to' | 'created' | 'delivered' | 'done'> & {
    estimate?: number | null;
    segments?: TimeSegment[];
    clock?: TaskClock | null;
  };
  now(): string;
}

// The daemon adds the cursor.
type TaskTime = Omit<Methods['task.time']['result'], 'cursor'>;
type TaskBox = NonNullable<TaskTime['box']>;

export function timeOf(id: unknown, tasks: Tasks): TaskTime {
  if (!Number.isInteger(id)) throw invalidParams('task is a task number', 'task');
  const t = tasks.find(id);
  const end = t.done ?? tasks.now();
  const seg = (kind: 'queued' | 'working', from: string, to: string): TimeSegment => ({
    kind,
    on: null,
    text: kind === 'queued' ? 'queued' : '',
    task: null,
    from,
    to,
    seconds: Math.round((Date.parse(to) - Date.parse(from)) / 1000),
  });
  const made =
    t.segments ??
    (t.delivered ? [seg('queued', t.created, t.delivered), seg('working', t.delivered, end)] : [seg('queued', t.created, end)]);
  const expected = t.done
    ? null
    : { start: t.delivered ?? tasks.now(), end: null, duration_s: null, basis: null, ahead: [], wall_until: null };
  const segments = made.filter((s) => s.seconds > 0);
  return {
    task: t.id,
    start: t.created,
    end: t.done,
    segments,
    estimate: t.estimate ?? null,
    expected,
    box: t.done ? null : boxOf(t, segments),
  };
}

/** The box split as the daemon makes it, from the fake's one box: its working time is its holder's, its starting time the waking. */
function boxOf(t: Pick<Task, 'to'> & { clock?: TaskClock | null }, segments: TimeSegment[]): TaskBox {
  const length = t.clock?.length_s ?? 4 * 3600;
  const ran = segments.filter((s) => s.kind === 'working' || s.kind === 'starting');
  const worked = ran.filter((s) => s.kind === 'working').reduce((n, s) => n + s.seconds, 0);
  const waking = ran.filter((s) => s.kind === 'starting').reduce((n, s) => n + s.seconds, 0);
  return {
    length_s: length,
    start: ran[0]?.from ?? null,
    worked: worked > 0 ? [{ agent: t.to, seconds: worked }] : [],
    waking_s: waking,
    left_s: length - worked - waking,
  };
}
