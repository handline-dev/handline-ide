// An opened account's last days: its 5-hour and week usage on one graph, the 100% limit dashed and each week reset marked, from the
// daemon's per-account samples (stats.outlook by_account).
import { usedPath } from '@handline/app-core';
import { useOutlook } from '@handline/app-data';

const W = 560;
const H = 80;

export function AccountHistory({ account }: { account: string }) {
  const o = useOutlook('7d').data;
  const mine = o?.by_account?.find((b) => b.account === account);
  if (!mine?.past.length) return null;
  const from = Date.parse(mine.past[0]!.at);
  const to = Date.parse(mine.past.at(-1)!.at);
  const x = (at: string) => ((Date.parse(at) - from) / Math.max(1, to - from)) * W;
  const five = usedPath(
    mine.past.map((p) => ({ at: p.at, left: p.five_left })),
    from,
    to,
    W,
    H,
  );
  const week = usedPath(
    mine.past.map((p) => ({ at: p.at, left: p.week_left })),
    from,
    to,
    W,
    H,
  );
  const resets = (mine.resets ?? []).filter((r) => r.window === '7d' && Date.parse(r.at) >= from && Date.parse(r.at) <= to);
  return (
    <figure className="flex flex-col gap-1">
      <figcaption className="flex items-center gap-4 text-xs text-muted">
        <span>Last 7 days</span>
        <span className="flex items-center gap-1">
          <span className="h-0.5 w-3 bg-accent" />
          5-hour
        </span>
        <span className="flex items-center gap-1">
          <span className="h-0.5 w-3 bg-warning" />
          Week
        </span>
      </figcaption>
      <svg
        viewBox={`0 0 ${W} ${H}`}
        preserveAspectRatio="none"
        className="h-20 w-full overflow-visible"
        role="img"
        aria-label="5-hour and week usage, last 7 days"
      >
        <line
          x1="0"
          x2={W}
          y1="0"
          y2="0"
          stroke="currentColor"
          strokeDasharray="4 4"
          className="text-border"
          vectorEffect="non-scaling-stroke"
        />
        {resets.map((r) => (
          <line
            key={r.at}
            x1={x(r.at)}
            x2={x(r.at)}
            y1="0"
            y2={H}
            stroke="currentColor"
            strokeDasharray="2 3"
            className="text-border"
            vectorEffect="non-scaling-stroke"
          />
        ))}
        <path d={five} fill="none" stroke="currentColor" strokeWidth="1.5" className="text-accent" vectorEffect="non-scaling-stroke" />
        <path d={week} fill="none" stroke="currentColor" strokeWidth="1.5" className="text-warning" vectorEffect="non-scaling-stroke" />
      </svg>
    </figure>
  );
}
