import { Hooks, useWrite } from '@handline/app-data';
import type { AgentJob, AgentView } from '@handline/client';
import { Bot, Eye, CircleStop, Layers, Terminal } from 'lucide-react';
import type { ReactNode } from 'react';
import { chip } from '#src/chat/agent/menuParts.tsx';
import { ago, useNow } from '#src/format/time.ts';
import { HoverCard } from '#src/ui/HoverCard.tsx';
import { toast } from '#src/ui/toast.tsx';
import { uuid } from '#src/ui/uuid.ts';

export type Job = NonNullable<AgentView['background']>[number];

const kindIcon: Record<Job['kind'], ReactNode> & Partial<Record<string, ReactNode>> = {
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

/** The chip's panel: each job with what it last wrote and, for a shell, its Stop; Stop all at the foot. Read when it opens. */
function JobPanel({ agent, fallback }: { agent: string; fallback: Job[] }) {
  const now = useNow();
  const jobs = Hooks.useAgentJobs({ agent }).data?.jobs;
  const stopAll = useWrite('agent.jobs.stop', agent);
  if (!jobs) return <JobList jobs={fallback} />;
  return (
    <div className="flex w-96 flex-col">
      <ul className="max-h-80 overflow-y-auto">
        {jobs.map((j) => (
          <JobRow key={j.id} agent={agent} job={j} now={now} />
        ))}
      </ul>
      {jobs.length > 1 && (
        <div className="flex justify-end border-t border-border px-2 py-1.5">
          <button
            type="button"
            disabled={stopAll.isPending}
            onClick={() => stopAll.mutate({ agent, request_id: uuid() }, { onError: notStopped })}
            className="rounded px-2 py-0.5 text-xs text-fg hover:bg-hover disabled:text-muted"
          >
            Stop all
          </button>
        </div>
      )}
    </div>
  );
}

const notStopped = (e: unknown) => toast(`Not stopped: ${e instanceof Error ? e.message : String(e)}`, { tone: 'warning' });

function JobRow({ agent, job, now }: { agent: string; job: AgentJob; now: number }) {
  const stop = useWrite('agent.job.stop', agent);
  const shell = job.type === 'shell';
  return (
    <li className="px-2 py-1.5 text-xs">
      <div className="flex h-6 items-center gap-2" title={job.name}>
        <span className="shrink-0 text-muted">{kindIcon[job.type] ?? <Layers size={13} aria-label={job.type} />}</span>
        <span className="min-w-0 flex-1 truncate text-fg">{job.name}</span>
        {job.started != null && (
          <span className="shrink-0 text-muted tabular-nums">{ago(new Date(job.started * 1000).toISOString(), now)}</span>
        )}
        {shell && (
          <button
            type="button"
            disabled={stop.isPending}
            onClick={() => stop.mutate({ agent, job: job.id, request_id: uuid() }, { onError: notStopped })}
            className="flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-fg hover:bg-hover disabled:text-muted"
          >
            <CircleStop size={12} />
            Stop
          </button>
        )}
      </div>
      {job.output && job.output.length > 0 && (
        <pre className="mt-1 ml-5 overflow-hidden font-mono text-xs whitespace-pre-wrap break-all text-muted">{job.output.join('\n')}</pre>
      )}
    </li>
  );
}

/** On the line under the message box: how many jobs run; a click opens them with their output and Stop. Nothing while none run. */
export function BackgroundItem({ agent, jobs }: { agent: string; jobs: Job[] }) {
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
      <JobPanel agent={agent} fallback={jobs} />
    </HoverCard>
  );
}
