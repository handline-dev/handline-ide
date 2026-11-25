// A task a seat of an agent holds or held is that agent's work beside its main task: the task page says why in one line, and folds what
// the seat did (chat.transcript with task) into one Work row, its current step live under it while the task runs.
import { useRead, useNameOf, useWorkTranscript } from '@handline/app-data';
import { ChevronDown, ChevronRight } from 'lucide-react';
import { useState } from 'react';
import { count } from '#src/format/count.ts';
import { ago, useNow } from '#src/format/time.ts';
import { goAgent } from '#src/layout/openTab.tsx';
import { taskOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { Step } from '#src/chat/TurnRecord.tsx';
import { ActivityIcon } from '#src/ui/activityIcon.tsx';

function MainTask({ id, title }: { id: number; title?: string }) {
  const open = useOpenable(taskOf(id));
  return (
    <button type="button" {...open} className="text-fg hover:underline">
      T{id}
      {title && ` ${title}`}
    </button>
  );
}

/** The line and the Work row; nothing for a task no seat holds or held. */
export function TaskAlongside({ task, holder, open }: { task: number; holder?: string; open: boolean }) {
  // useAgents hides seats; the full list has them.
  const agents = useRead('agents.list', {}).data?.agents;
  const seat = agents?.find((a) => a.key === holder && a.hired_from);
  if (!seat?.hired_from) return null;
  const agent = agents?.find((a) => a.key === seat.hired_from);
  const main = agent?.task && agent.task.id !== task ? agent.task : null;
  return <Alongside task={task} seat={seat.key} agent={seat.hired_from} main={main} open={open} />;
}

function Alongside({
  task,
  seat,
  agent,
  main,
  open,
}: {
  task: number;
  seat: string;
  agent: string;
  main: { id: number; title?: string } | null;
  open: boolean;
}) {
  const nameOf = useNameOf();
  const now = useNow(open ? 1000 : null);
  const [unfolded, setUnfolded] = useState(false);
  const items = useWorkTranscript(seat, task).data?.items ?? [];
  const steps = items.filter((i) => i.kind !== 'reply');
  const first = items[0]?.at;
  const last = items.at(-1);
  const took = first && (open ? ago(first, now) : last && ago(first, Date.parse(last.at)));
  const who = (
    <button type="button" onClick={() => goAgent(agent)} className="font-medium text-fg hover:underline">
      {nameOf(agent)}
    </button>
  );
  return (
    <div className="flex flex-col gap-2 text-sm text-muted">
      <p>
        <span className="mr-1.5 inline-flex align-middle">
          <ActivityIcon act={open ? 'working' : 'resting'} />
        </span>
        {who}{' '}
        {main ? (
          <>
            is busy on <MainTask id={main.id} title={main.title} />, so it {open ? 'does' : 'did'} this alongside
          </>
        ) : (
          <>{open ? 'does' : 'did'} this alongside its main work</>
        )}
      </p>
      {steps.length > 0 && (
        <div className="flex flex-col gap-2">
          <button
            type="button"
            aria-expanded={unfolded}
            onClick={() => setUnfolded((u) => !u)}
            className="flex w-fit items-center gap-1.5 hover:text-fg"
          >
            {unfolded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
            Work · {count(steps.length, 'step')}
            {took && took !== 'now' && ` · ${took}`}
          </button>
          {unfolded && (
            <div className="flex flex-col gap-2 border-l border-border pl-4">
              {items.map((i) => (
                <Step key={i.id} i={i} />
              ))}
            </div>
          )}
          {/* While it runs, what it does now: one line, no time (the row has the total). */}
          {open && !unfolded && last && last.kind !== 'reply' && (
            <div className="pl-5">
              <Step i={last} />
            </div>
          )}
        </div>
      )}
    </div>
  );
}
