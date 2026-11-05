// The time box's clock, drawn in the Time box row: how much is left before the length (which stays the editable value), and under it a bar
// of the length used and, when the clock stood still (waiting for the founder, blocked, at a limit), that time, which the box does not
// count. A click on it shows where all the time went, by kind.
import { span, timeView, type TimeLabel } from '@handline/app-core';
import type { TaskDetail, TimeSegment } from '@handline/client';
import { useTaskTime } from '@handline/app-data';
import { useNow } from '#src/format/time.ts';
import { Popover } from '#src/ui/Popover.tsx';
import { useState } from 'react';

/** < 1m, 4m, 58m, 1h 40m. */
const length = (seconds: number) => (seconds < 60 ? '< 1m' : span(seconds * 1000).replace(/ ([dhm])\b/g, '$1'));

/** The stretches the time box does not count, in the person's words. */
const PAUSED: Partial<Record<TimeLabel, string>> = {
  founder: 'waiting for you',
  task: 'blocked',
  agent: 'waiting on an agent',
  account: 'at a usage limit',
  waiting: 'waiting',
};

/** What is left of the box now (negative once over), and its length; null for a closed task, which has no clock. */
export function useLeft(t: TaskDetail): { left: number; total: number } | null {
  const now = useNow(30_000);
  const c = t.time;
  if (!c) return null;
  // While it runs the clock ends at `end`; paused, left_s holds what it had left.
  return { left: c.left_s ?? (c.end ? (Date.parse(c.end) - now) / 1000 : c.length_s), total: c.length_s };
}

/** The words before the length: "1h 15m left of", or "Over by 20m of" in red. */
export function LeftWords({ left }: { left: number }) {
  return left < 0 ? (
    <span className="font-medium text-danger">Over by {length(-left)} of</span>
  ) : (
    <span>
      <span className="font-medium text-fg">{length(left)} left</span> <span className="text-muted">of</span>
    </span>
  );
}

/** Who held the task at each moment, from its handoffs and assignee changes: before the first change, that change's `from`; after, its `to`. */
function holders(t: TaskDetail): { at: number; who: string }[] {
  const changes = [
    ...t.handoffs.map((h) => ({ at: Date.parse(h.at), from: h.from, to: h.to })),
    ...(t.assignments ?? []).filter((a) => a.what === 'assignee').map((a) => ({ at: Date.parse(a.at), from: a.from, to: a.to })),
  ].sort((a, b) => a.at - b.at);
  if (!changes.length) return [{ at: -Infinity, who: t.to }];
  return [{ at: -Infinity, who: changes[0]!.from || t.to }, ...changes.map((c) => ({ at: c.at, who: c.to }))];
}

/** The box's clock time in the order it ran: each working stretch as its holder's, waking up as itself, adjacent pieces of one key joined. */
function inOrder(t: TaskDetail, segments: TimeSegment[]): { key: string; seconds: number }[] {
  const hs = holders(t);
  const out: { key: string; seconds: number }[] = [];
  const push = (key: string, seconds: number) => {
    const last = out.at(-1);
    if (last?.key === key) last.seconds += seconds;
    else if (seconds > 0) out.push({ key, seconds });
  };
  for (const s of [...segments].sort((a, b) => a.from.localeCompare(b.from))) {
    if (s.kind === 'starting') push('starting', s.seconds);
    if (s.kind !== 'working') continue;
    const from = Date.parse(s.from);
    const to = Date.parse(s.to);
    hs.forEach((h, i) => {
      const a = Math.max(from, h.at);
      const b = Math.min(to, hs[i + 1]?.at ?? Infinity);
      if (b > a) push(`agent:${h.who}`, (b - a) / 1000);
    });
  }
  return out;
}

/** The worked seconds per agent, most first: each working stretch split where the holder changed. */
function workedBy(t: TaskDetail, segments: TimeSegment[]): { who: string; seconds: number }[] {
  const hs = holders(t);
  const by = new Map<string, number>();
  for (const s of segments) {
    if (s.kind !== 'working') continue;
    const from = Date.parse(s.from);
    const to = Date.parse(s.to);
    hs.forEach((h, i) => {
      const a = Math.max(from, h.at);
      const b = Math.min(to, hs[i + 1]?.at ?? Infinity);
      if (b > a) by.set(h.who, (by.get(h.who) ?? 0) + (b - a) / 1000);
    });
  }
  return [...by].map(([who, seconds]) => ({ who, seconds })).sort((a, b) => b.seconds - a.seconds);
}

/** The agents' colours, in order of who spent the most. */
const SERIES = ['bg-series-1', 'bg-series-2', 'bg-series-3', 'bg-series-4', 'bg-series-5', 'bg-series-6'];

/** Each kind of time's colour, the same in the gauge and its list. */
const TONE: Record<TimeLabel, string> = {
  working: 'bg-success',
  founder: 'bg-warning',
  task: 'bg-danger',
  agent: 'bg-accent',
  account: 'bg-danger',
  waiting: 'bg-muted',
  queued: 'bg-border',
  starting: 'bg-border',
};
const WORD: Record<TimeLabel, string> = {
  working: 'Worked',
  founder: 'Waiting for you',
  task: 'Blocked',
  agent: 'Waiting on an agent',
  account: 'At a usage limit',
  waiting: 'Waiting',
  queued: 'Queued',
  starting: 'Waking up',
};

/** The bar of the box used, and the time the clock stood still. A click opens the box in parts: each agent's time holding it in its colour,
 * waking up, and what is left; hovering a part lights it in the gauge. */
export function ClockBar({ t, left, total }: { t: TaskDetail; left: number; total: number }) {
  const segments = useTaskTime(t.id).data?.segments ?? [];
  const { totals } = timeView(segments);
  const paused = totals.filter((x) => PAUSED[x.label]);
  const still = paused.reduce((n, x) => n + x.seconds, 0);
  const usedS = total - left;
  // The gauge is the box: what ran its clock, then what is left. The clock runs while an agent holds the task in doing, working on it or
  // not, so that time is the holder's: what no stretch accounts for goes to the agent holding it now.
  const by = new Map(workedBy(t, segments).map((w) => [w.who, w.seconds]));
  const waking = totals.find((x) => x.label === 'starting')?.seconds ?? 0;
  const idle = usedS - waking - [...by.values()].reduce((n, x) => n + x, 0);
  if (idle > 0) by.set(t.to, (by.get(t.to) ?? 0) + idle);
  // Most first, waking up among them by its size: what took the box the most leads.
  const counted = [
    ...[...by]
      .sort((a, b) => b[1] - a[1])
      .map(([who, seconds], i) => ({ key: `agent:${who}`, word: who, seconds, tone: SERIES[i % SERIES.length]! })),
    ...(waking > 0 ? [{ key: 'starting', word: WORD.starting, seconds: waking, tone: TONE.starting }] : []),
  ].sort((a, b) => b.seconds - a.seconds);
  const rest = left > 0 ? [{ key: 'left', word: 'Left', seconds: left, tone: 'bg-border' }] : [];
  const shown = [...counted, ...rest];
  // The gauge in time order (the list is by size): the stretches as they ran, then the held time no stretch accounts for, which is the
  // holder's now and most of it recent, then what is left; each piece keeps its part's colour.
  const look = new Map(counted.map((x) => [x.key, x]));
  const timeline = [...inOrder(t, segments), ...(idle > 0 ? [{ key: `agent:${t.to}`, seconds: idle }] : [])].reduce<
    { key: string; seconds: number }[]
  >((out, p) => {
    const last = out.at(-1);
    if (last?.key === p.key) last.seconds += p.seconds;
    else out.push({ ...p });
    return out;
  }, []);
  const gauge = [
    ...timeline.map((p, i) => ({ ...p, n: i, tone: look.get(p.key)?.tone ?? 'bg-muted' })),
    ...rest.map((x) => ({ ...x, n: -1 })),
  ];
  const whole = shown.reduce((n, x) => n + x.seconds, 0);
  const [hot, setHot] = useState<string | null>(null);
  return (
    <Popover
      onOpenChange={() => setHot(null)}
      trigger={
        <button type="button" className="-mx-2 flex max-w-72 flex-col gap-1 rounded-md px-2 py-1 text-left hover:bg-hover/30">
          {/* The same gauge as inside, smaller. */}
          <Gauge parts={gauge} whole={whole} className="h-1.5" />
          {still > 0 && (
            <span className="text-xs text-muted">
              {/* One kind says itself; several get their total first. */}
              {paused.length === 1
                ? `Paused, ${PAUSED[paused[0]!.label]} ${length(still)}`
                : `Paused ${length(still)} · ${paused.map((x) => `${PAUSED[x.label]} ${length(x.seconds)}`).join(' · ')}`}
            </span>
          )}
        </button>
      }
    >
      <div className="flex w-72 flex-col gap-3">
        <div className="flex items-baseline justify-between">
          <span className="font-medium">
            {length(Math.max(0, usedS))} <span className="font-normal text-muted">of {length(total)}</span>
          </span>
          <span className={left < 0 ? 'text-danger' : 'text-muted'}>{left < 0 ? `Over by ${length(-left)}` : `${length(left)} left`}</span>
        </div>
        <Gauge parts={gauge} whole={whole} hot={hot} className="h-2.5" />
        <ul className="flex flex-col">
          {shown.map((x) => (
            <li
              key={x.key}
              onMouseEnter={() => setHot(x.key)}
              onMouseLeave={() => setHot(null)}
              className="flex h-7 items-center gap-2 rounded-sm px-1 hover:bg-hover/30"
            >
              <span className={`size-2 shrink-0 rounded-sm ${x.tone}`} />
              <span className="min-w-0 flex-1 truncate">{x.word}</span>
              <span className="text-muted">{length(x.seconds)}</span>
              <span className="w-9 text-right text-xs text-muted">{whole ? Math.round((x.seconds / whole) * 100) : 0}%</span>
            </li>
          ))}
        </ul>
        {/* Waits that stopped the clock: not in the box, said under it. */}
        {paused.length > 0 && (
          <p className="text-xs text-muted">Not counted: {paused.map((x) => `${PAUSED[x.label]} ${length(x.seconds)}`).join(' · ')}</p>
        )}
      </div>
    </Popover>
  );
}

/** The box as one bar: its pieces in time order, each its part's colour; with `hot`, the other parts dim. */
function Gauge({
  parts,
  whole,
  hot = null,
  className,
}: {
  parts: { key: string; n: number; seconds: number; tone: string }[];
  whole: number;
  hot?: string | null;
  className: string;
}) {
  return (
    <span
      className={`flex w-full gap-px overflow-hidden rounded-full bg-surface ${className}`}
      role="img"
      aria-label="Where the time box went"
    >
      {parts.map((x) => (
        <span
          key={`${x.key}:${x.n}`}
          className={`${x.tone} transition-opacity ${hot && hot !== x.key ? 'opacity-25' : ''}`}
          style={{ width: `${whole ? (x.seconds / whole) * 100 : 0}%` }}
        />
      ))}
    </span>
  );
}
