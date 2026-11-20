// The Run window of a plan's drafts, and a draft's own Run.
import { useNameOf, useWrite } from '@handline/app-data';
import type { Task, TaskDetail } from '@handline/client';
import { Play } from 'lucide-react';
import { startsAfter } from '#src/tools/storyWords.ts';
import { Button } from '#src/ui/Button.tsx';
import { ModalFrame, openModal } from '#src/ui/modal.tsx';
import { sayRefusal } from '#src/ui/refusal.tsx';

/** A draft's Run, at its page's title: Run is the OK of its plan, so it is the one button. */
export function DraftRun({ t }: { t: TaskDetail }) {
  return (
    <Button variant="primary" className="shrink-0" onClick={() => run(t.parent ?? t.id, [t], new Set())}>
      <Play size={14} />
      Run
    </Button>
  );
}

/** The Run window: each draft, who gets it and when it starts; Run N tasks queues them. */
export function run(parent: number, drafts: Task[], open: Set<number>) {
  void openModal<boolean>((close) => <RunWindow parent={parent} drafts={drafts} open={open} close={close} />);
}

function RunWindow({ parent, drafts, open, close }: { parent: number; drafts: Task[]; open: Set<number>; close: (v?: boolean) => void }) {
  const nameOf = useNameOf();
  const queue = useWrite('task.queue');
  const comment = useWrite('task.comment.add', String(parent));
  const now = drafts.filter((d) => !startsAfter(d, open).length).length;
  const go = () =>
    queue.mutate(
      { tasks: drafts.map((d) => d.id) },
      {
        onSuccess: () => {
          comment.mutate(
            { task: parent, text: `Ran ${drafts.length} tasks: ${drafts.map((d) => `T${d.id}`).join(', ')}` },
            { onError: sayRefusal },
          );
          close(true);
        },
        onError: sayRefusal,
      },
    );
  return (
    <ModalFrame
      title="Run this plan"
      size="xl"
      onClose={() => close()}
      footer={
        <>
          <span className="mr-auto self-center text-sm text-muted">
            {now} start now, {drafts.length - now} wait.
          </span>
          <Button onClick={() => close()}>Cancel</Button>
          <Button variant="primary" autoFocus disabled={queue.isPending} onClick={go}>
            <Play size={14} className="mr-1.5" />
            Run {drafts.length} tasks
          </Button>
        </>
      }
    >
      <table className="w-full text-sm">
        <thead className="text-left text-xs text-muted">
          <tr className="border-b border-border">
            <th className="py-2 font-normal" />
            <th className="py-2 font-normal">Task</th>
            <th className="py-2 font-normal">Goes to</th>
            <th className="py-2 font-normal">Starts</th>
          </tr>
        </thead>
        <tbody>
          {drafts.map((d) => {
            const after = startsAfter(d, open);
            return (
              <tr key={d.id} className="border-b border-border">
                <td className="py-2 pr-3 text-muted">T{d.id}</td>
                <td className="py-2 pr-3">{d.title}</td>
                <td className="py-2 pr-3">{nameOf(d.to)}</td>
                <td className={`py-2 ${after.length ? 'text-muted' : ''}`}>
                  {after.length ? `after ${after.map((x) => `T${x}`).join(', ')}` : 'now'}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </ModalFrame>
  );
}
