// The work an agent runs beside its own right now, held at the top of its chat box while it runs: one row per task, which opens the
// task's page. It leaves when the work ends; the chat keeps the task's card as the record.
import { ChevronRight } from 'lucide-react';
import { runningAlongside } from '@handline/app-core';
import { useChat, useTask } from '@handline/app-data';
import { ago, useNow } from '#src/format/time.ts';
import { taskOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { ActivityIcon } from '#src/ui/activityIcon.tsx';

function Row({ task, at, now }: { task: number; at: string; now: number }) {
  const title = useTask(task).data?.title;
  const open = useOpenable(taskOf(task));
  return (
    <button
      type="button"
      {...open}
      title={`Working alongside on T${task}`}
      className="flex h-7 w-full min-w-0 items-center gap-2 rounded-md px-1 text-left text-sm hover:bg-hover"
    >
      <span className="grid size-4 shrink-0 place-items-center">
        <ActivityIcon act="working" />
      </span>
      <span className="shrink-0 font-mono text-xs text-muted">T{task}</span>
      <span className="min-w-0 flex-1 truncate text-fg">{title}</span>
      <span className="shrink-0 text-xs text-muted tabular-nums">{ago(at, now)}</span>
      <ChevronRight size={14} className="shrink-0 text-muted" aria-hidden />
    </button>
  );
}

/** The tasks the agent runs beside its own now, oldest first. */
export const useAlongside = (agent: string) => runningAlongside(useChat(agent, 'all').items);

export function AlongsideStrip({ running }: { running: { task: number; at: string }[] }) {
  const now = useNow();
  return (
    <div aria-label="Working alongside" className="flex flex-col border-b border-border pb-1">
      {running.map((r) => (
        <Row key={r.task} task={r.task} at={r.at} now={now} />
      ))}
    </div>
  );
}
