// What passes between an agent and the founder on a task, in the agent's chat: one block per run of talk. It is answered in the task.
import { foundersTurn, noteText, type TimelineEntry, type TurnRecord } from '@handline/app-core';
import { useNameOf, useTask } from '@handline/app-data';
import { useMe } from '@handline/app-react';
import type { TranscriptItem } from '@handline/client';
import { motion } from 'framer-motion';
import { Hand, ListChecks } from 'lucide-react';
import { clock } from '#src/format/time.ts';
import { taskOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { useOpenTab } from '#src/layout/openTab.tsx';
import { spring } from '#src/motion.ts';
import { goToTurnOf } from '#src/tools/TaskTurn.ts';
import { Button } from '#src/ui/Button.tsx';

type Said = TranscriptItem & { task_comment: NonNullable<TranscriptItem['task_comment']> };
type Entry = TimelineEntry | TurnRecord;
const said = (e: Entry): Said | undefined =>
  e.kind === 'item' && e.item.task_comment ? { ...e.item, task_comment: e.item.task_comment } : undefined;

/** The talk on a task, in runs: lines next to each other on one task are one block, drawn at its first line; `blocks` holds each
 * block's lines by that line's id. */
export function taskBlocks(entries: Entry[]): { shown: Entry[]; blocks: Map<number, Said[]> } {
  const blocks = new Map<number, Said[]>();
  let open: Said[] | null = null;
  const shown = entries.filter((e) => {
    const i = said(e);
    if (!i) {
      open = null;
      return true;
    }
    if (open && open[0]!.task_comment.task === i.task_comment.task) {
      open.push(i);
      return false;
    }
    open = [i];
    blocks.set(i.id, open);
    return true;
  });
  return { shown, blocks };
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

/** A task's run of talk: its title, then one line per comment, who to whom and when over its first line. On the founder's turn,
 * Answer in task on the task's newest block. */
export function TaskCommentCard({ lines, latest, agent, fade }: { lines: Said[]; latest: boolean; agent: string; fade: boolean }) {
  const task = lines[0]!.task_comment.task;
  const t = useTask(task).data;
  const me = useMe();
  const nameOf = useNameOf();
  const answer = useAnswerInTask();
  const open = useOpenable(taskOf(task));
  const yours = latest && !!t && foundersTurn(t, me);
  return (
    <motion.div
      data-id={`taskcard:${task}:${lines[0]!.id}`}
      initial={fade ? { opacity: 0 } : false}
      animate={{ opacity: 1 }}
      transition={spring}
      className="my-3 flex w-full min-w-0 gap-2.5 text-sm"
    >
      <ListChecks size={16} className="mt-0.5 shrink-0 text-muted" aria-hidden />
      <div className="flex min-w-0 flex-1 flex-col gap-2.5">
        <button
          type="button"
          {...open}
          className="min-w-0 self-start truncate rounded-sm text-left outline-none hover:bg-hover/30 focus-visible:ring-2 focus-visible:ring-accent"
        >
          <span className="mr-1.5 text-muted">T{task}</span>
          <span className="font-medium text-fg">{t?.title ?? ''}</span>
        </button>
        {lines.map((l) => (
          <div key={l.id} className="flex min-w-0 flex-col">
            <span className="text-xs text-muted">
              {l.task_comment.kind === 'answer' ? `You → ${nameOf(agent)}` : `${nameOf(l.task_comment.who)} → you`} · {clock(l.at)}
            </span>
            <span className="truncate text-fg">{firstLine(l.task_comment.text)}</span>
          </div>
        ))}
        {yours && (
          <div className="flex items-center gap-2">
            <Hand size={14} className="text-warning" aria-hidden />
            <span className="font-medium text-warning">Your turn</span>
            <Button variant="primary" onClick={() => answer(task)}>
              Answer in task
            </Button>
          </div>
        )}
      </div>
    </motion.div>
  );
}
