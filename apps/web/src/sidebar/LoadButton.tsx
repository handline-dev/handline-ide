// The status bar's load: an icon that opens the heavy runs going now and what each agent uses, CPU and memory, heaviest first, under
// this machine's totals. Beside the icon it says when the Mac is slow or runs wait for a turn, and nothing while it is calm.
// CPU is a share of the whole machine, as its total is, not of one core. A row opens the agent's chat by the app's one scheme. It reads load.get only while open.
import { byLoad, bytesText, runLoadChip } from '@handline/app-core';
import { Hooks, useNameOf } from '@handline/app-data';
import { Activity } from 'lucide-react';
import { useState, type RefObject } from 'react';
import { chatOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { Popover } from '#src/ui/Popover.tsx';
import { RunLoadPart } from '#src/sidebar/RunLoad.tsx';

/** How often the open list reads: about the daemon's own sampling. */
const EVERY_MS = 5000;
const pct = (n: number) => `${Math.round(n)}%`;

export function LoadButton({ bar }: { bar: RefObject<HTMLDivElement | null> }) {
  const [open, setOpen] = useState(false);
  const chip = runLoadChip(Hooks.useDaemonStatus().data?.run_load);
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
          title="Heavy runs, and CPU and memory by agent"
          className="flex h-5 min-w-5 shrink-0 items-center justify-center gap-1.5"
        >
          {chip && <span className={chip.slow ? 'text-warning' : 'text-fg'}>{chip.text}</span>}
          <Activity size={14} className={chip?.slow ? 'text-warning' : ''} />
        </button>
      }
    >
      {open && <LoadList />}
    </Popover>
  );
}

function LoadList() {
  // The daemon samples every few seconds (every_s); reading as often shows each new sample.
  const load = Hooks.useLoadGet({}, { refetchMs: EVERY_MS }).data;
  if (!load) return <p className="w-80 px-2 py-1.5 text-sm text-muted">Reading…</p>;
  const m = load.machine;
  const rows = byLoad(load.agents ?? []);
  return (
    <div className="flex max-h-128 w-80 flex-col overflow-y-auto">
      <RunLoadPart everyMs={EVERY_MS} />
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
