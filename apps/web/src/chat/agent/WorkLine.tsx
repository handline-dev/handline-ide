import { Hooks, useTask } from '@handline/app-data';
import type { TranscriptItem } from '@handline/client';
import { ArrowRight, Check, ChevronRight, X } from 'lucide-react';
import { count } from '#src/format/count.ts';
import { ago, useNow } from '#src/format/time.ts';
import { workOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { ActivityIcon } from '#src/ui/activityIcon.tsx';

type Work = NonNullable<TranscriptItem['concurrent_work']>;

const ENDED = {
  done: { icon: <Check size={14} className="text-success" />, word: 'Done alongside:' },
  handed_off: { icon: <ArrowRight size={14} className="text-muted" />, word: 'Handed off alongside:' },
  discarded: { icon: <X size={14} className="text-muted" />, word: 'Discarded alongside:' },
} as const;

/** The tasks whose work beside the agent's own has an end line: their start lines no longer say it runs. */
export function endedWork(entries: readonly { kind: string; item?: TranscriptItem }[]): Set<number> {
  return new Set(
    entries.flatMap((e) => (e.kind === 'item' && e.item?.concurrent_work?.phase === 'ended' ? [e.item.concurrent_work.task_id] : [])),
  );
}

/** A task the agent did beside its own work, at its start and its end: one line that opens what was done, read only. A start
 * line whose work has ended says when it started. */
export function WorkLine({ work, at, over }: { work: Work; at: string; over: boolean }) {
  const title = useTask(work.task_id).data?.title;
  const commits = Hooks.useTaskChanges({ task: work.task_id }, { enabled: work.phase === 'ended' }).data?.commits.length;
  const now = useNow();
  const open = useOpenable(workOf(work.seat, work.task_id));
  const end = work.phase === 'ended' ? ENDED[work.ended ?? 'done'] : null;
  const running = !end && !over;
  return (
    <button
      type="button"
      {...open}
      className="my-1 flex max-w-full items-center gap-2 self-start rounded-lg border border-border px-3 py-2 text-left text-sm hover:bg-hover/30"
    >
      <span className="grid size-4 shrink-0 place-items-center">
        {end ? end.icon : <ActivityIcon act={running ? 'working' : 'resting'} />}
      </span>
      <span className="shrink-0 text-muted">{end ? end.word : running ? 'Working alongside on' : 'Started alongside:'}</span>
      <span className="min-w-0 truncate font-medium text-fg">
        T{work.task_id}
        {title && ` ${title}`}
      </span>
      <span className="shrink-0 text-muted">{end ? (commits ? `· ${count(commits, 'commit')}` : '') : `· ${ago(at, now)}`}</span>
      <ChevronRight size={14} className="shrink-0 text-muted" />
    </button>
  );
}
