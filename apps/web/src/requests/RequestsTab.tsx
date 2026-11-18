import { Ago } from '#src/ui/Ago.tsx';
// Requests: what waits on the founder (their turn, notes, rows put off) and what they decided, newest first. Their requests in progress and
// finished are in Tasks (My requests). The bell's popover previews the turn and the finished.
import { ListLoading } from '#src/ui/Loading.tsx';
import { AnimatePresence, motion } from 'framer-motion';
import { Children, isValidElement, useEffect, useRef, useState, type MouseEvent, type ReactNode } from 'react';
import { spring } from '#src/motion.ts';
import { Lightbulb, ListChecks } from 'lucide-react';
import type { PendingRequest, Proposal, Task } from '@handline/client';
import { TaskTab } from '#src/tools/TaskTab.tsx';
import { Button } from '#src/ui/Button.tsx';
import { useNameOf, useProposals, useTasks } from '@handline/app-data';
import { chatOf, taskOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { laterWords, pickAllState, pickGroup, turnKey, turnRow, type Picked } from '@handline/app-core';
import { useRequests } from '#src/requests/data.ts';
import { BulkButtons, PickProvider, usePick } from '#src/requests/RowMenu.tsx';
import { TurnLine } from '#src/requests/rows.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { Splitter } from '#src/ui/Splitter.tsx';
import { useTabState } from '#src/persist/tabState.ts';
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
  // The box stands for one kind: the kind picked, else the top row's. All of it picked is a full check, not a part one.
  const kind = pick?.picked.values().next().value ?? rows[0];
  const ofKind = kind ? rows.filter((r) => pickGroup(r) === pickGroup(kind)) : rows;
  const all = pick ? pickAllState(ofKind, pick.picked) : 'none';
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
            onChange={() => pick.pickAll(ofKind, all !== 'all')}
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

/** The list keeps at least 320px, and leaves the task at least 360px. */
const fitList = (px: number, size: number) => Math.max(320, Math.min(px, size - 360));

function Requests() {
  const r = useRequests();
  // The task view shows beside the list only from 48rem (@3xl); narrower, a row opens its task the standard way.
  const box = useRef<HTMLDivElement>(null);
  // The list's width beside the task, dragged on the handle between them; kept with the tab.
  const [listWidth, setListWidth] = useTabState('list-width', 416);
  const [boxWidth, setBoxWidth] = useState(Infinity);
  const split = boxWidth >= 48 * parseFloat(getComputedStyle(document.documentElement).fontSize);
  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setBoxWidth(el.clientWidth));
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
  const [{ key: chosenKey, at: lastAt, task: chosenTask }, setChosen] = useState<{ key: string | null; at: number; task: number | null }>({
    key: null,
    at: 0,
    task: null,
  });
  const choose = (p: PendingRequest, i: number) => setChosen({ key: turnKey(p), at: i, task: p.task });
  const found = withTask.findIndex((p) => turnKey(p) === chosenKey);
  // A shown row that left the list (answered, put off) stays shown; until one is chosen, the first.
  const left = found < 0 && chosenTask !== null;
  const at = found >= 0 ? found : Math.min(lastAt, Math.max(0, withTask.length - 1));
  const shown = left ? chosenTask : (withTask[at]?.task ?? null);
  const chosen: PendingRequest | undefined = left ? undefined : withTask[at];
  // The row shown first counts as chosen, so it too stays when answered (set while rendering: it adjusts state, no effect).
  const first = withTask[0];
  if (chosenKey === null && first) choose(first, 0);
  // From a row that left, Next is the row now in its place and Previous the one above it.
  const step = (d: number) => {
    const i = left ? lastAt + (d > 0 ? 0 : -1) : at + d;
    const next = withTask[i];
    if (next) choose(next, i);
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
      onPick={split && p.task !== null ? () => choose(p, withTask.indexOf(p)) : undefined}
    />
  );
  return (
    // A pane too narrow for both keeps the list alone.
    <div ref={box} className="@container flex min-h-0 flex-1">
      <WithSelectionBar
        count={n}
        onClear={() => pick?.clear()}
        actions={<BulkButtons />}
        className="w-full shrink-0"
        style={split ? { width: fitList(listWidth, boxWidth) } : undefined}
      >
        <ScrollFade keep="requests" fadeTop={false} className={`min-h-0 flex-1 ${selectionRoom(n)}`}>
          <div className="flex flex-col gap-6 pb-4 text-sm">
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
                <Decided shown={shown} onPick={split ? (id) => setChosen({ key: `decided:${id}`, at: lastAt, task: id }) : undefined} />
              </>
            )}
          </div>
        </ScrollFade>
      </WithSelectionBar>
      {/* The same handle as between panes and beside the sidebar. */}
      {split && (
        <Splitter
          axis="x"
          container={box}
          percent={(listWidth / boxWidth) * 100}
          onMove={(at, size) => setListWidth(fitList(at, size))}
          onStep={(d) => setListWidth((w) => fitList(w + d * 16, boxWidth))}
          label="Resize the list"
        />
      )}
      <div className="hidden min-w-0 flex-1 flex-col @3xl:flex">
        {shown !== null ? (
          <>
            <TaskTab key={shown} id={shown} />
          </>
        ) : (
          r.loaded && <p className="m-auto text-sm text-muted">No task to show.</p>
        )}
      </div>
    </div>
  );
}

/** What the founder decided, newest first: per task, the question, their answer, and what came of it. */
/** Every decision so far, ten more at a time. */
const PAGE = 10;
const EVER = new Date(0).toISOString();

function Decided({ shown, onPick }: { shown: number | null; onPick?: (task: number) => void }) {
  // One read that grows (the daemon's newest decisions first, at most 200): it stays live as decisions come, and Show more asks for
  // ten more, which come below.
  const [limit, setLimit] = useState(PAGE);
  const q = useTasks({ decided_since: EVER, order: 'decided', limit });
  // HR proposals the founder answered: each state read whole (proposal.list with a state has no 7-day bound).
  const approved = useProposals({ state: 'approved' }).data?.proposals ?? [];
  const rejected = useProposals({ state: 'rejected' }).data?.proposals ?? [];
  const failed = useProposals({ state: 'apply_failed' }).data?.proposals ?? [];
  const nameOf = useNameOf();
  const tasks = (q.data?.items ?? []).filter(isDecided);
  // The proposals of the time the tasks read so far reach back to (all of them once every task is read), so Show more brings the
  // older ones of both below.
  const reach = q.data?.more ? tasks.at(-1)?.decided.at : undefined;
  const items: Decision[] = [
    ...tasks.map((t) => ({ kind: 'task' as const, at: t.decided.at, t })),
    ...[...approved, ...rejected, ...failed]
      .filter((x) => x.settled && (!reach || x.settled >= reach))
      .map((x) => ({ kind: 'proposal' as const, at: x.settled!, x })),
  ].sort((a, b) => b.at.localeCompare(a.at));
  if (!items.length) return <Section title="Decided" rows={[]} empty={q.isPending ? '' : 'Nothing decided yet.'} />;
  return (
    <section className="flex flex-col">
      <h2 className="sticky top-0 z-20 flex min-h-9 items-center border-b border-border bg-bg px-2 text-sm font-medium text-fg">Decided</h2>
      <Rows>
        {items.map((d) =>
          d.kind === 'task' ? (
            <DecidedLine
              key={`task:${d.t.id}`}
              t={d.t}
              who={nameOf(d.t.to)}
              chosen={shown === d.t.id}
              onPick={onPick && (() => onPick(d.t.id))}
            />
          ) : (
            <DecidedProposal key={`hr:${d.x.id}`} x={d.x} />
          ),
        )}
      </Rows>
      {q.data?.more && limit < 200 && (
        <Button variant="ghost" className="mt-2 self-center" onClick={() => setLimit((n) => Math.min(200, n + PAGE))}>
          Show more
        </Button>
      )}
    </section>
  );
}

type DecidedTask = Task & { decided: NonNullable<Task['decided']> };
type Decision = { kind: 'task'; at: string; t: DecidedTask } | { kind: 'proposal'; at: string; x: Proposal };

const ANSWER: Record<string, string> = { approved: 'Allowed', rejected: 'Denied', apply_failed: 'Allowed, not applied' };

/** An HR proposal the founder answered, drawn as a task decision is: what it asked, the answer, and when. It opens the asking agent's
 *  chat, as its Your turn row did. */
function DecidedProposal({ x }: { x: Proposal }) {
  const nameOf = useNameOf();
  const asker = x.requested_by ?? x.agent;
  const open = useOpenable(chatOf(asker));
  return (
    <button type="button" {...open} className="flex w-full gap-2.5 border-b border-border px-2 py-3 text-left hover:bg-hover/30">
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="flex min-w-0 items-baseline gap-2">
          <span className="grid h-lh w-4 shrink-0 self-start place-items-center text-muted">
            <Lightbulb size={16} />
          </span>
          <span className="shrink-0 text-muted">HR</span>
          <span className="truncate font-medium text-fg">{x.title}</span>
          <span className="shrink-0 text-muted">{nameOf(asker)}</span>
          <span className="ml-auto shrink-0 pl-3 text-xs text-muted tabular-nums">
            <Ago at={x.settled!} />
          </span>
        </span>
        <span className="truncate text-muted">
          You {ANSWER[x.state] ?? x.state}
          {x.note ? ` “${x.note.replace(/\s+/g, ' ')}”` : ''}
        </span>
      </span>
    </button>
  );
}
const isDecided = (t: Task): t is DecidedTask => !!t.decided;

/** A decision: a click shows its task beside the list as a Your turn row does (onPick), else opens it the standard way. */
function DecidedLine({ t, who, chosen, onPick }: { t: DecidedTask; who: string; chosen: boolean; onPick?: () => void }) {
  const open = useOpenable(taskOf(t.id));
  const after = t.state === 'done' ? 'done' : t.state === 'discarded' ? 'discarded' : `${who} is on it`;
  return (
    <button
      type="button"
      {...open}
      onClick={(e: MouseEvent) => (onPick && !e.metaKey && !e.ctrlKey && !e.altKey ? onPick() : open.onClick(e))}
      className={`flex w-full gap-2.5 border-b border-border px-2 py-3 text-left ${chosen ? 'bg-selected' : 'hover:bg-hover/30'}`}
    >
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="flex min-w-0 items-baseline gap-2">
          <span className="grid h-lh w-4 shrink-0 self-start place-items-center text-muted">
            <ListChecks size={16} />
          </span>
          <span className="shrink-0 text-muted tabular-nums">T{t.id}</span>
          <span className="truncate font-medium text-fg">{t.title}</span>
          <span className="shrink-0 text-muted">{who}</span>
          <span className="ml-auto shrink-0 pl-3 text-xs text-muted tabular-nums">
            <Ago at={t.decided.at} />
          </span>
        </span>
        <span className="truncate text-muted">
          You “{t.decided.answer.replace(/\s+/g, ' ')}” → {after}
        </span>
      </span>
    </button>
  );
}
