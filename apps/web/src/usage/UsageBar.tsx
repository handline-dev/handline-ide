import type { Window } from '@handline/client';
import { useNow } from '#src/format/time.ts';

/** How long until `iso`, short: 45m, 3h 10m, 2d 4h. */
export function until(iso: string | null, now = Date.now()): string | null {
  if (!iso) return null;
  const m = Math.max(0, Math.round((Date.parse(iso) - now) / 60_000));
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ${m % 60}m`;
  return `${Math.floor(h / 24)}d ${h % 24}h`;
}

/** Past this much of a window used, it reads as a warning. */
export const HIGH = 80;

export const FIVE_HOURS = 5 * 3600_000;
export const WEEK = 7 * 86400_000;

/** One usage window as a bar: its name, how much is used, and when it resets. A thin mark on it is where now stands in the
 * window (`span` long, ending at its reset): use left of the mark is ahead of time, right of it is room. */
export function UsageBar({ name, w, span }: { name: string; w: Window | null; span: number }) {
  const pct = w ? Math.round(w.used_pct) : null;
  const high = pct != null && pct >= HIGH;
  const clock = useNow();
  const resets = until(w?.resets_at ?? null, clock);
  const left = w?.resets_at ? Date.parse(w.resets_at) - clock : null;
  const now = left == null ? null : Math.min(100, Math.max(0, (1 - left / span) * 100));
  return (
    <div className="flex flex-col gap-1">
      <div className="flex justify-between gap-6 text-xs">
        <span className="text-muted">{name}</span>
        <span className="text-fg tabular-nums">
          {pct == null ? 'no reading' : `${pct}%`}
          {resets && <span className="text-muted">{` · resets in ${resets}`}</span>}
        </span>
      </div>
      <div className="relative h-1.5 rounded-full bg-border">
        <div className={`h-full rounded-full ${high ? 'bg-warning' : 'bg-accent'}`} style={{ width: `${Math.min(100, pct ?? 0)}%` }} />
        {now != null && (
          <div
            className="absolute -top-0.5 -bottom-0.5 w-px bg-fg"
            style={{ left: `${now}%` }}
            title={`Now: ${Math.round(now)}% of the window gone`}
          />
        )}
      </div>
    </div>
  );
}
