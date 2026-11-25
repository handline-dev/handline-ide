import { useSize } from '#src/ui/useSize.ts';
import { Refs } from '#src/ui/refs.tsx';
import { UnreadDot } from '#src/ui/Dot.tsx';
// Who waits on whom in a workspace: an arrow is a wait (a turn on you, or a task blocked by another's), a dot an unread message to you.
import type { AgentView as Agent, Task } from '@handline/client';
import { Hand, Loader, TriangleAlert } from 'lucide-react';
import { useRef } from 'react';
import { activityOf, holdLine } from '#src/layout/agentList.ts';
import { chatOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';

const W = 224;
const H = 60;
const GAP = 20;

type Node = { key: string; x: number; y: number; agent?: Agent; line: string; mark: 'you' | 'wait' | 'error' | 'work' | null };
type Edge = { from: string; to: string };

/** Each agent's wait: on you (it asks you something in Needs you), on the agents holding the tasks it waits on, or none. */
export function waitsOf(
  agents: readonly Agent[],
  open: readonly Task[],
  asking: ReadonlySet<string>,
): { onYou: Set<string>; edges: Edge[] } {
  const holder = new Map(open.map((t) => [t.id, t.acting ?? t.to]));
  const here = new Set(agents.map((a) => a.key));
  const onYou = new Set(agents.filter((a) => asking.has(a.key) || activityOf(a) === 'you').map((a) => a.key));
  const edges: Edge[] = [...onYou].map((k) => ({ from: k, to: 'you' }));
  for (const a of agents) {
    if (a.status?.reason !== 'task') continue;
    for (const t of a.status.tasks ?? []) {
      const h = holder.get(t);
      if (h && h !== a.key && here.has(h)) edges.push({ from: a.key, to: h });
    }
  }
  return { onYou, edges };
}

function lineOf(a: Agent, onYou: boolean): Pick<Node, 'line' | 'mark'> {
  const task = a.task ? `T${a.task.id}` : null;
  if (onYou) return { line: `${task ? `${task} · ` : ''}waits on you`, mark: 'you' };
  const act = activityOf(a);
  if (a.status?.reason === 'task') return { line: `${task ? `${task} · ` : ''}${holdLine(a)?.toLowerCase() ?? 'waiting'}`, mark: 'wait' };
  if (act === 'error') return { line: holdLine(a) ?? 'Stopped', mark: 'error' };
  if (task && (act === 'working' || act === 'waking')) return { line: task, mark: 'work' };
  return { line: task ?? 'Idle', mark: null };
}

/** Three columns: who waits on you, you (and the stopped under you), everyone else, blocked ones next to the one they wait on. */
function layout(agents: readonly Agent[], onYou: Set<string>, edges: Edge[], width: number): Node[] {
  const left = agents.filter((a) => onYou.has(a.key));
  const stopped = agents.filter((a) => !onYou.has(a.key) && activityOf(a) === 'error' && a.status?.reason !== 'task');
  const rest = agents.filter((a) => !left.includes(a) && !stopped.includes(a));
  // A waiter goes right under what it waits on.
  const right: Agent[] = [];
  const place = (a: Agent) => {
    if (right.includes(a)) return;
    right.push(a);
    for (const e of edges) {
      const waiter = e.to === a.key ? rest.find((r) => r.key === e.from) : undefined;
      if (waiter) place(waiter);
    }
  };
  for (const a of rest) if (!edges.some((e) => e.from === a.key && e.to !== 'you')) place(a);
  for (const a of rest) place(a);
  const cols = width >= 3 * W + 2 * 48 ? [0, (width - W) / 2, width - W] : [0, 0, 0];
  const narrow = cols[2] === 0;
  const node = (a: Agent, x: number, y: number): Node => ({ key: a.key, x, y, agent: a, ...lineOf(a, onYou.has(a.key)) });
  const tall = Math.max(left.length, right.length, 1 + stopped.length);
  const mid = narrow ? 0 : ((tall - 1) * (H + GAP)) / 2 - (stopped.length * (H + GAP)) / 2;
  const nodes: Node[] = [];
  let y = 0;
  const column = (list: Agent[], x: number) => {
    list.forEach((a, i) => nodes.push(node(a, x, narrow ? (y += H + GAP) - H - GAP : i * (H + GAP))));
  };
  column(left, cols[0]);
  const youY = narrow ? (y += H + GAP) - H - GAP : Math.max(0, mid);
  nodes.push({
    key: 'you',
    x: cols[1],
    y: youY,
    line: onYou.size ? `${onYou.size} ${onYou.size === 1 ? 'agent waits' : 'agents wait'} on you` : 'Nobody waits on you',
    mark: null,
  });
  stopped.forEach((a, i) => nodes.push(node(a, cols[1], narrow ? (y += H + GAP) - H - GAP : youY + (i + 1) * (H + GAP))));
  column(right, cols[2]);
  return nodes;
}

function Box({ n }: { n: Node }) {
  const open = useOpenable(n.agent ? chatOf(n.agent.key) : null);
  const border =
    n.key === 'you' ? 'border-fg border-2' : n.mark === 'you' ? 'border-warning' : n.mark === 'error' ? 'border-danger' : 'border-border';
  const Icon = n.mark === 'you' ? Hand : n.mark === 'error' ? TriangleAlert : n.mark === 'work' ? Loader : null;
  const body = (
    <>
      <span className="flex items-center justify-between gap-2 truncate text-fg">
        {n.agent ? n.agent.name : 'You'}
        {!!n.agent?.founder_unread && <UnreadDot label="Unread message to you" />}
      </span>
      <span className="flex items-center gap-1 truncate text-muted">
        {Icon && <Icon size={12} className="shrink-0" />}
        <span className="truncate">
          <Refs text={n.line} />
        </span>
      </span>
    </>
  );
  const cls = `absolute flex flex-col justify-center gap-0.5 rounded-md border bg-bg px-3 text-left text-sm ${border}`;
  const style = { left: n.x, top: n.y, width: W, height: H };
  return n.agent ? (
    <button type="button" {...open} className={`${cls} outline-none focus-visible:ring-2 focus-visible:ring-accent`} style={style}>
      {body}
    </button>
  ) : (
    <div className={cls} style={style}>
      {body}
    </div>
  );
}

export function WorkspaceGraph({ agents, open, asking }: { agents: readonly Agent[]; open: readonly Task[]; asking: ReadonlySet<string> }) {
  const box = useRef<HTMLDivElement>(null);
  const width = useSize(box, (el) => el.clientWidth, 0);
  const { onYou, edges } = waitsOf(agents, open, asking);
  const nodes = width ? layout(agents, onYou, edges, width) : [];
  const at = new Map(nodes.map((n) => [n.key, n]));
  const height = nodes.reduce((m, n) => Math.max(m, n.y + H), 0);
  return (
    <div ref={box} className="relative w-full" style={{ height }}>
      <svg className="pointer-events-none absolute inset-0 overflow-visible" width={width} height={height} aria-hidden>
        <defs>
          <marker id="ws-arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="8" markerHeight="8" orient="auto">
            <path d="M0 0 L8 4 L0 8 z" className="fill-muted" />
          </marker>
          <marker id="ws-arrow-you" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="8" markerHeight="8" orient="auto">
            <path d="M0 0 L8 4 L0 8 z" className="fill-warning" />
          </marker>
        </defs>
        {edges.map((e) => {
          const a = at.get(e.from);
          const b = at.get(e.to);
          if (!a || !b) return null;
          // Side to side between columns; top to bottom within one.
          const same = a.x === b.x;
          const [x1, y1, x2, y2] = same
            ? [a.x + W / 2, a.y < b.y ? a.y + H : a.y, b.x + W / 2, a.y < b.y ? b.y : b.y + H]
            : [a.x < b.x ? a.x + W : a.x, a.y + H / 2, a.x < b.x ? b.x : b.x + W, b.y + H / 2];
          const d = same ? `M${x1} ${y1} L${x2} ${y2}` : `M${x1} ${y1} C${(x1 + x2) / 2} ${y1} ${(x1 + x2) / 2} ${y2} ${x2} ${y2}`;
          return (
            <path
              key={`${e.from}>${e.to}`}
              d={d}
              className={`fill-none ${e.to === 'you' ? 'stroke-warning' : 'stroke-muted'}`}
              strokeWidth={1.5}
              markerEnd={e.to === 'you' ? 'url(#ws-arrow-you)' : 'url(#ws-arrow)'}
            />
          );
        })}
      </svg>
      {nodes.map((n) => (
        <Box key={n.key} n={n} />
      ))}
    </div>
  );
}
