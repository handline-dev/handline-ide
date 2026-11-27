// The status bar's load: an icon that opens what each agent uses now, CPU and memory, heaviest first, under this machine's totals.
// CPU is a share of the whole machine, as its total is, not of one core. A row opens the agent's chat by the app's one scheme. It reads load.get only while open.
import { byLoad, bytesText } from '@handline/app-core';
import { useNameOf, useRead } from '@handline/app-data';
import { Activity } from 'lucide-react';
import { useState, type RefObject } from 'react';
import { chatOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { Popover } from '#src/ui/Popover.tsx';

/** How often the open list reads: about the daemon's own sampling. */
const EVERY_MS = 5000;
const pct = (n: number) => `${Math.round(n)}%`;

export function LoadButton({ bar }: { bar: RefObject<HTMLDivElement | null> }) {
  const [open, setOpen] = useState(false);
  return (
    <Popover
      side="top"
      align="end"
      corner={bar}
      open={open}
      onOpenChange={setOpen}
      trigger={
        <button
          type="button"
          aria-label="What each agent uses"
          title="CPU and memory by agent"
          className="flex size-5 items-center justify-center"
        >
          <Activity size={14} />
        </button>
      }
    >
      {open && <LoadList />}
    </Popover>
  );
}

function LoadList() {
  // The daemon samples every few seconds (every_s); reading as often shows each new sample.
  const load = useRead('load.get', {}, { refetchMs: EVERY_MS }).data;
  if (!load) return <p className="w-80 px-2 py-1.5 text-sm text-muted">Reading…</p>;
  const m = load.machine;
  const rows = byLoad(load.agents ?? []);
  return (
    <div className="flex max-h-96 w-80 flex-col overflow-y-auto">
      <p className="flex justify-between px-2 pb-1.5 text-xs text-muted">
        <span>This Mac</span>
        <span className="tabular-nums">
          CPU {pct(m.load_per_core * 100)} · {bytesText(m.ram_used_bytes)} of {bytesText(m.ram_total_bytes)}
        </span>
      </p>
      {rows.length > 0 && (
        <p className="flex gap-3 px-2 pb-1 text-xs text-muted">
          <span className="flex-1">Agent</span>
          <span className="w-14 text-right">CPU</span>
          <span className="w-16 text-right">Memory</span>
        </p>
      )}
      {rows.length === 0 && <p className="px-2 py-1.5 text-sm text-muted">No agent is running.</p>}
      {rows.map((r) => (
        <Row key={r.agent} agent={r.agent} cpu={r.cpu_pct / Math.max(1, m.cores)} rss={r.rss_bytes} />
      ))}
    </div>
  );
}

function Row({ agent, cpu, rss }: { agent: string; cpu: number; rss: number }) {
  const name = useNameOf()(agent);
  return (
    <button
      type="button"
      {...useOpenable(chatOf(agent))}
      className="flex items-center gap-3 border-t border-border px-2 py-1.5 text-left text-sm hover:bg-hover/30"
    >
      <span className="min-w-0 flex-1 truncate text-fg">{name}</span>
      <span className="w-14 text-right tabular-nums text-muted">{pct(cpu)}</span>
      <span className="w-16 text-right tabular-nums text-muted">{bytesText(rss)}</span>
    </button>
  );
}
