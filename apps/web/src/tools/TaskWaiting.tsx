// What a task waiting on the founder asks, in the words of the comment that asked. The wait ends with an answer to that comment
// (its Reference in Activity, then the comment box below), or with Dismiss when there is nothing to answer; seeing it does not end it.
import { isPerson } from '@handline/app-core';
import type { TaskDetail } from '@handline/client';
import { classify, sentence, useGiveBack, useWrite } from '@handline/app-data';
import { Hand, X } from 'lucide-react';
import { useState } from 'react';
import { showHistory } from '#src/tools/TaskDiff.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { toast } from '#src/ui/toast.tsx';
import { ago, clock } from '#src/format/time.ts';

type Memo = TaskDetail['memos'][number];

/** The founder's requests on this task still open. */
export const openAsks = (t: TaskDetail, me: string) =>
  (t.requests ?? []).filter((r) => (r.to === 'human' || isPerson(r.to, me)) && (r.state === 'queued' || r.state === 'landed'));

export function WaitingOnYou({
  t,
  me,
  asked,
  onShow,
  onDone,
}: {
  t: TaskDetail;
  me: string;
  asked: Memo;
  onShow: () => void;
  /** Reads the task again once the asks are closed: the wait is worked out from them, and no task notice comes for it. */
  onDone: () => void;
}) {
  const read = useWrite('inbox.mark_read');
  const open = openAsks(t, me);
  // Dismiss: the asks are marked read and the wait ends; the agent is told nothing more.
  // Dimmed from the click until the task comes back without the wait (the box then goes); a failure brings it back.
  const [going, setGoing] = useState(false);
  const dismiss = () => {
    setGoing(true);
    read.mutate(
      { ids: open.map((r) => r.id) },
      {
        onSuccess: onDone,
        onError: (e) => {
          setGoing(false);
          toast(`Dismiss did not work: ${e instanceof Error ? e.message : String(e)}`, { tone: 'warning' });
        },
      },
    );
  };
  return (
    // The asking comment's first lines; a click goes to it in Activity, read whole and answered there (its Reference).
    <div
      aria-busy={going}
      className={`group relative flex flex-col gap-1 transition-opacity ${going ? 'pointer-events-none opacity-40' : ''}`}
    >
      {open.length > 0 && (
        <span className="absolute -top-1 -right-2 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
          <IconButton className="size-6!" label="Dismiss" disabled={going} onClick={dismiss}>
            <X size={14} />
          </IconButton>
        </span>
      )}
      <button type="button" onClick={onShow} className="flex flex-col gap-1 text-left">
        <span className="flex items-center gap-2 font-medium">
          <Hand size={14} className="shrink-0 text-warning" />
          Waiting on you
        </span>
        <span className="pl-6 text-xs text-muted">
          {asked.author} · {clock(asked.at)}
        </span>
        <span className="line-clamp-2 pl-6">{asked.text.replace(/@human\s*/g, '')}</span>
      </button>
    </div>
  );
}

/** The founder's turn is the task's one question: the ask, an "edited" mark with older texts folded under it, and Give back. */
export function YourTurn({ t, onDone, onShow }: { t: TaskDetail; onDone: () => void; onShow: () => void }) {
  const back = useGiveBack();
  const turn = t.turns?.at(-1);
  const earlier = turn?.earlier ?? [];
  const give = () =>
    back.give(t.id, { onSuccess: onDone, onError: (e) => toast(`Give back did not work: ${sentence(classify(e))}`, { tone: 'warning' }) });
  return (
    <div aria-busy={back.pending} className={`flex items-start gap-2 ${back.pending ? 'pointer-events-none opacity-40' : ''}`}>
      <Hand size={14} className="mt-1 shrink-0 text-warning" />
      <span className="flex min-w-0 flex-1 flex-col gap-1">
        <span className="flex items-center gap-2 font-medium">
          <button type="button" onClick={onShow} className="hover:underline">
            Your turn
          </button>
          {turn?.edited && (
            <button
              type="button"
              disabled={!earlier.length}
              onClick={() =>
                showHistory('Versions of the question', [
                  ...earlier.map((e) => ({ text: e.ask, at: e.at, by: turn?.by ?? '' })),
                  { text: turn?.ask ?? '', at: turn?.edited ?? '', by: turn?.by ?? '' },
                ])
              }
              title={turn.edited}
              className="text-xs font-normal text-muted enabled:underline enabled:decoration-dotted enabled:hover:text-fg"
            >
              edited {ago(turn.edited)}
            </button>
          )}
        </span>
        {turn?.ask && <span className="whitespace-pre-wrap">{turn.ask}</span>}
      </span>
      <button type="button" onClick={give} className="shrink-0 rounded-md px-2 py-1 text-sm font-medium hover:bg-hover/30">
        Give back
      </button>
    </div>
  );
}
