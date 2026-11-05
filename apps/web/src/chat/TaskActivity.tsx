import { AnimatePresence, motion } from 'framer-motion';
import { ArrowRight, Check, ChevronDown, Pencil, Plus, X } from 'lucide-react';
import { useEffect, useState } from 'react';
import type { HistoryRow, Task } from '@handline/client';
import { useFeed } from '@handline/app-data';
import { useStore } from '@handline/app-react';
import { spring } from '#src/motion.ts';
import { clock } from '#src/format/time.ts';
import { taskOf } from '#src/layout/kinds.tsx';
import { openProps, useOpenTab } from '#src/layout/openTab.tsx';

export type TaskEventKind = 'add' | 'edit' | 'done' | 'handoff' | 'discard';
export type TaskEvent = {
  key: string;
  kind: TaskEventKind;
  id: string;
  title: string;
  detail: string;
  before?: string;
  after?: string;
  /** When it happened: shown under the card on hover. */
  at?: string;
};

const LOOK: Record<TaskEventKind, { icon: typeof Plus; tone: string }> = {
  add: { icon: Plus, tone: 'bg-accent' },
  edit: { icon: Pencil, tone: 'bg-muted' },
  done: { icon: Check, tone: 'bg-success' },
  handoff: { icon: ArrowRight, tone: 'bg-warning' },
  discard: { icon: X, tone: 'bg-danger' },
};

/** The rows kept open; older ones fold into one line at the top. */
const SHOWN = 2;
const FOLD_FROM = 4;

/** An agent's task changes in one card: one row per change, a new one growing in at the bottom, the oldest folding up as they pile. */
export function TaskActivity({ events }: { events: TaskEvent[] }) {
  const [open, setOpen] = useState(false);
  if (!events.length) return null;
  const folds = events.length >= FOLD_FROM;
  const older = folds ? events.slice(0, -SHOWN) : [];
  const recent = folds ? events.slice(-SHOWN) : events;
  // On hover, the time under the card, in a row the size of the one under a message: one change's time, or the first to the last.
  const ats = events.flatMap((e) => (e.at ? [e.at] : []));
  const when = ats.length ? (clock(ats[0]!) === clock(ats.at(-1)!) ? clock(ats[0]!) : `${clock(ats[0]!)} ~ ${clock(ats.at(-1)!)}`) : '';
  return (
    <div className="group flex min-w-0 flex-col items-start">
      <motion.div
        layout
        transition={spring}
        // Already there when the chat opens, it just shows; a card that comes later fades in with its line (the stream's AnimatePresence).
        initial={false}
        className="max-w-150 overflow-hidden rounded-lg border border-border bg-bg text-sm"
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
                {older.map((e) => (
                  <span key={e.key} className={`-mr-0.5 size-2 rounded-full border border-bg ${LOOK[e.kind].tone}`} />
                ))}
              </span>
              <span className="min-w-0 truncate">
                Earlier {older.length} · {older.map((e) => e.id).join(', ')}
              </span>
              <ChevronDown size={12} className={`shrink-0 transition-transform ${open ? 'rotate-180' : ''}`} />
            </button>
            <motion.div
              initial={false}
              animate={open ? { height: 'auto', opacity: 1 } : { height: 0, opacity: 0 }}
              transition={spring}
              className="overflow-hidden"
            >
              {older.map((e) => (
                <Row key={e.key} e={e} ruled />
              ))}
            </motion.div>
          </>
        )}
        <AnimatePresence initial={false}>
          {recent.map((e, i) => (
            <Row key={e.key} e={e} ruled={i > 0 || older.length > 0} />
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

function Row({ e, ruled }: { e: TaskEvent; ruled: boolean }) {
  const { icon: Icon, tone } = LOOK[e.kind];
  const openTab = useOpenTab();
  return (
    <motion.div
      // A row that comes in only fades in; the card around it grows to it (its layout animation).
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      transition={spring}
      className="overflow-hidden"
    >
      {/* A click opens the task; Cmd+click beside. */}
      <button
        type="button"
        {...openProps((how) => openTab(taskOf(Number(e.id.slice(1))), how))}
        className={`flex w-full gap-2.5 px-3 py-2 text-left hover:bg-hover/30 ${ruled ? 'border-t border-border' : ''}`}
      >
        <span className={`mt-0.5 grid size-5 shrink-0 place-items-center rounded-md text-accent-fg ${tone}`}>
          <Icon size={12} strokeWidth={3} />
        </span>
        <div className="min-w-0">
          <div className="truncate">
            <span className="mr-1 text-muted">{e.id}</span>
            <span className={`font-medium ${e.kind === 'discard' ? 'text-muted line-through' : 'text-fg'}`}>{e.title}</span>
          </div>
          <div className="text-xs text-muted">
            {e.before !== undefined && (
              <>
                <span className="line-through">{e.before}</span> → {e.after}
                {e.detail && ' · '}
              </>
            )}
            {e.detail}
          </div>
        </div>
      </button>
    </motion.div>
  );
}

/** One task change, read from its history row: what kind of change, and the line under its title. */
function fromRow(task: number, title: string, rows: HistoryRow[]): TaskEvent | null {
  const id = `T${task}`;
  const key = `${task}:${rows[0].at}`;
  const at = rows[0].at;
  const state = rows.find((r) => r.field === 'state');
  if (state?.was === '' || rows.some((r) => r.field === 'title' && r.was === ''))
    return { key, at, kind: 'add', id, title, detail: [assignee(rows), state?.now].filter(Boolean).join(' · ') || 'New task' };
  if (state?.now === 'done') return { key, at, kind: 'done', id, title, detail: state.note ?? '', before: state.was, after: 'done' };
  if (state?.now === 'discarded') return { key, at, kind: 'discard', id, title, detail: state.note ?? 'Discarded' };
  const who = rows.find((r) => r.field === 'assignee');
  if (who) return { key, at, kind: 'handoff', id, title, detail: state?.note ?? '', before: who.was || 'nobody', after: who.now };
  const r = rows[0];
  if (r.field === 'memo') return { key, at, kind: 'edit', id, title, detail: `Memo: ${r.now.replace(/\s+/g, ' ')}` };
  if (r.field === 'body') return { key, at, kind: 'edit', id, title, detail: 'Brief edited' };
  return { key, at, kind: 'edit', id, title, detail: r.field === 'title' ? '' : r.field, before: r.was || '—', after: r.now || '—' };
}
const assignee = (rows: HistoryRow[]) => rows.find((r) => r.field === 'assignee')?.now;

/** Per agent, the task changes it made while this page is open, kept across chat switches. */
const made = new Map<string, TaskEvent[]>();
const heard = new Set<() => void>();

/** The task changes `agent` makes from now on: each task.changed is matched to its newest history row, and kept when the agent made it. */
export function useTaskActivity(agent: string): TaskEvent[] {
  const store = useStore();
  useFeed('tasks', true);
  const [, bump] = useState(0);
  useEffect(() => {
    const redraw = () => bump((n) => n + 1);
    heard.add(redraw);
    return () => void heard.delete(redraw);
  }, []);
  useEffect(
    () =>
      store.client.onEvent((e) => {
        if (e.type !== 'task.changed') return;
        const t = e.data as unknown as Task;
        void store.client.call('task.history', { task: t.id }).then(({ rows }) => {
          if (!rows.length) return;
          const at = rows[0].at;
          const now = rows.filter((r) => r.at === at);
          const by = now[0].by;
          const ev = fromRow(t.id, t.title, now);
          if (!ev) return;
          const list = made.get(by) ?? [];
          if (list.some((x) => x.key === ev.key)) return;
          made.set(by, [...list, ev]);
          for (const f of heard) f();
        });
      }),
    [store],
  );
  return made.get(agent) ?? [];
}
