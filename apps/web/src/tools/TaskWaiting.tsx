// What a task waiting on the founder asks, in the words of the comment that asked. The wait ends with an answer to that comment
// (its Reference in Activity, then the comment box below), or with Dismiss when there is nothing to answer; seeing it does not end it.
import { isPerson, renderRefs } from '@handline/app-core';
import type { TaskDetail } from '@handline/client';
import { useDirectory, useWrite } from '@handline/app-data';
import { Who } from '#src/ui/Who.tsx';
import { Hand, X } from 'lucide-react';
import { useState } from 'react';
import { IconButton } from '#src/ui/IconButton.tsx';
import { toast } from '#src/ui/toast.tsx';
import { clock } from '#src/format/time.ts';

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
  const dir = useDirectory();
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
          <Who k={asked.author} /> · {clock(asked.at)}
        </span>
        <span className="line-clamp-2 pl-6">{renderRefs(dir, asked.text.replaceAll(`<@${me}>`, '')).trim()}</span>
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
