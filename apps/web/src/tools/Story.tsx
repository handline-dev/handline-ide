// A task's sub-tasks: the Plan box on its page (PlanFlow), and the graph with Run in a tab of its own (PlanTab).
import { taskGraph, type GraphNode } from '@handline/app-core';
import { hooks, useNameOf, useTasks, useWrite } from '@handline/app-data';
import { useMe } from '@handline/app-react';
import type { Task, TaskDetail } from '@handline/client';
import { Play } from 'lucide-react';
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useTabState } from '#src/persist/tabState.ts';
import { planOf, taskOf } from '#src/layout/kinds.tsx';
import { useNewTab, useOpenable } from '#src/layout/openable.tsx';
import { TaskChat } from '#src/tools/TaskTab.tsx';
import { SubTask } from '#src/tools/StorySub.tsx';
import { startsAfter, wordOf, type Sub, type Word } from '#src/tools/storyWords.ts';
import { Button } from '#src/ui/Button.tsx';
import { ModalFrame, openModal } from '#src/ui/modal.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { sayRefusal } from '#src/ui/refusal.tsx';
import { look } from '#src/ui/taskLook.tsx';
import { Avatar } from '#src/tools/TaskEvents.tsx';

const BOX = { w: 200, h: 128 };

/** A task's sub-tasks as a graph in a tab of its own (plan:<id>), opened from the task's Plan box; a node opens that sub-task beside
 * its own chat. */
export function PlanTab({ id }: { id: number }) {
  const t = hooks.useTaskGet({ task: id }).data;
  const nameOf = useNameOf();
  const [picked, setPicked] = useTabState<number | null>('sub', null);
  if (!t) return <div className="flex-1" />;
  const sub = t.subtasks.find((x) => x.id === picked) ?? null;
  return (
    <div className="flex min-h-0 min-w-0 flex-1">
      {sub && (
        <div className="flex min-h-0 min-w-0 basis-2/5 flex-col border-r border-border">
          <div className="flex h-12 shrink-0 items-center gap-2 border-b border-border px-4 text-sm font-semibold">
            <Avatar name={nameOf(sub.owner)} mine={false} />
            {nameOf(sub.owner)}
            <span className="font-normal text-muted">on T{sub.id}</span>
          </div>
          <TaskChat key={sub.id} id={sub.id} />
        </div>
      )}
      {sub ? <SubTask key={sub.id} sub={sub} story={t} onPick={setPicked} /> : <Plan parent={t.id} subs={t.subtasks} onPick={setPicked} />}
    </div>
  );
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
      </p>
      <div className="flex h-2 gap-0.5 overflow-hidden rounded-full">
        {bar.map(([n, tone]) => n > 0 && <span key={tone} className={tone} style={{ flex: n }} />)}
      </div>
      <div className="grid gap-y-2" style={{ gridTemplateColumns: '1fr 3rem 1fr 3rem 1fr' }}>
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

function Plan({ parent, subs, onPick }: { parent: number; subs: Sub[]; onPick: (id: number) => void }) {
  const me = useMe();
  const kids = [...(useTasks({ parent, all: true }).data?.items ?? [])].sort((a, b) => a.id - b.id);
  const [box, fit] = useFit();
  const g = taskGraph(kids, { me, closed: true, size: BOX });
  const k = fit(g.width, g.height);
  const byId = new Map(kids.map((x) => [x.id, x]));
  // Only a sub-task not yet done or discarded holds another back.
  const open = new Set(kids.filter((x) => !['done', 'discarded'].includes(x.state)).map((x) => x.id));
  const drafts = kids.filter((x) => x.state === 'draft');
  const subOf = new Map(subs.map((x) => [x.id, x]));
  const word = (x: Task) => wordOf(x, subOf.get(x.id), me, open);
  const ready = (x: Task) => word(x).kind === 'you';
  const waiting = (x: Task) => word(x).kind === 'waiting';
  const tally = (label: string, f: (x: Task) => boolean) => {
    const n = kids.filter(f).length;
    return n ? `${n} ${label}` : null;
  };
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
              <Node key={n.id} n={n} t={byId.get(n.id)!} word={word(byId.get(n.id)!)} onPick={onPick} />
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

function Node({ n, t, word, onPick }: { n: GraphNode; t: Task; word: Word; onPick: (id: number) => void }) {
  const who = useNameOf()(t.to);
  const s = look(t);
  const draft = t.state === 'draft';
  const ready = word.kind === 'you';
  return (
    <button
      type="button"
      onClick={() => onPick(n.id)}
      style={{ left: n.x, top: n.y, width: n.w, height: n.h }}
      className={`absolute flex flex-col gap-1 rounded-md border p-3 text-left hover:bg-hover/30 ${draft ? 'border-dashed' : ''} ${ready ? 'border-warning bg-warning/10' : 'border-border bg-bg'}`}
    >
      <span className="text-xs text-muted">T{n.id}</span>
      <span className="line-clamp-2 shrink-0 text-sm font-semibold text-fg">{n.title}</span>
      <span className="flex items-center gap-1.5 text-xs text-fg">
        <Avatar name={who} mine={false} />
        {who}
      </span>
      <span className={`mt-auto flex items-center gap-1 text-xs ${ready ? 'text-warning' : 'text-muted'}`}>
        {!draft && <s.icon size={12} />}
        {word.text}
      </span>
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
