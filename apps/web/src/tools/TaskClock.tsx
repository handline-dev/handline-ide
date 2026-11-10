// The time box's clock, drawn in the Time box row: how much is left before the length (which stays the editable value), and under it a bar
// of the length used and, when the clock stood still (waiting for the founder, blocked, at a limit), that time, which the box does not
// count. A click shows the box in parts.
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

/** The agents' colours, in order of who spent the most. */
const SERIES = ['bg-series-1', 'bg-series-2', 'bg-series-3', 'bg-series-4', 'bg-series-5', 'bg-series-6'];
/** Waking up is time spent, so it is filled, in a grey of its own; what is left is the empty track. */
const WAKING = 'bg-muted';
const LEFT = 'bg-transparent';

/** The whole task's time, apart from the box: "Worked 23m · waiting for you 5h01m". Waking up counts as worked. */
function wholeLine(segments: TimeSegment[]): string {
  const { totals } = timeView(segments);
  const worked = totals.filter((x) => x.label === 'working' || x.label === 'starting').reduce((n, x) => n + x.seconds, 0);
  return [
    worked > 0 && `Worked ${length(worked)}`,
    ...totals.filter((x) => PAUSED[x.label]).map((x) => `${PAUSED[x.label]} ${length(x.seconds)}`),
  ]
    .filter(Boolean)
    .join(' · ');
}

/** A closed task has no box: the Time box row says where its whole time went. */
export function WholeTime({ task }: { task: number }) {
  const line = wholeLine(useTaskTime(task).data?.segments ?? []);
  return line ? <span className="text-xs text-muted">{line}</span> : null;
}

/** The bar of the box used, and the time the clock stood still. A click opens task.time's box in parts: each agent's time in its colour,
 * waking up, and what is left; hovering a part lights it in the gauge. Under it, the whole task's time in one line. */
export function ClockBar({ t, left, total }: { t: TaskDetail; left: number; total: number }) {
  const read = useTaskTime(t.id);
  const time = read.data;
  const segments = time?.segments ?? [];
  const box = time?.box;
  const paused = timeView(segments).totals.filter((x) => PAUSED[x.label]);
  const still = paused.reduce((n, x) => n + x.seconds, 0);
  // The box's own numbers once read, so the parts add up to the length; until then the ticking clock, its used time as one part.
  const boxLeft = box?.left_s ?? left;
  const boxLength = box?.length_s ?? total;
  const parts = [
    ...(box ? [] : [{ key: 'used', word: 'Used', seconds: Math.max(0, total - left), tone: SERIES[0]! }]),
    ...(box?.worked ?? []).map((w, i) => ({
      key: `agent:${w.agent}`,
      word: w.agent,
      seconds: w.seconds,
      tone: SERIES[i % SERIES.length]!,
    })),
    ...(box && box.waking_s > 0 ? [{ key: 'starting', word: 'Waking up', seconds: box.waking_s, tone: WAKING }] : []),
    ...(boxLeft > 0 ? [{ key: 'left', word: 'Left', seconds: boxLeft, tone: LEFT }] : []),
  ];
  const whole = parts.reduce((n, x) => n + x.seconds, 0);
  const all = wholeLine(segments);
  const [hot, setHot] = useState<string | null>(null);
  return (
    <Popover
      onOpenChange={(o) => {
        setHot(null);
        // Read the box again on opening, so it agrees with the ticking row above.
        if (o) void read.refetch();
      }}
      trigger={
        <button type="button" className="-mx-2 flex max-w-72 flex-col gap-1 rounded-md px-2 py-1 text-left hover:bg-hover/30">
          {/* The same gauge as inside, smaller. */}
          <Gauge parts={parts} whole={whole} className="h-1.5" />
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
            {length(Math.max(0, boxLength - boxLeft))} <span className="font-normal text-muted">of {length(boxLength)}</span>
          </span>
          <span className={boxLeft < 0 ? 'text-danger' : 'text-muted'}>
            {boxLeft < 0 ? `Over by ${length(-boxLeft)}` : `${length(boxLeft)} left`}
          </span>
        </div>
        <Gauge parts={parts} whole={whole} hot={hot} className="h-2.5" />
        <ul className="flex flex-col">
          {parts.map((x) => (
            <li
              key={x.key}
              onMouseEnter={() => setHot(x.key)}
              onMouseLeave={() => setHot(null)}
              className="flex h-7 items-center gap-2 rounded-sm px-1 hover:bg-hover/30"
            >
              <span className={`size-2 shrink-0 rounded-sm ${x.tone === LEFT ? 'border border-muted' : x.tone}`} />
              <span className="min-w-0 flex-1 truncate">{x.word}</span>
              <span className="text-muted">{length(x.seconds)}</span>
              <span className="w-9 text-right text-xs text-muted">{whole ? Math.round((x.seconds / whole) * 100) : 0}%</span>
            </li>
          ))}
        </ul>
        {all && <p className="border-t border-border pt-2 text-xs text-muted">Whole task: {all}</p>}
      </div>
    </Popover>
  );
}

/** The box as one bar: each part in its colour, what is left the empty track; with `hot`, the other parts dim. */
function Gauge({
  parts,
  whole,
  hot = null,
  className,
}: {
  parts: { key: string; seconds: number; tone: string }[];
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
          key={x.key}
          className={`${x.tone} transition-opacity ${hot && hot !== x.key ? 'opacity-25' : ''}`}
          style={{ width: `${whole ? (x.seconds / whole) * 100 : 0}%` }}
        />
      ))}
    </span>
  );
}
