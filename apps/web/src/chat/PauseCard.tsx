import { hooks, useAgents, useTask } from '@handline/app-data';
import { Pause, Play } from 'lucide-react';
import { useState } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { toast } from '#src/ui/toast.tsx';

/** At the end of a chat whose agent has a task the pause holds: run just that one, or resume all new work. */
export function PauseCard({ agent }: { agent: string }) {
  const status = useAgents().data?.agents.find((a) => a.key === agent)?.status;
  const held = status?.reason === 'paused' ? (status.tasks[0] ?? null) : null;
  if (held === null) return null;
  return <Card task={held} more={status!.tasks.length - 1} />;
}

function Card({ task, more }: { task: number; more: number }) {
  const title = useTask(task).data?.title;
  const machine = hooks.useDaemonStatus().data?.paused?.machine?.name;
  const resume = hooks.useDaemonResume();
  const sendNow = hooks.useTaskSendNow();
  const [busy, setBusy] = useState(false);
  const act = (run: () => Promise<unknown>, failed: string) => {
    setBusy(true);
    run()
      .catch(() => toast(failed, { tone: 'warning' }))
      .finally(() => setBusy(false));
  };
  return (
    <div
      className={`flex max-w-150 flex-col gap-3 rounded-lg border border-border p-3 text-sm ${busy ? 'pointer-events-none opacity-60' : ''}`}
    >
      <div className="flex gap-2.5">
        <Pause size={16} className="mt-0.5 shrink-0 text-warning" />
        <div className="flex min-w-0 flex-col gap-0.5">
          <p className="text-fg">{machine ? `${machine} is paused, so this waits:` : 'New work is paused, so this waits:'}</p>
          <p className="truncate text-muted">
            T{task} {title}
            {more > 0 && ` and ${more} more`}
          </p>
        </div>
      </div>
      <div className="flex gap-2 pl-6">
        <Button variant="primary" onClick={() => act(() => sendNow.mutateAsync({ task }), `T${task} did not start.`)}>
          <Play size={14} />
          Run just this
        </Button>
        <Button onClick={() => act(() => resume.mutateAsync({}), 'Did not resume.')}>Resume all</Button>
      </div>
    </div>
  );
}
