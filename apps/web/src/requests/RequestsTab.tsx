// Requests: what waits on the founder (their turn, notes, rows put off) and what they decided today. Their requests in progress and
// finished are in Tasks (My requests). The bell's popover previews the turn and the finished.
import { ListLoading } from '#src/ui/Loading.tsx';
import { AnimatePresence, motion } from 'framer-motion';
import { Children, isValidElement, useEffect, useRef, useState, type ReactNode } from 'react';
import { spring } from '#src/motion.ts';
import { ChevronDown, ChevronUp, ListChecks } from 'lucide-react';
import type { PendingRequest, Task } from '@handline/client';
import { TaskTab } from '#src/tools/TaskTab.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { useNameOf, useTasks } from '@handline/app-data';
import { clock } from '#src/format/time.ts';
import { taskOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { laterWords, pickAllState, turnKey, turnRow, type Picked } from '@handline/app-core';
import { useRequests } from '#src/requests/data.ts';
import { BulkButtons, PickProvider, usePick } from '#src/requests/RowMenu.tsx';
import { TurnLine } from '#src/requests/rows.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { selectionRoom, WithSelectionBar } from '#src/ui/SelectionBar.tsx';

/** A section's rows: one that comes or goes opens or closes its height, and the rest move to their new places (a row whose time
 *  changed moves to its new place in the order). */
function Rows({ children }: { children?: ReactNode }) {
  return (
    <div className="flex flex-col">
      <AnimatePresence initial={false}>
        {Children.map(children, (c) =>
          isValidElement(c) ? (
            <motion.div
              key={c.key}
              layout="position"
              initial={{ height: 0, opacity: 0 }}
              animate={{ height: 'auto', opacity: 1 }}
              exit={{ height: 0, opacity: 0 }}
              transition={spring}
              className="overflow-hidden"
            >
              {c}
            </motion.div>
          ) : null,
        )}
      </AnimatePresence>
    </div>
  );
}

/** A section of the list. Its title holds a box that picks every row of it, beside picks in other sections. */
function Section({
  title,
  rows,
  empty,
  action,
  children,
}: {
  title: string;
  rows: Picked[];
  empty: string;
  action?: ReactNode;
  children?: ReactNode;
}) {
  const pick = usePick();
  const all = pick ? pickAllState(rows, pick.picked) : 'none';
  return (
    <section className="flex flex-col">
      {/* The section's name stays under the tab bar while its rows pass below it. */}
      <h2 className="sticky top-0 z-20 flex min-h-9 items-center gap-2.5 border-b border-border bg-bg px-2 text-sm font-medium text-fg">
        {pick && rows.length > 0 && (
          <input
            type="checkbox"
            aria-label={`Pick every row of ${title}`}
            title={`Pick every row of ${title}`}
            checked={all === 'all'}
            ref={(box) => {
              if (box) box.indeterminate = all === 'some';
            }}
            onChange={() => pick.pickAll(rows, all !== 'all')}
            className="size-4 accent-accent"
          />
        )}
        <span>{title}</span>
        <span className="flex-1 text-muted tabular-nums">{rows.length > 0 ? rows.length : ''}</span>
        {action}
      </h2>
      <Rows>{children}</Rows>
      {rows.length === 0 && <p className="px-2 py-3 text-muted">{empty}</p>}
    </section>
  );
}

export function RequestsTab() {
  return (
    <PickProvider>
      <Requests />
    </PickProvider>
  );
}

function Requests() {
  const r = useRequests();
  // The task view shows beside the list only from 48rem (@3xl); narrower, a row opens its task the standard way.
  const box = useRef<HTMLDivElement>(null);
  const [split, setSplit] = useState(true);
  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setSplit(el.clientWidth >= 48 * parseFloat(getComputedStyle(document.documentElement).fontSize)));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const pick = usePick();
  const rows = {
    turn: r.turn.map((p) => turnRow(p)),
    notes: r.notes.map((p) => turnRow(p)),
    later: r.later.map((x) => turnRow(x.p)),
  };
  // A picked row that leaves the list (answered, or done elsewhere) is not picked any more.
  const listed = Object.values(rows)
    .flat()
    .map((x) => x.key)
    .join('|');
  useEffect(() => pick?.keep(new Set(listed.split('|'))), [listed]); // eslint-disable-line react-hooks/exhaustive-deps
  const n = pick?.picked.size ?? 0;
  // The rows with a task, in list order: the split view steps through them.
  const withTask = [...r.turn, ...r.notes, ...r.later.map((x) => x.p)].filter((p) => p.task !== null);
  const [{ key: chosenKey, at: lastAt }, setChosen] = useState<{ key: string | null; at: number }>({ key: null, at: 0 });
  const found = withTask.findIndex((p) => turnKey(p) === chosenKey);
  // A shown row that left the list (answered, put off) gives way to the one after it.
  const at = found >= 0 ? found : Math.min(lastAt, Math.max(0, withTask.length - 1));
  const chosen: PendingRequest | undefined = withTask[at];
  const step = (d: number) => {
    const next = withTask[at + d];
    if (next) setChosen({ key: turnKey(next), at: at + d });
  };
  const stepRef = useRef(step);
  useEffect(() => {
    stepRef.current = step;
  });
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null;
      if (t?.closest('input, textarea, select') || t?.isContentEditable || e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === 'j') stepRef.current(1);
      else if (e.key === 'k') stepRef.current(-1);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);
  const line = (p: PendingRequest, until?: string) => (
    <TurnLine
      key={turnKey(p)}
      p={p}
      until={until}
      full
      chosen={chosen === p}
      onPick={split && p.task !== null ? () => setChosen({ key: turnKey(p), at: withTask.indexOf(p) }) : undefined}
    />
  );
  return (
    // A pane too narrow for both keeps the list alone.
    <div ref={box} className="@container flex min-h-0 flex-1">
      <WithSelectionBar
        count={n}
        onClear={() => pick?.clear()}
        actions={<BulkButtons />}
        className="w-full shrink-0 @3xl:w-104 @3xl:border-r @3xl:border-border"
      >
        <ScrollFade keep="requests" fadeTop={false} className={`min-h-0 flex-1 ${selectionRoom(n)}`}>
          <div className="flex flex-col gap-6 px-3 py-4 text-sm">
            {r.failed && <p className="px-2 text-danger">Requests did not load.</p>}
            {!r.loaded && !r.failed && <ListLoading label="Loading requests" />}
            {r.loaded && (
              <>
                <Section title="Your turn" rows={rows.turn} empty="Nothing waits on you.">
                  {r.turn.map((p) => line(p))}
                </Section>
                {r.notes.length > 0 && (
                  <Section title="Notes" rows={rows.notes} empty="">
                    {r.notes.map((p) => line(p))}
                  </Section>
                )}
                {r.later.length > 0 && (
                  <Section title="Later" rows={rows.later} empty="">
                    {r.later.map(({ p, at: back }) => line(p, laterWords(back)))}
                  </Section>
                )}
                <Decided />
              </>
            )}
          </div>
        </ScrollFade>
      </WithSelectionBar>
      <div className="hidden min-w-0 flex-1 flex-col @3xl:flex">
        {chosen && chosen.task !== null ? (
          <>
            <div className="flex h-9 shrink-0 items-center justify-end gap-1 border-b border-border px-3 text-xs text-muted tabular-nums">
              <span className="mr-1">
                {at + 1} / {withTask.length}
              </span>
              <IconButton label="Previous (k)" onClick={() => step(-1)} disabled={at === 0}>
                <ChevronUp size={14} />
              </IconButton>
              <IconButton label="Next (j)" onClick={() => step(1)} disabled={at === withTask.length - 1}>
                <ChevronDown size={14} />
              </IconButton>
            </div>
            <TaskTab key={chosen.task} id={chosen.task} />
          </>
        ) : (
          r.loaded && <p className="m-auto text-sm text-muted">No task to show.</p>
        )}
      </div>
    </div>
  );
}

/** What the founder decided today, newest first: per task, the question, their answer, and what came of it. */
function Decided() {
  const [since] = useState(() => {
    const d = new Date();
    d.setHours(0, 0, 0, 0);
    return d.toISOString();
  });
  const q = useTasks({ decided_since: since });
  const nameOf = useNameOf();
  const items = (q.data?.items ?? []).filter(isDecided).sort((a, b) => b.decided.at.localeCompare(a.decided.at));
  if (!items.length) return <Section title="Decided today" rows={[]} empty={q.isPending ? '' : 'Nothing decided today.'} />;
  return (
    <section className="flex flex-col">
      <h2 className="sticky top-0 z-20 flex min-h-9 items-center border-b border-border bg-bg px-2 text-sm font-medium text-fg">
        Decided today
      </h2>
      <Rows>
        {items.map((t) => (
          <DecidedLine key={t.id} t={t} who={nameOf(t.to)} />
        ))}
      </Rows>
    </section>
  );
}

type DecidedTask = Task & { decided: NonNullable<Task['decided']> };
const isDecided = (t: Task): t is DecidedTask => !!t.decided;

function DecidedLine({ t, who }: { t: DecidedTask; who: string }) {
  const open = useOpenable(taskOf(t.id));
  const after = t.state === 'done' ? 'done' : t.state === 'discarded' ? 'discarded' : `${who} is on it`;
  return (
    <button type="button" {...open} className="flex w-full gap-2.5 border-b border-border px-2 py-3 text-left hover:bg-hover/30">
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="flex min-w-0 items-baseline gap-2">
          <span className="grid h-lh w-4 shrink-0 self-start place-items-center text-muted"><ListChecks size={16} /></span>
          <span className="shrink-0 text-muted tabular-nums">T{t.id}</span>
          <span className="truncate font-medium text-fg">{t.title}</span>
          <span className="shrink-0 text-muted">{who}</span>
          <span className="ml-auto shrink-0 pl-3 text-xs text-muted tabular-nums">{clock(t.decided.at)}</span>
        </span>
        <span className="truncate text-muted">
          You “{t.decided.answer.replace(/\s+/g, ' ')}” → {after}
        </span>
      </span>
    </button>
  );
}
