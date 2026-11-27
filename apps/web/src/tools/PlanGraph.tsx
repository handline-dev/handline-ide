// A story's sub-tasks as a graph: the open one outlined, its arrows in the accent colour, the cards with no link to it dimmed.
import { taskGraph, type GraphNode } from '@handline/app-core';
import { useTasks } from '@handline/app-data';
import { useMe } from '@handline/app-react';
import type { Task } from '@handline/client';
import { Play } from 'lucide-react';
import { useRef } from 'react';
import { taskOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { useRun } from '#src/tools/RunPlan.tsx';
import { wordOf, type Sub, type Word } from '#src/tools/storyWords.ts';
import { Button } from '#src/ui/Button.tsx';
import { FaceName } from '#src/ui/Face.tsx';
import { look } from '#src/ui/taskLook.tsx';
import { useSize } from '#src/ui/useSize.ts';

const BOX = { w: 200, h: 128 };

/** `onPick` null: a card opens its task the standard way (a pane too narrow for the detail beside the graph). */
export function PlanGraph({
  parent,
  subs,
  open: picked,
  onPick,
  bare = false,
}: {
  /** Only the graph, for a page that says the head and has its own Run. */
  bare?: boolean;
  parent: number;
  subs: Sub[];
  open: number | null;
  onPick: ((id: number) => void) | null;
}) {
  const me = useMe();
  const kids = [...(useTasks({ parent, all: true }).data?.items ?? [])].sort((a, b) => a.id - b.id);
  const [box, fit] = useFit();
  const g = taskGraph(kids, { me, closed: true, size: BOX });
  const k = fit(g.width, g.height);
  const byId = new Map(kids.map((x) => [x.id, x]));
  // Only a sub-task not yet done or discarded holds another back.
  const open = new Set(kids.filter((x) => !['done', 'discarded'].includes(x.state)).map((x) => x.id));
  const drafts = kids.filter((x) => x.state === 'draft');
  const { run } = useRun();
  const subOf = new Map(subs.map((x) => [x.id, x]));
  const word = (x: Task) => wordOf(x, subOf.get(x.id), me, open);
  const ready = (x: Task) => word(x).kind === 'you';
  const waiting = (x: Task) => word(x).kind === 'waiting';
  const tally = (label: string, f: (x: Task) => boolean) => {
    const n = kids.filter(f).length;
    return n ? `${n} ${label}` : null;
  };
  const touches = (e: { from: number; to: number }) => picked !== null && (e.from === picked || e.to === picked);
  const linked = new Set(g.edges.filter(touches).flatMap((e) => [e.from, e.to]));
  const head =
    drafts.length === kids.length && kids.length > 0
      ? [
          'Draft',
          `${kids.length} tasks`,
          `${subs.filter((x) => x.plan_state === 'ok').length} of ${kids.length} plans OK`,
          tally('for you', ready),
        ]
      : [
          `${kids.filter((x) => x.state === 'done').length}/${kids.length} merged`,
          tally('for you', ready),
          tally('up next', (x) => x.state === 'queued' && !waiting(x)),
          tally('working', (x) => x.state === 'doing' && !ready(x) && !waiting(x)),
          tally('waiting', waiting),
          tally('draft', (x) => x.state === 'draft'),
        ];
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      {!bare && (
        <p className="flex h-12 shrink-0 items-center gap-1.5 truncate border-b border-border px-6 text-sm text-muted">
          <span className="font-semibold text-fg">Plan</span>
          {head.filter(Boolean).map((w) => (
            <span key={String(w)}>· {w}</span>
          ))}
        </p>
      )}
      <div ref={box} className="flex min-h-0 flex-1 overflow-hidden p-6">
        <div className="relative mx-auto shrink-0" style={{ width: g.width * k, height: g.height * k }}>
          <div className="absolute top-0 left-0 origin-top-left" style={{ width: g.width, height: g.height, transform: `scale(${k})` }}>
            <svg width={g.width} height={g.height} className="absolute inset-0 text-muted" aria-hidden>
              <defs>
                {(['muted', 'accent'] as const).map((c) => (
                  <marker
                    key={c}
                    id={`story-arrow-${c}`}
                    viewBox="0 0 8 8"
                    refX="7"
                    refY="4"
                    markerWidth="8"
                    markerHeight="8"
                    orient="auto"
                  >
                    <path d="M0 0 L8 4 L0 8 z" className={c === 'accent' ? 'fill-accent' : 'fill-muted'} />
                  </marker>
                ))}
              </defs>
              {g.edges.map((e) => (
                <path
                  key={`${e.from}-${e.to}`}
                  d={curve(g.nodes, g.edges, e.from, e.to)}
                  fill="none"
                  className={touches(e) ? 'stroke-accent' : picked !== null ? 'stroke-muted opacity-30' : 'stroke-muted'}
                  strokeWidth={touches(e) ? 2 : 1}
                  markerEnd={`url(#story-arrow-${touches(e) ? 'accent' : 'muted'})`}
                />
              ))}
            </svg>
            {g.nodes.map((n) => (
              <Node
                key={n.id}
                n={n}
                t={byId.get(n.id)!}
                word={word(byId.get(n.id)!)}
                onPick={onPick}
                look={picked === null ? 'plain' : n.id === picked ? 'open' : linked.has(n.id) ? 'plain' : 'dim'}
              />
            ))}
          </div>
        </div>
      </div>
      {!bare && (
        <div className="flex shrink-0 items-center gap-3 border-t border-border px-6 py-3 text-xs text-muted">
          <span className="flex-1 truncate">A → B: B starts after A is merged</span>
          {drafts.length > 0 && (
            <Button variant="primary" onClick={() => run(drafts, open)}>
              <Play size={14} className="mr-1.5" />
              Run
            </Button>
          )}
        </div>
      )}
    </div>
  );
}

function Node({
  n,
  t,
  word,
  onPick,
  look: shown,
}: {
  n: GraphNode;
  t: Task;
  word: Word;
  onPick: ((id: number) => void) | null;
  look: 'plain' | 'open' | 'dim';
}) {
  const s = look(t);
  const draft = t.state === 'draft';
  const ready = word.kind === 'you';
  const openable = useOpenable(onPick ? null : taskOf(n.id));
  const frame = shown === 'open' ? 'border-2 border-accent' : ready ? 'border-warning' : 'border-border';
  return (
    <button
      type="button"
      {...(onPick ? { onClick: () => onPick(n.id) } : openable)}
      aria-current={shown === 'open' || undefined}
      style={{ left: n.x, top: n.y, width: n.w, height: n.h }}
      className={`absolute flex flex-col gap-1 rounded-md border p-3 text-left hover:bg-hover/30 ${draft && shown !== 'open' ? 'border-dashed' : ''} ${frame} ${ready ? 'bg-warning/10' : 'bg-bg'} ${shown === 'dim' ? 'opacity-40' : ''}`}
    >
      <span className="text-xs text-muted">T{n.id}</span>
      <span className="line-clamp-2 shrink-0 text-sm font-semibold text-fg">{n.title}</span>
      <span className="flex items-center gap-1.5 text-xs text-fg">
        <FaceName who={t.to} />
      </span>
      <span className={`mt-auto flex items-center gap-1 text-xs ${ready ? 'text-warning' : 'text-muted'}`}>
        {!draft && <s.icon size={12} />}
        {word.text}
      </span>
    </button>
  );
}

const xOf = (nodes: GraphNode[], id: number) => nodes.find((n) => n.id === id)!.x;

/** An arrow from the bottom of one box to the top of another, as a soft S. */
function curve(nodes: GraphNode[], edges: { from: number; to: number }[], from: number, to: number): string {
  const a = nodes.find((n) => n.id === from)!;
  const b = nodes.find((n) => n.id === to)!;
  // Each arrow leaves its own point on the bottom and lands on its own point on the top, so no two share a stroke.
  const spot = (n: GraphNode, list: number[], id: number) => n.x + (n.w * (list.indexOf(id) + 1)) / (list.length + 1);
  const outs = edges
    .filter((e) => e.from === from)
    .map((e) => e.to)
    .sort((p, q) => xOf(nodes, p) - xOf(nodes, q));
  const ins = edges
    .filter((e) => e.to === to)
    .map((e) => e.from)
    .sort((p, q) => xOf(nodes, p) - xOf(nodes, q));
  const [x1, y1, x2, y2] = [spot(a, outs, to), a.y + a.h, spot(b, ins, from), b.y - 2];
  const m = (y1 + y2) / 2;
  return `M${x1} ${y1} C${x1} ${m} ${x2} ${m} ${x2} ${y2}`;
}

/** The graph shrinks to fit its pane, never grows past its own size. */
function useFit() {
  const ref = useRef<HTMLDivElement>(null);
  const room = useSize(ref, (el) => ({ w: el.clientWidth - 48, h: el.clientHeight - 48 }), { w: 0, h: 0 });
  const fit = (w: number, h: number) => (room.w > 0 && w > 0 ? Math.min(1, room.w / w, room.h / h) : 1);
  return [ref, fit] as const;
}
