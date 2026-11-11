import { AnimatePresence, motion, useReducedMotion } from 'framer-motion';
import { ChevronDown } from 'lucide-react';
import { useState, useSyncExternalStore, type ReactNode } from 'react';
import type { HistoryRow, Task } from '@handline/client';
import { useFeed, useNameOf, useTaskChanges, useTasks } from '@handline/app-data';
import { look } from '#src/ui/taskLook.tsx';
import { spring } from '#src/motion.ts';
import { clock } from '#src/format/time.ts';
import { taskOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';

/** `start`: a task this agent was given, by its maker or handed on by its holder. */
export type TaskEventKind = 'add' | 'edit' | 'done' | 'handoff' | 'discard' | 'start';
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

/** The last thing done, said after where the task stands: "handed 21:28". Done and discarded say it in the state. */
const VERB: Record<TaskEventKind, string> = { add: 'created', edit: 'edited', done: '', handoff: 'handed', discard: '', start: 'given' };

/** Where the task stands now and who holds it: "With web-data", "Queued for daemon-core", "Done". */
function holder(t: Task, nameOf: (key: string) => string): string {
  const to = t.to === 'human' ? 'you' : nameOf(t.to);
  const word = look(t).word;
  if (t.state === 'done' || t.state === 'discarded' || word === 'Failed') return word;
  if (t.state === 'queued') return `Queued for ${to}`;
  if (t.state === 'draft') return `Draft for ${to}`;
  return t.state === 'doing' && word === 'Doing' ? `With ${to}` : `${word} · with ${to}`;
}

/** One task the card touched, with its changes, oldest first. */
type Touched = { id: string; events: TaskEvent[] };

/** The tasks kept open; older ones fold into one line at the top. */
const SHOWN = 2;
const FOLD_FROM = 4;

/** An agent's task changes in one card: one row per task, where it stands now; a task changed again changes its row in place. */
export function TaskActivity({ events }: { events: TaskEvent[] }) {
  const [open, setOpen] = useState(false);
  // Each task's state now, kept current by the tasks feed.
  const ids = [...new Set(events.map((e) => Number(e.id.slice(1))))];
  const tasks = useTasks({ ids }, { enabled: ids.length > 0 }).data?.items ?? [];
  const now = new Map(tasks.map((t) => [`T${t.id}`, t]));
  if (!events.length) return null;
  // In the order the tasks first came up, so a row stays where it is.
  const byTask = new Map<string, Touched>();
  for (const e of events) {
    const t = byTask.get(e.id) ?? { id: e.id, events: [] };
    t.events.push(e);
    byTask.set(e.id, t);
  }
  const touched = [...byTask.values()];
  const folds = touched.length >= FOLD_FROM;
  const older = folds ? touched.slice(0, -SHOWN) : [];
  const recent = folds ? touched.slice(-SHOWN) : touched;
  // On hover, the time under the card, in a row the size of the one under a message: one change's time, or the first to the last.
  const ats = events.flatMap((e) => (e.at ? [e.at] : []));
  const when = ats.length ? (clock(ats[0]!) === clock(ats.at(-1)!) ? clock(ats[0]!) : `${clock(ats[0]!)} ~ ${clock(ats.at(-1)!)}`) : '';
  return (
    <div className="group flex w-full min-w-0 flex-col items-start">
      <motion.div
        transition={spring}
        // Already there when the chat opens, it just shows; a card that comes later fades in with its line (the stream's AnimatePresence).
        initial={false}
        style={{ maxWidth: 'min(100%, 37.5rem)' }}
        className="overflow-hidden rounded-lg border border-border bg-bg text-sm"
      >
        {older.length > 0 && (
          <>
            {/* The fold line stays put; a click opens the older rows under it, in this card, and closes them again. */}
            <button
              type="button"
              aria-expanded={open}
              onClick={() => setOpen(!open)}
              className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs text-muted hover:bg-hover/30"
            >
              <span className="flex">
                {older.map((t) => (
                  <span
                    key={t.id}
                    className={`-mr-0.5 size-2 rounded-full border border-bg transition-colors duration-500 motion-reduce:transition-none ${now.get(t.id) ? look(now.get(t.id)!).tone : 'bg-surface'}`}
                  />
                ))}
              </span>
              <span className="min-w-0 truncate">
                Earlier {older.length} tasks · {older.map((t) => t.id).join(', ')}
              </span>
              <ChevronDown size={12} className={`shrink-0 transition-transform ${open ? 'rotate-180' : ''}`} />
            </button>
            <motion.div
              initial={false}
              animate={open ? { height: 'auto', opacity: 1 } : { height: 0, opacity: 0 }}
              transition={spring}
              className="overflow-hidden"
            >
              {older.map((t) => (
                <Row key={t.id} t={t} task={now.get(t.id)} ruled />
              ))}
            </motion.div>
          </>
        )}
        <AnimatePresence initial={false}>
          {recent.map((t, i) => (
            <Row key={t.id} t={t} task={now.get(t.id)} ruled={i > 0 || older.length > 0} />
          ))}
        </AnimatePresence>
      </motion.div>
      <time
        dateTime={ats[0]}
        className="mb-2 flex h-6 items-center text-xs text-muted opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
      >
        {when}
      </time>
    </div>
  );
}

function Row({ t, task, ruled }: { t: Touched; task?: Task; ruled: boolean }) {
  const s = task ? look(task) : null;
  const nameOf = useNameOf();
  const last = t.events.at(-1)!;
  // Where it stands now, then the last thing done and when: "With web-data · handed 21:28 · 2 changes".
  const did = [VERB[last.kind], last.at && clock(last.at)].filter(Boolean).join(' ');
  const line = [task ? holder(task, nameOf) : last.detail, did, t.events.length > 1 && `${t.events.length} changes`]
    .filter(Boolean)
    .join(' · ');
  return (
    <motion.div
      // A row that comes in only fades in.
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      transition={spring}
      className="overflow-hidden"
    >
      {/* A click opens the task, whose Activity has every change; Cmd+click beside. */}
      <button
        type="button"
        {...useOpenable(taskOf(Number(t.id.slice(1))))}
        className={`flex w-full gap-2.5 px-3 py-2 text-left hover:bg-hover/30 ${ruled ? 'border-t border-border' : ''}`}
      >
        <span
          className={`mt-0.5 grid size-5 shrink-0 place-items-center rounded-md text-accent-fg transition-colors duration-500 motion-reduce:transition-none ${s?.tone ?? 'bg-surface'}`}
        >
          {s && (
            <Swap k={s.word}>
              <s.icon size={12} strokeWidth={3} />
            </Swap>
          )}
        </span>
        <div className="min-w-0">
          <div className="truncate">
            <span className="mr-1 text-muted">{t.id}</span>
            <span className={`font-medium ${task?.state === 'discarded' ? 'text-muted line-through' : 'text-fg'}`}>
              {task?.title || last.title}
            </span>
          </div>
          <div className="grid text-xs text-muted">
            {/* Words change in place once the task is read; the first reading just shows. */}
            {task ? (
              <Swap k={line}>
                <span className="truncate">{line}</span>
              </Swap>
            ) : (
              <span className="truncate">{line}</span>
            )}
          </div>
        </div>
      </button>
    </motion.div>
  );
}

/** What it shows, changed in place: the old words slide up and out as the new come in under them; at once when motion is reduced. */
function Swap({ k, children }: { k: string; children: ReactNode }) {
  const still = useReducedMotion();
  const t = still ? { duration: 0 } : spring;
  return (
    <AnimatePresence initial={false}>
      <motion.span
        key={k}
        initial={{ opacity: 0, y: '60%' }}
        animate={{ opacity: 1, y: 0 }}
        // The old words leave quicker than the new come in, so the two hardly overlap.
        exit={{ opacity: 0, y: '-60%', transition: still ? { duration: 0 } : { duration: 0.12 } }}
        transition={t}
        className="col-start-1 row-start-1 flex min-w-0"
      >
        {children}
      </motion.span>
    </AnimatePresence>
  );
}

/** One task change, read from its history row: what kind of change, and the line under its title. */
function fromRow(task: number, title: string, rows: HistoryRow[]): TaskEvent | null {
  const id = `T${task}`;
  const key = `${task}:${rows[0].at}`;
  const at = rows[0].at;
  const state = rows.find((r) => r.field === 'state');
  if (state?.was === '' || rows.some((r) => r.field === 'title' && r.was === ''))
    return {
      key,
      at,
      kind: 'add',
      id,
      title,
      to: assignee(rows),
      detail: [assignee(rows), state?.now].filter(Boolean).join(' · ') || 'New task',
    };
  if (state?.now === 'done') return { key, at, kind: 'done', id, title, detail: state.note ?? '', before: state.was, after: 'done' };
  if (state?.now === 'discarded') return { key, at, kind: 'discard', id, title, detail: state.note ?? 'Discarded' };
  const who = rows.find((r) => r.field === 'assignee');
  if (who)
    return { key, at, kind: 'handoff', id, title, to: who.now, detail: state?.note ?? '', before: who.was || 'nobody', after: who.now };
  const r = rows[0];
  if (r.field === 'blocked_by')
    return { key, at, kind: 'edit', id, title, detail: r.now ? `Waits on ${r.now}` : `No longer waits on ${r.was}` };
  if (r.field === 'memo') return { key, at, kind: 'edit', id, title, detail: `Comment: ${r.now.replace(/\s+/g, ' ')}` };
  if (r.field === 'body') return { key, at, kind: 'edit', id, title, detail: 'Brief edited' };
  return { key, at, kind: 'edit', id, title, detail: r.field === 'title' ? '' : r.field, before: r.was || '—', after: r.now || '—' };
}
const assignee = (rows: HistoryRow[]) => rows.find((r) => r.field === 'assignee')?.now;

/** Per agent, the task changes it made while this page is open, kept across chat switches. */
const made = new Map<string, TaskEvent[]>();
const heard = new Set<() => void>();
const NONE: TaskEvent[] = [];
const listen = (f: () => void) => {
  heard.add(f);
  return () => void heard.delete(f);
};

/** The task changes `agent` makes from now on: each task.changed is matched to its newest history row, and kept when the agent made it. */
export function useTaskActivity(agent: string): TaskEvent[] {
  useFeed('tasks', true);
  useTaskChanges((t, now) => {
    const by = now[0]!.by;
    const ev = fromRow(t.id, t.title, now);
    if (!ev) return;
    const list = made.get(by) ?? [];
    if (list.some((x) => x.key === ev.key)) return;
    made.set(by, [...list, ev]);
    for (const f of heard) f();
  });
  // Through the store's subscription: React Compiler caches a plain `made.get(agent)` by `agent`.
  return useSyncExternalStore(listen, () => made.get(agent) ?? NONE);
}
