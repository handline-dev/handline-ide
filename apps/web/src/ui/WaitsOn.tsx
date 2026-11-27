// An agent that waits on another's task: an hourglass for its mark, and under its name the task it waits on, who acts on it, and how many more.
import { useNameOf } from '@handline/app-data';
import { Hourglass } from 'lucide-react';
import type { KeyboardEvent, MouseEvent } from 'react';
import { taskOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';

export type Waits = { task: number; holder: string | null; more: number };

export function WaitsMark() {
  return (
    <span title="Waiting on a task" role="img" aria-label="Waiting on a task" className="inline-grid place-items-center">
      <Hourglass size={14} className="text-warning opacity-70" />
    </span>
  );
}

/** "Waiting on T12 (holder) and 3 more tasks"; `short` (a sidebar row) drops the holder into the chip's tooltip. */
export function WaitsLine({ w, short = false }: { w: Waits; short?: boolean }) {
  const nameOf = useNameOf();
  const open = useOpenable(taskOf(w.task));
  const holder = w.holder ? nameOf(w.holder) : null;
  // Inside a row that opens on click: the chip's own click is the task's alone.
  const stop =
    <E extends MouseEvent | KeyboardEvent>(f: (e: E) => void) =>
    (e: E) => {
      e.stopPropagation();
      f(e);
    };
  const more = w.more > 0 && (short ? ` +${w.more} more` : ` and ${w.more} more ${w.more === 1 ? 'task' : 'tasks'}`);
  return (
    <span className="flex min-w-0 items-center gap-1 text-xs text-muted">
      <span className="shrink-0">Waiting on</span>
      <button
        type="button"
        title={holder ? `T${w.task} \u00b7 ${holder} is on it` : `T${w.task}`}
        onClick={stop(open.onClick)}
        onAuxClick={stop(open.onAuxClick)}
        onKeyDown={stop(open.onKeyDown)}
        onContextMenu={(e) => e.stopPropagation()}
        className="shrink-0 rounded-sm bg-accent/15 px-1 text-accent tabular-nums hover:bg-accent/25"
      >
        T{w.task}
      </button>
      <span className="min-w-0 truncate">
        {!short && holder && `(${holder})`}
        {more}
      </span>
    </span>
  );
}
