// Under a task's clock: what its model calls cost at list price (task.get's spend, the task and its sub-tasks). A click shows who and
// which model. A task change notice carries no cost, so the figure is the one the page last read.
import { spendShares, taskCostText, usdText } from '@handline/app-core';
import { useNameOf } from '@handline/app-data';
import type { TaskDetail } from '@handline/client';
import { timeLength } from '#src/tools/TaskClock.tsx';
import { Popover } from '#src/ui/Popover.tsx';

/** One side of the cost: its rows, largest first, each with its dollars and its share. */
function Shares({ title, by, word }: { title: string; by: Record<string, number>; word: (key: string) => string }) {
  const rows = spendShares(by);
  if (!rows.length) return null;
  return (
    <section className="flex flex-col">
      <h3 className="text-xs text-muted">{title}</h3>
      <ul className="flex flex-col">
        {rows.map((r) => (
          <li key={r.key} className="flex h-7 items-center gap-2">
            <span className="min-w-0 flex-1 truncate">{word(r.key)}</span>
            <span className="text-muted tabular-nums">{usdText(r.usd)}</span>
            <span className="w-9 text-right text-xs text-muted tabular-nums">{Math.round(r.share * 100)}%</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

/** Nothing for a task no call was charged to. */
export function TaskSpend({ spend }: { spend: TaskDetail['spend'] }) {
  const nameOf = useNameOf();
  if (!spend) return null;
  return (
    <Popover
      trigger={
        <button type="button" className="-mx-2 w-fit rounded-md px-2 py-0.5 text-left text-xs text-muted hover:bg-hover/30">
          {taskCostText(spend)}
        </button>
      }
    >
      <div className="flex w-72 flex-col gap-3">
        <div className="flex items-baseline justify-between gap-3">
          <span className="font-medium">
            {usdText(spend.usd)} <span className="font-normal text-muted">at API prices</span>
          </span>
          {spend.minutes > 0 && <span className="text-muted">{timeLength(spend.minutes * 60)} of calls</span>}
        </div>
        <p className="text-xs text-muted">
          What the model calls of this task and its sub-tasks cost at list price. A subscription is not billed for them.
        </p>
        <Shares title="By agent" by={spend.by_agent} word={(k) => (k ? nameOf(k) : 'No agent')} />
        <Shares title="By model" by={spend.by_model} word={(k) => k || 'Unknown model'} />
      </div>
    </Popover>
  );
}
