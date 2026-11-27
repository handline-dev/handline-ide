// A task's sub-tasks: the graph with the open one beside it in a tab of its own (PlanTab).
import { useSize } from '#src/ui/useSize.ts';
import { useRead } from '@handline/app-data';
import { useContext, useEffect, useRef, type ReactNode, type RefObject } from 'react';
import { useTabState } from '#src/persist/tabState.ts';
import { planOf } from '#src/layout/kinds.tsx';
import { PlanGraph } from '#src/tools/PlanGraph.tsx';
import { TabRoute } from '#src/layout/openTab.tsx';
import { usePersisted } from '#src/persist/kv.ts';
import { Splitter } from '#src/ui/Splitter.tsx';
import { useEscape } from '#src/ui/useEscape.ts';
import { SubTask } from '#src/tools/StorySub.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';

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
