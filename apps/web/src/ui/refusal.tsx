import { refusalLine } from '@handline/app-core';
import { taskOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { toast } from '#src/ui/toast.tsx';

/** A refused close of a task, said in one plain line, with a link to the task to deal with first when there is one. */
export function sayRefusal(e: unknown) {
  const { text, open } = refusalLine(e instanceof Error ? e.message : String(e));
  toast(
    <>
      {text}
      {open !== null && (
        <>
          {' · '}
          <OpenTask id={open} />
        </>
      )}
    </>,
    { tone: 'warning', ms: 8000 },
  );
}

function OpenTask({ id }: { id: number }) {
  return (
    <button type="button" {...useOpenable(taskOf(id))} className="text-accent hover:underline">
      Open T{id}
    </button>
  );
}
