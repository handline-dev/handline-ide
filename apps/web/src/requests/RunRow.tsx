// A draft only the founder can run, as a Requests row: its line, its Run, and the tasks any row holds up.
import { useTasks, useWrite } from '@handline/app-data';
import type { PendingRequest } from '@handline/client';
import { Play } from 'lucide-react';
import type { ReactNode } from 'react';
import { useRun } from '#src/tools/RunPlan.tsx';
import { Button } from '#src/ui/Button.tsx';
import { Linked } from '#src/ui/Markdown.tsx';
import { sayRefusal } from '#src/ui/refusal.tsx';

const list = (ids: number[]) => ids.map((n) => `T${n}`).join(', ');

/** A run row's line when it asks nothing: what waits on the draft, or who left it. */
export function runLine(p: PendingRequest, name: string): string {
  const holds = p.holds ?? [];
  if (holds.length) return `${list(holds)} ${holds.length === 1 ? 'waits' : 'wait'} on it`;
  return `New draft from ${name}`;
}

/** The tasks waiting behind a row, each a link, and a run row's answers at its end; nothing when neither. */
export function Holds({ p, answers }: { p: PendingRequest; answers?: ReactNode }) {
  if (!p.holds?.length && !answers) return null;
  return (
    <span className="flex flex-wrap items-center gap-x-3 gap-y-1 px-2 pb-3 text-sm text-muted">
      {p.holds?.length ? <Linked text={`Holds ${list(p.holds)}`} /> : null}
      {answers && <span className="ml-auto flex items-center gap-2">{answers}</span>}
    </span>
  );
}

/** Run: a lone draft is queued at once; a parent whose drafts are under it opens their Run window. */
export function RunButton({ task }: { task: number }) {
  const queue = useWrite('task.queue');
  const { run } = useRun();
  const listed = useTasks({ parent: task, all: true }).data;
  const kids = listed?.items ?? [];
  const drafts = kids.filter((x) => x.state === 'draft');
  const go = () => {
    if (drafts.length) {
      const open = new Set(kids.filter((x) => !['done', 'discarded'].includes(x.state)).map((x) => x.id));
      return run(task, drafts, open);
    }
    queue.mutate({ tasks: [task] }, { onError: sayRefusal });
  };
  return (
    <Button size="sm" variant="primary" disabled={queue.isPending || !listed} onClick={go}>
      <Play size={12} />
      Run
    </Button>
  );
}
