import { letterOf } from '@handline/app-core';
import { useSize } from '#src/ui/useSize.ts';
import { Ago } from '#src/ui/Ago.tsx';
// Requests: what waits on the founder (their turn, notes, rows put off) and what they decided, newest first. Their requests in progress and
// finished are in Tasks (My requests). The bell's popover previews the turn and the finished.
import { ListLoading } from '#src/ui/Loading.tsx';
import { AnimatePresence, motion } from 'framer-motion';
import { Children, isValidElement, useEffect, useRef, type MouseEvent, type ReactNode } from 'react';
import { spring } from '#src/motion.ts';
import { Lightbulb, ListChecks } from 'lucide-react';
import type { PendingRequest, Proposal, Task } from '@handline/client';
import { ProposalView } from '#src/requests/ProposalView.tsx';
import { TaskTab } from '#src/tools/TaskTab.tsx';
import { Button } from '#src/ui/Button.tsx';
import { useDirectory, useNameOf, useProposals, useTasks } from '@handline/app-data';
import { chatOf, taskOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import {
  decidedTitle,
  isPainted,
  laterWords,
  pickAllState,
  pickGroup,
  plainRefs,
  requestClick,
  turnKey,
  turnRow,
  type Picked,
  type RowAim,
} from '@handline/app-core';
import { goToTurnOf, showComment } from '#src/tools/TaskTurn.ts';
import { ServiceRow, useServiceDown } from '#src/requests/ServiceRow.tsx';
import type { useRequests } from '#src/requests/data.ts';
import { InCluster } from '#src/connection/clusters.tsx';
import { useAllRequests, useTurnCount, type AllRequests } from '#src/requests/shared.tsx';
import { HereWord } from '#src/connection/ClusterWord.tsx';
import { BulkButtons, NoPick, PickProvider, usePick } from '#src/requests/RowMenu.tsx';
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
  count = rows.length,
  empty,
  action,
  children,
}: {
  title: string;
  /** The rows it picks from; `count`, every row it shows (another cluster's too). */
  rows: Picked[];
  count?: number;
  empty: string;
  action?: ReactNode;
  children?: ReactNode;
}) {
  const pick = usePick();
  // The box stands for one kind: the kind picked, else the top row's.
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
        <span className="flex-1 text-muted tabular-nums">{count > 0 ? count : ''}</span>
        {action}
      </h2>
      <Rows>{children}</Rows>
      {count === 0 && <p className="px-2 py-3 text-muted">{empty}</p>}
    </section>
  );
}

/** An HR proposal waiting on the founder: the split view shows its detail, as it shows a task. */
const isProposal = (p: PendingRequest) => p.source === 'proposal' && !!p.id;

export function RequestsTab() {
  return (
    <PickProvider>
      <Requests all={useAllRequests() ?? []} />
    </PickProvider>
  );
}

/** A row and the cluster it is from ('' for this Mac's). */
type At = { p: PendingRequest; cluster: string };
const keyOf = (x: At) => (x.cluster ? `${x.cluster}/${turnKey(x.p)}` : turnKey(x.p));

/** The list keeps at least 320px, and leaves the task at least 360px. */
const fitList = (px: number, size: number) => Math.max(320, Math.min(px, size - 360));

/** Every cluster's requests in one list: with several, newest first, each joined one's row with its name; only this Mac's are picked. */
function Requests({ all }: { all: AllRequests }) {
  const down = useServiceDown();
  const turnCount = useTurnCount();
  const own = all.find((a) => a.cluster === '')?.data;
  const several = all.length > 1;
  const merged = (of: (d: ReturnType<typeof useRequests>) => PendingRequest[]): At[] => {
    const xs = all.flatMap((a) => of(a.data).map((p) => ({ p, cluster: a.cluster })));
    return several ? xs.sort((a, b) => b.p.last_at.localeCompare(a.p.last_at)) : xs;
  };
  const r = {
    loaded: !!own?.loaded,
    failed: !!own?.failed,
    turn: merged((d) => d.turn),
    notes: merged((d) => d.notes),
    later: all.flatMap((a) => a.data.later.map((x) => ({ x: { p: x.p, cluster: a.cluster }, at: x.at }))),
  };
  // The task view shows beside the list only from 48rem (@3xl); narrower, a row opens its task the standard way.
  const box = useRef<HTMLDivElement>(null);
  // The list's width beside the task, dragged on the handle between them; kept with the tab.
  const [listWidth, setListWidth] = useTabState('list-width', 416);
  const boxWidth = useSize(box, (el) => el.clientWidth, Infinity);
  const split = boxWidth >= 48 * parseFloat(getComputedStyle(document.documentElement).fontSize);
  const pick = usePick();
  const mine = (xs: At[]) => xs.filter((x) => !x.cluster).map((x) => turnRow(x.p));
  const rows = { turn: mine(r.turn), notes: mine(r.notes), later: mine(r.later.map((l) => l.x)) };
  // A picked row that leaves the list (answered, or done elsewhere) is not picked any more.
  const listed = Object.values(rows)
    .flat()
    .map((x) => x.key)
    .join('|');
  useEffect(() => pick?.keep(new Set(listed.split('|'))), [listed]); // eslint-disable-line react-hooks/exhaustive-deps
  const n = pick?.picked.size ?? 0;
  // The rows the split view shows (a task's, or a proposal), in list order: it steps through them.
  const showable = [...r.turn, ...r.notes, ...r.later.map((l) => l.x)].filter((x) => x.p.task !== null || isProposal(x.p));
  type Chosen = { key: string | null; at: number; task: number | null; proposal?: number; cluster: string };
  // Kept with the tab, so leaving Requests and coming back shows the same row.
  const [{ key: chosenKey, at: lastAt, task: chosenTask, proposal: chosenProposal, cluster: chosenCluster }, setChosen] =
    useTabState<Chosen>('chosen', {
      key: null,
      at: 0,
      task: null,
      cluster: '',
    });
  const choose = (x: At, i: number) =>
    setChosen({ key: keyOf(x), at: i, task: x.p.task, proposal: isProposal(x.p) ? Number(x.p.id) : undefined, cluster: x.cluster });
  // A click: a row of the task already beside the list brings its own content into view there, not the task again (requestClick).
  const click = (aim: RowAim, pick: () => void) => {
    const { scroll } = requestClick(shown, aim);
    pick();
    if (scroll === 'turn') goToTurnOf(aim.task!);
    else if (scroll) showComment(scroll.comment);
  };
  const found = showable.findIndex((x) => keyOf(x) === chosenKey);
  // A shown row that left the list (answered, put off) stays shown; until one is chosen, the first.
  const left = found < 0 && (chosenTask !== null || chosenProposal !== undefined);
  const at = found >= 0 ? found : Math.min(lastAt, Math.max(0, showable.length - 1));
  const shown = left ? (chosenProposal !== undefined ? null : chosenTask) : (showable[at]?.p.task ?? null);
  const chosen: At | undefined = left ? undefined : showable[at];
  const shownIn = left ? chosenCluster : (chosen?.cluster ?? '');
  // The row shown first counts as chosen, so it too stays when answered (set while rendering: it adjusts state, no effect).
  const first = showable[0];
  if (chosenKey === null && first) choose(first, 0);
  // From a row that left, Next is the row now in its place and Previous the one above it.
  const step = (d: number) => {
    const i = left ? lastAt + (d > 0 ? 0 : -1) : at + d;
    const next = showable[i];
    if (next) choose(next, i);
  };
  const stepRef = useRef(step);
  useEffect(() => {
    stepRef.current = step;
  });
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null;
      if (t?.closest('input, textarea, select') || t?.isContentEditable || e.metaKey || e.ctrlKey || e.altKey || e.shiftKey) return;
      if (letterOf(e) === 'j') stepRef.current(1);
      else if (letterOf(e) === 'k') stepRef.current(-1);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);
  // Another cluster's row is drawn in it, so its answers and menu go there; it is not picked with this Mac's.
  const line = (x: At, until?: string) => (
    <InCluster key={keyOf(x)} id={x.cluster} dim>
      <NoPick off={!x.cluster}>
        <TurnLine
          p={x.p}
          until={until}
          full
          word={several && <HereWord />}
          chosen={!!chosen && isPainted(keyOf(chosen), keyOf(x))}
          onPick={
            split && x.p.task !== null
              ? () =>
                  click({ key: keyOf(x), task: x.p.task, comment: x.p.source === 'comment' && x.p.id ? Number(x.p.id) : undefined }, () =>
                    choose(x, showable.indexOf(x)),
                  )
              : undefined
          }
        />
      </NoPick>
    </InCluster>
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
            {r.failed && !down && <p className="px-2 text-danger">Requests did not load.</p>}
            {!r.loaded && !r.failed && !down && <ListLoading label="Loading requests" />}
            {(r.loaded || down) && (
              <Section title="Your turn" rows={rows.turn} count={turnCount} empty="Nothing waits on you.">
                {down && <ServiceRow key="service" />}
                {r.turn.map((p) => line(p))}
              </Section>
            )}
            {r.loaded && (
              <>
                {r.notes.length > 0 && (
                  <Section title="Notes" rows={rows.notes} count={r.notes.length} empty="">
                    {r.notes.map((p) => line(p))}
                  </Section>
                )}
                {r.later.length > 0 && (
                  <Section title="Later" rows={rows.later} count={r.later.length} empty="">
                    {r.later.map(({ x, at: back }) => line(x, laterWords(back)))}
                  </Section>
                )}
                {all.map((a) => (
                  <InCluster key={a.cluster} id={a.cluster}>
                    <Decided
                      painted={(id) => isPainted(chosenKey, `${a.cluster}/decided:${id}`)}
                      word={several && <HereWord />}
                      onPick={
                        split
                          ? (id) =>
                              click({ key: `${a.cluster}/decided:${id}`, task: id }, () =>
                                setChosen({ key: `${a.cluster}/decided:${id}`, at: lastAt, task: id, cluster: a.cluster }),
                              )
                          : undefined
                      }
                    />
                  </InCluster>
                ))}
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
        <InCluster id={shownIn}>
          {left && chosenProposal !== undefined ? (
            <ProposalView key={chosenProposal} id={chosenProposal} />
          ) : chosen && isProposal(chosen.p) ? (
            <ProposalView key={chosen.p.id} id={Number(chosen.p.id)} />
          ) : shown !== null ? (
            <TaskTab key={`${shownIn}/${shown}`} id={shown} />
          ) : (
            r.loaded && <p className="m-auto text-sm text-muted">No task to show.</p>
          )}
        </InCluster>
      </div>
    </div>
  );
}

/** What the founder decided, newest first: per task, the question, their answer, and what came of it. */
/** Every decision so far, ten more at a time. */
const PAGE = 10;
const EVER = new Date(0).toISOString();

function Decided({ painted, onPick, word }: { painted: (task: number) => boolean; onPick?: (task: number) => void; word?: ReactNode }) {
  // One read that grows (the daemon's newest decisions first, at most 200): it stays live as decisions come, and Show more asks for
  // ten more, which come below.
  const [limit, setLimit] = useTabState('decided-limit', PAGE);
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
  // Another cluster's decisions show only when there are some, under a heading with its name.
  if (!items.length) return word ? null : <Section title="Decided" rows={[]} empty={q.isPending ? '' : 'Nothing decided yet.'} />;
  return (
    <section className="flex flex-col">
      <h2 className="sticky top-0 z-20 flex min-h-9 items-center justify-between gap-2 border-b border-border bg-bg px-2 text-sm font-medium text-fg">
        Decided
        {word}
      </h2>
      <Rows>
        {items.map((d) =>
          d.kind === 'task' ? (
            <DecidedLine
              key={`task:${d.t.id}`}
              t={d.t}
              who={nameOf(d.t.to)}
              chosen={painted(d.t.id)}
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

/** A proposal the founder answered, drawn as a task decision is: what it asked, the answer, and when. It opens the asking agent's
 *  chat, as its Your turn row did. */
function DecidedProposal({ x }: { x: Proposal }) {
  const nameOf = useNameOf();
  const dir = useDirectory();
  const asker = x.requested_by ?? x.agent;
  const open = useOpenable(chatOf(asker));
  return (
    <button type="button" {...open} className="flex w-full gap-2.5 border-b border-border px-2 py-3 text-left hover:bg-hover/30">
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="flex min-w-0 items-baseline gap-2">
          <span className="grid h-lh w-4 shrink-0 self-start place-items-center text-muted">
            <Lightbulb size={16} />
          </span>
          <span className="truncate font-medium text-fg">{decidedTitle(x, nameOf, (t) => plainRefs(dir, t))}</span>
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
