// A parent task waits on each open sub-task, so task.get lists them as blockers too.
import type { TaskDetail } from '@handline/client';

/** The blockers that are not the task's own sub-tasks: what it really waits on. */
export const outsideBlockers = (t: Pick<TaskDetail, 'blockers' | 'children'>) => t.blockers.filter((b) => !t.children.includes(b.task));

/** "2/3 done": sub-tasks done of those not discarded; null when every one was discarded. */
export function subtaskCount(subtasks: { state: string }[]): string | null {
  const counted = subtasks.filter((s) => s.state !== 'discarded');
  return counted.length ? `${counted.filter((s) => s.state === 'done').length}/${counted.length} done` : null;
}
