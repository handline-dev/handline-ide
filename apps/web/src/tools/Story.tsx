// A story: a task with sub-tasks. Left, its comments as a chat with whoever holds it; right, the graph of its sub-tasks and Run.
import { foundersTurn, taskGraph, type GraphNode } from '@handline/app-core';
import { useNameOf, useTasks, useWrite } from '@handline/app-data';
import { useMe } from '@handline/app-react';
import type { Task, TaskDetail } from '@handline/client';
import { Play } from 'lucide-react';
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { taskOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { Button } from '#src/ui/Button.tsx';
import { ModalFrame, openModal } from '#src/ui/modal.tsx';
import { sayRefusal } from '#src/ui/refusal.tsx';
import { look } from '#src/ui/taskLook.tsx';
import { Avatar } from '#src/tools/TaskEvents.tsx';

const BOX = { w: 200, h: 128 };

export function Story({ t, chat }: { t: TaskDetail; chat: ReactNode }) {
  const name = useNameOf()(t.to);
  return (
    <div className="flex min-h-0 min-w-0 flex-1">
      <div className="flex min-h-0 min-w-0 basis-2/5 flex-col border-r border-border">
        <div className="flex h-12 shrink-0 items-center gap-2 border-b border-border px-4 text-sm font-semibold">
          <Avatar name={name} mine={false} />
          {name}
        </div>
        {chat}
      </div>
      <Plan parent={t.id} />
    </div>
  );
}

const startsAfter = (t: Task, open: Set<number>) => (t.blocked_by ?? []).filter((n) => open.has(n));

function Plan({ parent }: { parent: number }) {
  const me = useMe();
  const kids = [...(useTasks({ parent }).data?.items ?? [])].sort((a, b) => a.id - b.id);
  const [box, fit] = useFit();
  const g = taskGraph(kids, { me, closed: true, size: BOX });
  const k = fit(g.width, g.height);
  const byId = new Map(kids.map((x) => [x.id, x]));
  // Only a sub-task not yet done or discarded holds another back.
  const open = new Set(kids.filter((x) => !['done', 'discarded'].includes(x.state)).map((x) => x.id));
  const drafts = kids.filter((x) => x.state === 'draft');
  const ready = (x: Task) => x.state !== 'done' && foundersTurn(x, me);
  const waiting = (x: Task) => !ready(x) && x.state !== 'done' && look(x).word === 'Waiting';
  const tally = (label: string, f: (x: Task) => boolean) => {
    const n = kids.filter(f).length;
    return n ? `${n} ${label}` : null;
  };
  const head =
    drafts.length === kids.length
      ? ['Draft', `${kids.length} tasks`]
      : [
          `${kids.filter((x) => x.state === 'done').length}/${kids.length} merged`,
          tally('ready for you', ready),
          tally('up next', (x) => x.state === 'queued' && !waiting(x)),
          tally('working', (x) => x.state === 'doing' && !ready(x) && !waiting(x)),
          tally('waiting', waiting),
          tally('draft', (x) => x.state === 'draft'),
        ];
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <p className="flex h-12 shrink-0 items-center gap-1.5 border-b border-border px-6 text-sm text-muted">
        <span className="font-semibold text-fg">Plan</span>
        {head.filter(Boolean).map((w) => (
          <span key={String(w)}>· {w}</span>
        ))}
      </p>
      <div ref={box} className="flex min-h-0 flex-1 overflow-hidden p-6">
        <div className="relative mx-auto shrink-0" style={{ width: g.width * k, height: g.height * k }}>
          <div className="absolute top-0 left-0 origin-top-left" style={{ width: g.width, height: g.height, transform: `scale(${k})` }}>
            <svg width={g.width} height={g.height} className="absolute inset-0 text-muted" aria-hidden>
              <defs>
                <marker id="story-arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="8" markerHeight="8" orient="auto">
                  <path d="M0 0 L8 4 L0 8 z" fill="currentColor" />
                </marker>
              </defs>
              {g.edges.map((e) => (
                <path
                  key={`${e.from}-${e.to}`}
                  d={curve(g.nodes, g.edges, e.from, e.to)}
                  fill="none"
                  stroke="currentColor"
                  markerEnd="url(#story-arrow)"
                />
              ))}
            </svg>
            {g.nodes.map((n) => (
              <Node key={n.id} n={n} t={byId.get(n.id)!} ready={ready(byId.get(n.id)!)} open={open} />
            ))}
          </div>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-3 border-t border-border px-6 py-3 text-xs text-muted">
        <span className="flex-1">A → B: B starts after A is merged</span>
        {drafts.length > 0 && (
          <Button variant="primary" onClick={() => run(parent, drafts, open)}>
            <Play size={14} className="mr-1.5" />
            Run
          </Button>
        )}
      </div>
    </div>
  );
}

function Node({ n, t, ready, open }: { n: GraphNode; t: Task; ready: boolean; open: Set<number> }) {
  const go = useOpenable(taskOf(n.id));
  const who = useNameOf()(t.to);
  const s = look(t);
  const after = startsAfter(t, open);
  const draft = t.state === 'draft';
  const word = ready
    ? 'Ready for you'
    : s.word === 'Waiting' && after.length
      ? `Waiting on ${after.map((x) => `T${x}`).join(', ')}`
      : s.word;
  return (
    <button
      type="button"
      {...go}
      style={{ left: n.x, top: n.y, width: n.w, height: n.h }}
      className={`absolute flex flex-col gap-1 rounded-md border p-3 text-left hover:bg-hover/30 ${draft ? 'border-dashed border-border' : ready ? 'border-warning bg-warning/10' : 'border-border bg-bg'}`}
    >
      <span className="text-xs text-muted">T{n.id}</span>
      <span className="line-clamp-2 shrink-0 text-sm font-semibold text-fg">{n.title}</span>
      <span className="flex items-center gap-1.5 text-xs text-fg">
        <Avatar name={who} mine={false} />
        {who}
      </span>
      {!draft && (
        <span className={`mt-auto flex items-center gap-1 text-xs ${ready ? 'text-warning' : 'text-muted'}`}>
          <s.icon size={12} />
          {word}
        </span>
      )}
    </button>
  );
}

/** The Run window: each draft, who gets it and when it starts; Run N tasks queues them. */
function run(parent: number, drafts: Task[], open: Set<number>) {
  void openModal<boolean>((close) => <RunWindow parent={parent} drafts={drafts} open={open} close={close} />);
}

function RunWindow({ parent, drafts, open, close }: { parent: number; drafts: Task[]; open: Set<number>; close: (v?: boolean) => void }) {
  const nameOf = useNameOf();
  const queue = useWrite('task.queue');
  const comment = useWrite('task.comment.add', String(parent));
  const now = drafts.filter((d) => !startsAfter(d, open).length).length;
  const go = () =>
    queue.mutate(
      { tasks: drafts.map((d) => d.id) },
      {
        onSuccess: () => {
          comment.mutate(
            { task: parent, text: `Ran ${drafts.length} tasks: ${drafts.map((d) => `T${d.id}`).join(', ')}` },
            { onError: sayRefusal },
          );
          close(true);
        },
        onError: sayRefusal,
      },
    );
  return (
    <ModalFrame
      title="Run this plan"
      size="xl"
      onClose={() => close()}
      footer={
        <>
          <span className="mr-auto self-center text-sm text-muted">
            {now} start now, {drafts.length - now} wait.
          </span>
          <Button onClick={() => close()}>Cancel</Button>
          <Button variant="primary" autoFocus disabled={queue.isPending} onClick={go}>
            <Play size={14} className="mr-1.5" />
            Run {drafts.length} tasks
          </Button>
        </>
      }
    >
      <table className="w-full text-sm">
        <thead className="text-left text-xs text-muted">
          <tr className="border-b border-border">
            <th className="py-2 font-normal" />
            <th className="py-2 font-normal">Task</th>
            <th className="py-2 font-normal">Goes to</th>
            <th className="py-2 font-normal">Starts</th>
          </tr>
        </thead>
        <tbody>
          {drafts.map((d) => {
            const after = startsAfter(d, open);
            return (
              <tr key={d.id} className="border-b border-border">
                <td className="py-2 pr-3 text-muted">T{d.id}</td>
                <td className="py-2 pr-3">{d.title}</td>
                <td className="py-2 pr-3">{nameOf(d.to)}</td>
                <td className={`py-2 ${after.length ? 'text-muted' : ''}`}>
                  {after.length ? `after ${after.map((x) => `T${x}`).join(', ')}` : 'now'}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </ModalFrame>
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
  const [room, setRoom] = useState({ w: 0, h: 0 });
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const o = new ResizeObserver(() => setRoom({ w: el.clientWidth - 48, h: el.clientHeight - 48 }));
    o.observe(el);
    return () => o.disconnect();
  }, []);
  const fit = (w: number, h: number) => (room.w > 0 && w > 0 ? Math.min(1, room.w / w, room.h / h) : 1);
  return [ref, fit] as const;
}
