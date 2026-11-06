import type { AgentView } from '@handline/client';
import { Bot, Eye, Layers, Terminal } from 'lucide-react';
import type { ReactNode } from 'react';
import { chip } from '#src/chat/agent/menuParts.tsx';
import { ago, useNow } from '#src/format/time.ts';
import { HoverCard } from '#src/ui/HoverCard.tsx';

export type Job = NonNullable<AgentView['background']>[number];

const kindIcon: Record<Job['kind'], ReactNode> = {
  shell: <Terminal size={13} aria-label="shell" />,
  monitor: <Eye size={13} aria-label="monitor" />,
  subagent: <Bot size={13} aria-label="subagent" />,
};

/** What runs behind an agent's prompt, oldest first: its kind, what it is, and how long it has run. */
export function JobList({ jobs }: { jobs: Job[] }) {
  const now = useNow();
  return (
    <ul className="max-h-64 max-w-80 overflow-y-auto">
      {jobs.map((j, i) => (
        <li key={i} title={j.label} className="flex h-6 items-center gap-2 px-2 text-xs">
          <span className="shrink-0 text-muted">{kindIcon[j.kind]}</span>
          <span className="min-w-0 flex-1 truncate text-fg">{j.label}</span>
          <span className="ml-4 shrink-0 text-muted tabular-nums">{ago(j.since, now)}</span>
        </li>
      ))}
    </ul>
  );
}

/** On the line under the message box: how many jobs run; resting on it lists them, a click keeps the list open. Nothing while none run. */
export function BackgroundItem({ jobs }: { jobs: Job[] }) {
  if (jobs.length === 0) return null;
  return (
    <HoverCard
      pin
      trigger={
        <button type="button" className={`${chip} shrink-0`}>
          <Layers size={13} className="shrink-0" />
          <span className="truncate">{jobs.length} running</span>
        </button>
      }
    >
      <JobList jobs={jobs} />
    </HoverCard>
  );
}
