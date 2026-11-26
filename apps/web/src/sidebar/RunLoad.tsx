// The load popover's first part: the heavy runs going now, each against what it usually takes, the runs waiting for a turn, why the
// Mac counts as slow, and what Handline does about it. It reads run.load only while the popover is open.
import { ago, bytesText, causeLines, hostLine, policyNow, runTime, type RunLoadView } from '@handline/app-core';
import { Hooks, useNameOf } from '@handline/app-data';
import { TriangleAlert } from 'lucide-react';
import { chatOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { openSettings } from '#src/settings/open.ts';

type Run = RunLoadView['runs'][number];

export function RunLoadPart({ everyMs }: { everyMs: number }) {
  const load = Hooks.useRunLoad({}, { refetchMs: everyMs }).data;
  if (!load) return null;
  const heavy = load.runs.filter((r) => r.heavy);
  return (
    <div className="flex flex-col pb-2">
      {load.slow && (
        <div role="status" className="mb-2 flex flex-col gap-0.5 rounded-md bg-warning/10 px-2 py-1.5 text-xs">
          <p className="flex items-center justify-between gap-2 text-sm text-fg">
            <span className="flex items-center gap-1.5">
              <TriangleAlert size={14} className="text-warning" />
              The Mac is slow
            </span>
            {load.since && <span className="text-xs text-muted tabular-nums">for {ago(load.since)}</span>}
          </p>
          {causeLines(load).map((line) => (
            <p key={line} className="text-fg">
              {line}
            </p>
          ))}
          <p className="text-muted">{hostLine(load.host)}</p>
        </div>
      )}
      <p className="flex justify-between px-2 pb-1 text-xs text-muted">
        <span>Heavy runs</span>
        <span className="tabular-nums">
          {heavy.length}
          {load.cap !== null && ` of ${load.cap}`}
        </span>
      </p>
      {heavy.length === 0 && <p className="px-2 py-1.5 text-sm text-muted">No heavy run is going.</p>}
      {heavy.map((r) => (
        <RunRow key={`${r.agent}:${r.started}:${r.shape}`} r={r} />
      ))}
      {load.waiting.map((w) => (
        <WaitRow key={`${w.agent}:${w.since}`} agent={w.agent} shape={w.shape} since={w.since} />
      ))}
      <p className="flex items-baseline justify-between gap-2 border-t border-border px-2 pt-2 text-xs text-muted">
        <span>{policyNow(load)}</span>
        <button type="button" onClick={() => openSettings({ section: 'devices' })} className="shrink-0 text-fg hover:underline">
          Change
        </button>
      </p>
    </div>
  );
}

function RunRow({ r }: { r: Run }) {
  const name = useNameOf()(r.agent);
  return (
    <button
      type="button"
      {...useOpenable(chatOf(r.agent))}
      className="flex flex-col border-t border-border px-2 py-1.5 text-left hover:bg-hover/30"
    >
      <span className="flex items-baseline gap-3 text-sm">
        <span className="min-w-0 flex-1 truncate text-fg">{name}</span>
        <span className={`shrink-0 tabular-nums ${r.late ? 'text-warning' : 'text-muted'}`}>{runTime(r.started, r.usual_s)}</span>
      </span>
      <span className="flex items-baseline gap-3 text-xs text-muted">
        <span className="min-w-0 flex-1 truncate">
          {r.shape}
          {r.background && ' · in the background'}
        </span>
        {r.cpu_pct !== null && r.rss_bytes !== null && (
          <span className="shrink-0 tabular-nums">
            {Math.round(r.cpu_pct)}% of a core · {bytesText(r.rss_bytes)}
          </span>
        )}
      </span>
    </button>
  );
}

function WaitRow({ agent, shape, since }: { agent: string; shape: string; since: string }) {
  const name = useNameOf()(agent);
  return (
    <button
      type="button"
      {...useOpenable(chatOf(agent))}
      className="flex flex-col border-t border-border px-2 py-1.5 text-left hover:bg-hover/30"
    >
      <span className="flex items-baseline gap-3 text-sm">
        <span className="min-w-0 flex-1 truncate text-fg">{name}</span>
        <span className="shrink-0 text-muted tabular-nums">waiting {ago(since)}</span>
      </span>
      <span className="truncate text-xs text-muted">{shape}</span>
    </button>
  );
}
