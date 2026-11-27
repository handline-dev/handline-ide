// A task's sub-tasks: the Plan box on its page (PlanFlow), and the graph with the open one beside it in a tab of its own (PlanTab).
import { useSize } from '#src/ui/useSize.ts';
import { useNameOf, useRead, useTasks } from '@handline/app-data';
import { useMe } from '@handline/app-react';
import type { Task, TaskDetail } from '@handline/client';
import { Play } from 'lucide-react';
import { useContext, useEffect, useRef, useState, type ReactNode, type RefObject } from 'react';
import { useTabState } from '#src/persist/tabState.ts';
import { planOf, taskOf } from '#src/layout/kinds.tsx';
import { useNewTab, useOpenable } from '#src/layout/openable.tsx';
import { PlanGraph } from '#src/tools/PlanGraph.tsx';
import { TabRoute } from '#src/layout/openTab.tsx';
import { usePersisted } from '#src/persist/kv.ts';
import { Splitter } from '#src/ui/Splitter.tsx';
import { useEscape } from '#src/ui/useEscape.ts';
import { SubTask } from '#src/tools/StorySub.tsx';
import { useRun } from '#src/tools/RunPlan.tsx';
import { startsAfter, wordOf, type Word } from '#src/tools/storyWords.ts';
import { Button } from '#src/ui/Button.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { look } from '#src/ui/taskLook.tsx';

const fitDetail = (px: number, size: number) => Math.max(360, Math.min(px, size - 320));

/** A task's sub-tasks as a graph in a tab of its own (plan:<id>); a card opens that sub-task beside the graph (plan:<id>/<sub>), and a
 * pane too narrow for both keeps the graph alone, as Requests does, its cards opening their tasks the standard way. */
export function PlanTab({ id, sub: picked }: { id: number; sub: number | null }) {
  const t = useRead('task.get', { task: id }).data;
  const route = useContext(TabRoute);
  const [held, setHeld] = useTabState<number | null>('sub', null);
  const shown = route ? picked : held;
  const pick = (n: number | null) => (route ? route(planOf(id, n)) : setHeld(n));
  const box = useRef<HTMLDivElement>(null);
  const boxWidth = useSize(box, (el) => el.clientWidth, 0);
  const split = boxWidth >= 48 * parseFloat(getComputedStyle(document.documentElement).fontSize);
  const [width, setWidth] = usePersisted('plan-detail-width', 560);
  const subs = t?.subtasks ?? [];
  const sub = split ? (subs.find((x) => x.id === shown) ?? null) : null;
  const order = [...subs].sort((a, b) => a.id - b.id);
  const at = sub ? order.findIndex((x) => x.id === sub.id) : -1;
  const step = (d: number) => {
    const next = order[at + d];
    if (next) pick(next.id);
  };
  useEscape(!!sub, () => pick(null));
  useArrows(!!sub, step, box);
  if (!t) return <div className="flex-1" />;
  return (
    <div ref={box} className="flex min-h-0 min-w-0 flex-1">
      <PlanGraph parent={t.id} subs={subs} open={sub?.id ?? null} onPick={split ? pick : null} />
      {sub && (
        <>
          <Splitter
            axis="x"
            container={box}
            percent={((boxWidth - fitDetail(width, boxWidth)) / boxWidth) * 100}
            onMove={(x, size) => setWidth(fitDetail(size - x, size))}
            onStep={(d) => setWidth((w) => fitDetail(w - d * 16, boxWidth))}
            label="Resize the task"
          />
          <div className="flex min-h-0 min-w-0 shrink-0 flex-col" style={{ width: fitDetail(width, boxWidth) }}>
            <SubTask key={sub.id} sub={sub} story={t} onPick={pick} at={[at + 1, order.length]} onStep={step} />
          </div>
        </>
      )}
    </div>
  );
}

/** Left and right step through the sub-tasks while one is open, unless typing. */
function useArrows(on: boolean, step: (d: number) => void, box: RefObject<HTMLElement | null>) {
  const latest = useRef(step);
  useEffect(() => {
    latest.current = step;
  });
  useEffect(() => {
    if (!on) return;
    const key = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      if (el && el !== document.body && !box.current?.contains(el)) return;
      if (el?.closest('input, textarea, select, [role=separator]') || el?.isContentEditable || e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === 'ArrowLeft') latest.current(-1);
      else if (e.key === 'ArrowRight') latest.current(1);
    };
    document.addEventListener('keydown', key);
    return () => document.removeEventListener('keydown', key);
  }, [on, box]);
}

const ROW = 64; // a flow box's height and the gap under it
const SHOWN = { now: 4, next: 3 };

/** The task page's Plan box: a bar of where the sub-tasks stand, then three columns, done folded to a count, what is on now (the
 * founder's first), and what waits directly on those, each with what it waits on and a line to it; more than fit fold to +N. */
export function PlanFlow({ t }: { t: TaskDetail }) {
  const me = useMe();
  const kids = useTasks({ parent: t.id, all: true }).data?.items ?? [];
  const [more, setMore] = useState({ now: false, next: false });
  const newTab = useNewTab(planOf(t.id));
  const graph = useOpenable(planOf(t.id));
  const subOf = new Map(t.subtasks.map((x) => [x.id, x]));
  const live = kids.filter((x) => x.state !== 'discarded');
  const open = new Set(live.filter((x) => x.state !== 'done').map((x) => x.id));
  const word = (x: Task) => wordOf(x, subOf.get(x.id), me, open);
  const forYou = (x: Task) => word(x).kind === 'you';
  const done = live.filter((x) => x.state === 'done').sort((a, b) => (b.done ?? '').localeCompare(a.done ?? ''));
  const held = (x: Task) => x.state === 'draft' || startsAfter(x, open).length > 0;
  const now = live
    .filter((x) => x.state !== 'done' && !held(x))
    .sort((a, b) => Number(word(b).kind === 'you') - Number(word(a).kind === 'you'));
  const nowIds = new Set(now.map((x) => x.id));
  const waits = live.filter((x) => x.state !== 'done' && held(x));
  // What waits directly on what is on now, and anything that asks the founder, theirs first.
  const next = waits
    .filter((x) => forYou(x) || startsAfter(x, open).some((n) => nowIds.has(n)) || !startsAfter(x, open).length)
    .sort((a, b) => Number(forYou(b)) - Number(forYou(a)));
  const later = waits.length - next.length;
  const drafts = live.filter((x) => x.state === 'draft');
  const { run } = useRun();
  const nowShown = more.now ? now : now.slice(0, SHOWN.now);
  const nextShown = more.next ? next : next.slice(0, SHOWN.next);
  if (!live.length) return null;
  const you = live.filter((x) => x.state !== 'done' && forYou(x)).length;
  const working = now.filter((x) => !forYou(x)).length;
  const waiting = waits.filter((x) => !forYou(x)).length;
  const head = [
    `${done.length} of ${live.length} merged`,
    you && `${you} ready for you`,
    working && `${working} working`,
    waiting && `${waiting} waiting`,
  ].filter(Boolean);
  const bar = [
    [done.length, 'bg-success'],
    [you, 'bg-warning'],
    [working, 'bg-accent'],
    [waiting, 'bg-border'],
  ] as const;
  const rows = Math.max(1, nowShown.length, nextShown.length);
  const lines = nextShown.flatMap((x, j) =>
    startsAfter(x, open).flatMap((n) => {
      const i = nowShown.findIndex((y) => y.id === n);
      return i < 0 ? [] : [{ key: `${n}-${x.id}`, y1: i * ROW + 28, y2: j * ROW + 28 }];
    }),
  );
  const column = (label: string) => <span className="text-xs text-muted">{label}</span>;
  return (
    <section className="flex flex-col gap-3 rounded-md border border-border p-4">
      <p className="flex items-center gap-1.5 text-sm text-muted">
        <span className="font-semibold text-fg">Plan</span>
        {head.join(' · ')}
        <button type="button" {...graph} onClick={newTab} className="ml-auto rounded-sm px-1 text-accent hover:bg-hover/30">
          Open graph in new tab
        </button>
        {drafts.length > 0 && (
          <Button size="sm" variant="primary" onClick={() => run(drafts, open)}>
            <Play size={12} />
            Run
          </Button>
        )}
      </p>
      <div className="flex h-2 gap-0.5 overflow-hidden rounded-full">
        {bar.map(([n, tone]) => n > 0 && <span key={tone} className={tone} style={{ flex: n }} />)}
      </div>
      <div className="grid gap-y-2" style={{ gridTemplateColumns: 'minmax(0,1fr) 3rem minmax(0,1fr) 3rem minmax(0,1fr)' }}>
        {column('Done')}
        <span />
        {column('Now')}
        <span />
        {column('Next up')}
        <div className="flex flex-col gap-2">
          {!done.length && <span className="px-3 text-xs text-muted">Nothing merged yet</span>}
          {done.length > 0 && (
            <div className="flex h-14 flex-col justify-center rounded-md border border-success/50 px-3 text-sm">
              <span className="font-semibold text-success">✓ {done.length} merged</span>
              <span className="truncate text-xs text-muted">
                last: T{done[0].id} {done[0].title}
              </span>
            </div>
          )}
        </div>
        <span />
        <div className="flex flex-col gap-2">
          {nowShown.map((x) => (
            <FlowBox key={x.id} t={x} word={word(x)} />
          ))}
          {now.length > nowShown.length && (
            <MoreButton n={now.length - nowShown.length} label="more" onClick={() => setMore({ ...more, now: true })} />
          )}
        </div>
        <svg
          className="h-full w-full text-border"
          style={{ height: rows * ROW }}
          preserveAspectRatio="none"
          viewBox={`0 0 48 ${rows * ROW}`}
          aria-hidden
        >
          {lines.map((l) => (
            <path key={l.key} d={`M0 ${l.y1} C24 ${l.y1} 24 ${l.y2} 48 ${l.y2}`} fill="none" stroke="currentColor" strokeWidth={1.3} />
          ))}
        </svg>
        <div className="flex flex-col gap-2">
          {nextShown.map((x) => (
            <FlowBox key={x.id} t={x} word={word(x)} />
          ))}
          {next.length > nextShown.length && (
            <MoreButton n={next.length - nextShown.length} label="next" onClick={() => setMore({ ...more, next: true })} />
          )}
          {later > 0 && <span className="px-3 text-xs text-muted">{later} later</span>}
        </div>
      </div>
    </section>
  );
}

function FlowBox({ t, word }: { t: Task; word: Word }) {
  const who = useNameOf()(t.to);
  const you = word.kind === 'you';
  const waiting = t.state === 'draft' || word.kind === 'waiting';
  const s = look(t);
  return (
    <button
      type="button"
      {...useOpenable(taskOf(t.id))}
      className={`flex h-14 min-w-0 flex-col justify-center rounded-md border px-3 text-left text-sm hover:bg-hover/30 ${
        you ? 'border-2 border-warning bg-warning/10' : waiting ? 'border-dashed border-border' : 'border-accent'
      }`}
    >
      <span className="truncate">
        <span className="text-muted">T{t.id}</span> <span className="font-semibold">{t.title}</span>
      </span>
      <span className={`flex items-center gap-1 truncate text-xs ${you ? 'text-warning' : waiting ? 'text-muted' : 'text-accent'}`}>
        {who} · {!waiting && t.state !== 'draft' && <s.icon size={12} />}
        {word.text}
      </span>
    </button>
  );
}

function MoreButton({ n, label, onClick }: { n: number; label: string; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} className="self-start rounded-sm px-3 text-xs text-accent hover:bg-hover/30">
      +{n} {label}
    </button>
  );
}

/** A story's chat column: the comments, then the box. */
export function storyChat(activity: ReactNode, box: ReactNode, id: number) {
  return (
    <>
      <ScrollFade keep={`task:${id}`} fromEnd data-task-scroll={id} className="min-h-0 flex-1">
        <div className="px-4 py-4 text-base text-fg">{activity}</div>
      </ScrollFade>
      {box}
    </>
  );
}
