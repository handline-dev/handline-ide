// What passes between an agent and the founder on a task, in the agent's chat: one card per task. It is answered in the task.
import { foundersTurn, noteText, type TimelineEntry, type TurnRecord } from '@handline/app-core';
import { useNameOf, useTask } from '@handline/app-data';
import { useMe } from '@handline/app-react';
import type { TranscriptItem } from '@handline/client';
import { motion } from 'framer-motion';
import { Hand } from 'lucide-react';
import { clock } from '#src/format/time.ts';
import { taskOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { useOpenTab } from '#src/layout/openTab.tsx';
import { spring } from '#src/motion.ts';
import { goToTurnOf } from '#src/tools/TaskTurn.ts';
import { Button } from '#src/ui/Button.tsx';
import { look } from '#src/ui/taskLook.tsx';

type Said = TranscriptItem & { task_comment: NonNullable<TranscriptItem['task_comment']> };
type Entry = TimelineEntry | TurnRecord;
const said = (e: Entry): Said | undefined =>
  e.kind === 'item' && e.item.task_comment ? { ...e.item, task_comment: e.item.task_comment } : undefined;

/** Each task's card is drawn at its newest line: the task's older lines leave the stream. */
export function taskCards(entries: Entry[]): Entry[] {
  const newest = new Map<number, number>();
  for (const e of entries) {
    const i = said(e);
    if (i) newest.set(i.task_comment.task, i.id);
  }
  return entries.filter((e) => {
    const i = said(e);
    return !i || newest.get(i.task_comment.task) === i.id;
  });
}

/** Opens the task beside the chat, at the founder's turn, with the comment box ready to write in. */
function useAnswerInTask() {
  const open = useOpenTab();
  return (task: number) => {
    goToTurnOf(task, true);
    open(taskOf(task), 'side');
  };
}

const firstLine = (body: string) =>
  noteText(body)
    .split('\n')
    .find((l) => l.trim()) ?? '';

/** A task's card with its newest line; on the founder's turn, tinted with a hand and Answer in task. The time shows on hover. */
export function TaskCommentCard({ last, agent, fade }: { last: Said; agent: string; fade: boolean }) {
  const task = last.task_comment.task;
  const t = useTask(task).data;
  const me = useMe();
  const nameOf = useNameOf();
  const answer = useAnswerInTask();
  const open = useOpenable(taskOf(task));
  const yours = !!t && foundersTurn(t, me);
  const s = t ? look(t) : null;
  const who = last.task_comment.kind === 'answer' ? `You → ${nameOf(agent)}` : `${nameOf(last.task_comment.who)} → you`;
  return (
    <motion.div
      data-id={`taskcard:${task}`}
      initial={fade ? { opacity: 0 } : false}
      animate={{ opacity: 1 }}
      transition={spring}
      className="group flex w-full min-w-0 flex-col items-start"
    >
      <div
        style={{ maxWidth: 'min(100%, 37.5rem)' }}
        className={`w-full overflow-hidden rounded-lg text-sm ${
          yours ? 'border border-warning/40 bg-warning/10' : 'border border-border bg-bg'
        }`}
      >
        {yours ? (
          <div className="flex gap-3 py-2.5 pr-3 pl-3">
            <Hand size={28} strokeWidth={1.75} className="mt-0.5 shrink-0 text-warning" aria-hidden />
            <div className="min-w-0 flex-1">
              <div className="flex min-w-0 items-baseline">
                <span className="shrink-0 font-semibold text-warning">Your turn</span>
                <span className="shrink-0 whitespace-pre text-muted"> on T{task} · </span>
                <button
                  type="button"
                  {...open}
                  className="min-w-0 truncate rounded-sm font-medium text-fg outline-none hover:underline focus-visible:ring-2 focus-visible:ring-accent"
                >
                  {t?.title ?? ''}
                </button>
              </div>
              <p className="mt-0.5 line-clamp-2 text-fg">{firstLine(last.task_comment.text)}</p>
              <div className="mt-2 flex justify-end">
                <Button variant="primary" onClick={() => answer(task)}>
                  Answer in task
                </Button>
              </div>
            </div>
          </div>
        ) : (
          <button
            type="button"
            {...open}
            className="flex w-full gap-2.5 px-3 py-2 text-left outline-none hover:bg-hover/30 focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-inset"
          >
            <span className={`mt-0.5 grid size-5 shrink-0 place-items-center rounded-md text-accent-fg ${s?.tone ?? 'bg-surface'}`}>
              {s && <s.icon size={12} strokeWidth={3} />}
            </span>
            <span className="min-w-0">
              <span className="block truncate">
                <span className="mr-1 text-muted">T{task}</span>
                <span className="font-medium text-fg">{t?.title ?? ''}</span>
              </span>
              <span className="block truncate text-xs text-muted">{who}</span>
              <span className="mt-0.5 line-clamp-2 block text-fg">{firstLine(last.task_comment.text)}</span>
            </span>
          </button>
        )}
      </div>
      <time
        dateTime={last.at}
        className="mb-2 flex h-6 items-center text-xs text-muted opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
      >
        {clock(last.at)}
      </time>
    </motion.div>
  );
}
