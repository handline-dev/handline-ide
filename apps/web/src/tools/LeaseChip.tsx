import { Hourglass, Play, Plug } from 'lucide-react';
import { ago, useNow } from '#src/format/time.ts';
import { HoverCard } from '#src/ui/HoverCard.tsx';
import type { LeaseWait } from '#src/tools/leaseWait.ts';

/** A task's wait in line: the run's name and its place when not next; hover or tap opens the line. A port wait is one word. */
export function LeaseChip({ wait, task }: { wait: LeaseWait; task: number }) {
  const now = useNow();
  if (wait.kind === 'port')
    return (
      <span className="inline-flex items-center gap-1">
        <Plug size={12} className="shrink-0" /> Waiting for a port
      </span>
    );
  const chip = (
    <span
      role="button"
      tabIndex={0}
      draggable={false}
      onClick={(e) => e.stopPropagation()}
      onAuxClick={(e) => e.stopPropagation()}
      onDragStart={(e) => e.stopPropagation()}
      onKeyDown={(e) => {
        if (e.key !== 'Enter' && e.key !== ' ') return;
        e.preventDefault();
        e.stopPropagation();
        e.currentTarget.click();
      }}
      className="inline-flex items-center gap-1 rounded-sm border border-border px-1.5 align-middle text-fg"
    >
      <Hourglass size={12} className="shrink-0" />
      {wait.run}
      {wait.position > 1 && <span className="text-xs text-muted">#{wait.position}</span>}
    </span>
  );
  return (
    <HoverCard trigger={chip} pin side="top">
      <div className="flex flex-col gap-1.5 px-2 py-1.5 text-sm">
        <span className="text-xs text-muted">{wait.run}</span>
        {wait.holders.map((h) => (
          <span key={h.key} className="flex items-center gap-2">
            <Play size={12} className="w-4 shrink-0 text-success" />
            {h.name}
            <span className="ml-auto pl-4 text-muted">In use · {h.since ? ago(h.since, now) : 'now'}</span>
          </span>
        ))}
        {wait.waiting.map((w) => (
          <span key={`${w.key}:${w.position}`} className={`flex items-center gap-2 ${w.task === task ? 'font-semibold' : ''}`}>
            <span className="w-4 shrink-0 text-center text-muted">{w.position}</span>
            {w.task === task ? `T${task}` : w.name}
            {w.task === task && <span className="ml-auto pl-4 text-muted">This task</span>}
          </span>
        ))}
      </div>
    </HoverCard>
  );
}
