// What a task waiting on the founder asks, in the words of the comment that asked. The wait ends with an answer to that comment
// (its Reference in Activity, then the comment box below), or with Dismiss when there is nothing to answer; seeing it does not end it.
import { isPerson } from '@handline/app-core';
import type { TaskDetail } from '@handline/client';
import { useWrite } from '@handline/app-data';
import { Hand, X } from 'lucide-react';
import { useEffect, useState } from 'react';
import { showHistory } from '#src/tools/TaskDiff.tsx';
import { askedAt, goToTurn, hintLine } from '#src/tools/TaskTurn.ts';
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

/** The choices an ask lays out, one a line: "A) ...", "B. ...", "1: ..."; fewer than two is no choice. */
const CHOICE = /^[ \t]*([A-D1-4])[).:][ \t]+(.+)$/gm;
export function choices(ask: string) {
  const found = [...ask.matchAll(CHOICE)].map((m) => ({ key: m[1]!, text: m[2]!.trim() }));
  return found.length >= 2 ? found : [];
}

/** The founder's turn: its question, a button to where it was asked in Activity (marked faintly while pointed at), and an "edited"
 * mark with the older texts. It is answered, or given back, from the comment box below. */
export function YourTurn({ t, me }: { t: TaskDetail; me: string }) {
  const turn = t.turns?.at(-1);
  const earlier = turn?.earlier ?? [];
  const asking = askedAt(t, me);
  const line = asking?.line;
  // The mark comes off when the box goes or points elsewhere, as well as when the pointer leaves.
  useEffect(() => () => void (line && hintLine(line, false)), [line]);
  const mark = (on: boolean) => line && hintLine(line, on);
  const head = (
    <>
      <span className="flex items-center gap-2 font-medium">
        <Hand size={14} className="shrink-0 text-warning" />
        Your turn
        {asking && (
          <span className="invisible ml-auto truncate text-xs font-normal text-muted group-hover/turn:visible group-focus-visible/turn:visible">
            Click to go to this question in Activity: {asking.who} · {clock(asking.at)}
          </span>
        )}
      </span>
      {turn?.ask && <span className="pl-6 whitespace-pre-wrap">{turn.ask}</span>}
    </>
  );
  return (
    <div className="flex flex-col gap-1">
      {asking ? (
        <button
          type="button"
          onClick={() => goToTurn(t, me)}
          onMouseEnter={() => mark(true)}
          onMouseLeave={() => mark(false)}
          onFocus={() => mark(true)}
          onBlur={() => mark(false)}
          className="group/turn -mx-3 -my-2 flex flex-col gap-1 rounded-lg px-3 py-2 text-left transition-colors outline-none hover:bg-warning/15 focus-visible:ring-2 focus-visible:ring-accent"
        >
          {head}
        </button>
      ) : (
        <div className="flex flex-col gap-1">{head}</div>
      )}
      {turn?.edited && (
        <button
          type="button"
          disabled={!earlier.length}
          onClick={() =>
            showHistory('Versions of the question', [
              ...earlier.map((e) => ({ text: e.ask, at: e.at, by: turn.by })),
              { text: turn.ask ?? '', at: turn.edited ?? '', by: turn.by },
            ])
          }
          title={turn.edited}
          className="mt-2 w-fit pl-6 text-xs text-muted enabled:underline enabled:decoration-dotted enabled:hover:text-fg"
        >
          edited {ago(turn.edited)}
        </button>
      )}
    </div>
  );
}
