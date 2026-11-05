// The head of Usage, as the legacy page had it: "Week 43% left" and the accounts' week left together, recorded to now and guessed after
// it (dashed), each account's weekly reset under the axis, and the part lost at the last reset as a bar. Moving over the plot reads a point.
// Every number is the daemon's (stats.outlook); the drawing is the legacy one in this app's tokens.
import { axisTicks, dateTime, guessLine, lostAtLastReset, runwayWords, weekResets } from '@handline/app-core';
import type { Outlook as Data } from '@handline/client';
import { useOutlook } from '@handline/app-data';
import { RotateCcw } from 'lucide-react';
import { useState, type PointerEvent } from 'react';
import { usePersisted } from '#src/persist/kv.ts';

type Range = Data['range'];
const RANGES: Range[] = ['12h', '24h', '3d', '7d', '14d'];
const W = 560;
const H = 160;

type Labels = Record<string, string>;
const named = (l: Labels, a: string) => l[a] ?? a;
/** "Fri 09:00". */
const resetAt = (at: string) => new Date(at).toLocaleString([], { weekday: 'short', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' });

/** The guess ends at the last guessed reset: past it the daemon's line goes on as if the week did not reset. */
function clipAtReset(o: Data): Data {
  const end = lostAtLastReset(o);
  return end ? { ...o, future: o.future.filter((p) => Date.parse(p.at) <= Date.parse(end.at)) } : o;
}

export function Outlook({ labels }: { labels: Labels }) {
  const [range, setRange] = usePersisted<Range>('usage:range', '7d');
  const q = useOutlook(range);
  const o = q.data;
  return (
    <section aria-label="Week left" className="flex flex-col gap-3">
      <div className="flex flex-wrap items-baseline gap-3">
        <div className="min-w-0 flex-1">{o && <Readout o={o} labels={labels} />}</div>
        <div role="radiogroup" aria-label="Range" className="flex gap-1 rounded-lg bg-surface p-1">
          {RANGES.map((r) => (
            <button
              key={r}
              type="button"
              role="radio"
              aria-checked={r === range}
              onClick={() => setRange(r)}
              className="h-6 rounded-sm px-2 text-xs text-muted hover:text-fg aria-checked:bg-bg aria-checked:text-fg"
            >
              {r}
            </button>
          ))}
        </div>
      </div>
      {q.isError && <p className="text-sm text-danger">The outlook did not load.</p>}
      {!o && !q.isError && <div className="h-60 animate-pulse rounded-md bg-surface" />}
      {o && <Plot o={o} labels={labels} />}
    </section>
  );
}

/** "Week 43% left" and what the pace loses at the last reset, or when an account runs out. */
function Readout({ o, labels }: { o: Data; labels: Labels }) {
  const left = o.past.filter((p) => p.left_pct !== null).at(-1)?.left_pct ?? null;
  const lost = lostAtLastReset(o);
  const soonest = o.runway.filter((r) => r.runs_out_in_seconds !== null).sort((a, b) => a.runs_out_in_seconds! - b.runs_out_in_seconds!)[0];
  const pace = lost
    ? `at this pace about ${Math.round(lost.pct)}% of ${named(labels, lost.account)}'s week goes unused`
    : soonest
      ? `at this pace ${named(labels, soonest.account)} ${runwayWords(soonest)}`
      : '';
  return (
    <p className="flex flex-wrap items-baseline gap-x-2">
      <strong className="text-xl font-semibold text-fg tabular-nums">
        {left === null ? 'Week: no reading' : `Week ${Math.round(left)}% left`}
      </strong>
      {pace && <span className="text-sm text-muted">· {pace}</span>}
    </p>
  );
}

function Plot({ o: full, labels }: { o: Data; labels: Labels }) {
  const o = clipAtReset(full);
  const all = [...o.past, ...o.future].filter((p) => p.left_pct !== null);
  const t0 = Date.parse((all[0] ?? { at: o.now }).at);
  const t1 = Date.parse((all.at(-1) ?? { at: o.now }).at);
  const x = (at: string) => (t1 === t0 ? 0 : ((Date.parse(at) - t0) / (t1 - t0)) * W);
  const pct = (at: string) => (t1 === t0 ? 0 : ((Date.parse(at) - t0) / (t1 - t0)) * 100);
  const y = (p: number) => H - (p / 100) * H;
  const [cursor, setCursor] = useState<number | null>(null);
  const points = [...o.past.map((p) => ({ ...p, guess: false })), ...o.future.map((p) => ({ ...p, guess: true }))].filter(
    (p) => p.left_pct !== null,
  );
  const near =
    cursor === null || !points.length ? null : points.reduce((b, p) => (Math.abs(x(p.at) - cursor) < Math.abs(x(b.at) - cursor) ? p : b));
  const resets = weekResets(o).filter((r) => Date.parse(r.at) >= t0 && Date.parse(r.at) <= t1);
  const lost = lostAtLastReset(full);
  const path = (pts: Data['past']) =>
    pts
      .filter((p) => p.left_pct !== null)
      .map((p, i) => `${i ? 'L' : 'M'}${x(p.at).toFixed(1)},${y(p.left_pct!).toFixed(1)}`)
      .join(' ');
  const scrub = (e: PointerEvent<HTMLDivElement>) => {
    const r = e.currentTarget.getBoundingClientRect();
    setCursor(Math.min(W, Math.max(0, ((e.clientX - r.left) / (r.width || 1)) * W)));
  };
  const thin = { vectorEffect: 'non-scaling-stroke' } as const;
  // Resets near the pointer (within 2% of the span) are read with the point: which accounts reset then, and when.
  const nearResets =
    cursor === null ? [] : resets.filter((r) => Math.abs(x(r.at) - cursor) <= W * 0.02).sort((a, b) => a.at.localeCompare(b.at));
  return (
    <div className="flex flex-col">
      <div className="relative pl-10">
        <div className="relative" onPointerMove={scrub} onPointerDown={scrub} onPointerLeave={() => setCursor(null)}>
          <svg
            viewBox={`0 0 ${W} ${H}`}
            preserveAspectRatio="none"
            className="h-60 w-full overflow-visible"
            role="img"
            aria-label="Week left, recorded and guessed"
          >
            {[0, 50, 100].map((g) => (
              <line key={g} x1={0} x2={W} y1={y(g)} y2={y(g)} stroke="var(--border)" style={thin} />
            ))}
            {resets.map((r) => (
              <line
                key={`${r.account}${r.at}`}
                x1={x(r.at)}
                x2={x(r.at)}
                y1={0}
                y2={H}
                stroke="var(--muted)"
                strokeDasharray="1 3"
                style={thin}
              />
            ))}
            <path d={path(o.past)} fill="none" stroke="var(--accent)" strokeWidth={2} style={thin} />
            <path d={path(guessLine(o))} fill="none" stroke="var(--accent)" strokeWidth={2} strokeDasharray="5 4" style={thin} />
            <line x1={x(o.now)} x2={x(o.now)} y1={0} y2={H} stroke="var(--fg)" strokeWidth={2} style={thin} />
            {near && <line x1={x(near.at)} x2={x(near.at)} y1={0} y2={H} stroke="var(--muted)" style={thin} />}
          </svg>
          <span className="absolute -top-5 -translate-x-1/2 text-xs text-muted" style={{ left: `${pct(o.now)}%` }}>
            now
          </span>
          {/* The point under the pointer, read beside it at its height: no room is kept for it under the plot. */}
          {near && (
            <span
              aria-live="polite"
              className="pointer-events-none absolute z-10 rounded-md border border-border bg-bg px-2 py-1 text-xs whitespace-nowrap text-fg shadow-popover"
              style={{
                left: `${(x(near.at) / W) * 100}%`,
                top: `${100 - near.left_pct!}%`,
                transform: `translate(${x(near.at) / W > 0.7 ? 'calc(-100% - 8px)' : '8px'}, -50%)`,
              }}
            >
              <span className="block">
                {dateTime(near.at)} · {Math.round(near.left_pct!)}% left
                {near.guess && <span className="text-muted"> (guess)</span>}
              </span>
              {nearResets.map((r) => (
                <span key={`${r.account}${r.at}`} className="flex items-center gap-1 text-muted">
                  <RotateCcw size={11} className="shrink-0" />
                  {named(labels, r.account)} resets {resetAt(r.at)}
                </span>
              ))}
            </span>
          )}
          {[0, 50, 100].map((g) => (
            <span
              key={g}
              className="absolute -left-10 w-8 -translate-y-1/2 text-right text-xs text-muted tabular-nums"
              style={{ top: `${100 - g}%` }}
            >
              {g}%
            </span>
          ))}
          {lost && Date.parse(lost.at) >= t0 && Date.parse(lost.at) <= t1 && (
            <>
              <span
                className="absolute bottom-px w-3 -translate-x-full bg-accent"
                style={{ left: `${pct(lost.at)}%`, height: `${lost.pct}%` }}
              />
              <span
                className="absolute -translate-y-full pr-4 text-right text-xs whitespace-nowrap text-accent"
                style={{ right: `${100 - pct(lost.at)}%`, bottom: `${lost.pct}%` }}
                title={`${named(labels, lost.account)}: ${Math.round(lost.pct)}% of the week is lost at the reset ${dateTime(lost.at)} at this pace`}
              >
                {Math.round(lost.pct)}% lost
              </span>
            </>
          )}
        </div>
        {/* Under the plot: the day ticks, and each account's weekly reset with its time. */}
        <div className="relative mt-2 h-5 text-xs text-muted">
          {axisTicks(t0, t1).map((t) => (
            <span key={t.at} className="absolute -translate-x-1/2" style={{ left: `${((t.at - t0) / (t1 - t0 || 1)) * 100}%` }}>
              {t.label}
            </span>
          ))}
        </div>
      </div>
    </div>
  );
}
