// An opened account's history: its 5-hour usage over the last day and its week usage over 7 days, two graphs side by side, the 100% limit dashed and each week
// reset marked on the week graph, from the daemon's per-account samples (stats.outlook by_account). An account with no 5-hour window says so instead of a graph.
import { usedPath } from '@handline/app-core';
import { useOutlook } from '@handline/app-data';

const W = 280;
const H = 80;

type Sample = { at: string; left: number | null };

function Plot({ title, samples, from, to, marks }: { title: string; samples: Sample[]; from: number; to: number; marks: string[] }) {
  const x = (at: string) => ((Date.parse(at) - from) / Math.max(1, to - from)) * W;
  return (
    <figure className="flex min-w-0 flex-col gap-1">
      <figcaption className="text-xs text-muted">{title}</figcaption>
      <svg
        viewBox={`0 0 ${W} ${H}`}
        preserveAspectRatio="none"
        className="h-20 w-full overflow-visible"
        role="img"
        aria-label={`${title} usage, last 7 days`}
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
        {marks.map((at) => (
          <line
            key={at}
            x1={x(at)}
            x2={x(at)}
            y1="0"
            y2={H}
            stroke="currentColor"
            strokeDasharray="2 3"
            className="text-border"
            vectorEffect="non-scaling-stroke"
          />
        ))}
        <path
          d={usedPath(samples, from, to, W, H)}
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          className="text-accent"
          vectorEffect="non-scaling-stroke"
        />
      </svg>
    </figure>
  );
}

/** One window's history of an account over `range`: the daemon's points are 96 over twice the range, so the 5h window reads from a day
 *  (a point each 30 minutes) and shows its fills and resets; over 7 days a point each 3.5 hours would draw them as long ramps. */
function History({
  account,
  range,
  title,
  field,
}: {
  account: string;
  range: '24h' | '7d';
  title: string;
  field: 'five_left' | 'week_left';
}) {
  const mine = useOutlook(range).data?.by_account?.find((b) => b.account === account);
  if (!mine?.past.length) return <div />;
  if (field === 'five_left' && !mine.five_hour)
    return (
      <figure className="flex min-w-0 flex-col gap-1">
        <figcaption className="text-xs text-muted">{title}</figcaption>
        <p className="flex h-20 items-center text-sm text-muted">This account has no 5-hour limit.</p>
      </figure>
    );
  const from = Date.parse(mine.past[0]!.at);
  const to = Date.parse(mine.past.at(-1)!.at);
  const resets = (mine.resets ?? [])
    .filter((r) => r.window === (field === 'five_left' ? '5h' : '7d') && Date.parse(r.at) >= from && Date.parse(r.at) <= to)
    .map((r) => r.at);
  return (
    <Plot
      title={title}
      samples={mine.past.map((p) => ({ at: p.at, left: p[field] }))}
      from={from}
      to={to}
      marks={field === 'week_left' ? resets : []}
    />
  );
}

export function AccountHistory({ account }: { account: string }) {
  return (
    <div className="grid grid-cols-2 gap-4">
      <History account={account} range="24h" title="5-hour · last 24 hours" field="five_left" />
      <History account={account} range="7d" title="Week · last 7 days" field="week_left" />
    </div>
  );
}
