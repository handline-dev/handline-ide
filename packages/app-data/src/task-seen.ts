// What opening a task marks as read (task.seen): the newest comment, and again when a newer one arrives while the page is open.
import { useEffect, useRef } from 'react';
import { useTask } from '#src/read.ts';
import { useWrite } from '#src/write.ts';

/** Marks the task's comments as seen once it has loaded and `active`, and again for each newer comment; never twice for one number. */
export function useTaskSeen(task: number, active = true): void {
  const mark = useWrite('task.seen', String(task));
  const t = useTask(task).data;
  const newest = t ? Math.max(0, ...t.memos.map((m) => m.id)) : 0;
  const told = useRef(0);
  useEffect(() => {
    told.current = 0;
  }, [task]);
  useEffect(() => {
    if (!active || !t || t.unread === undefined || newest <= told.current) return;
    if (t.unread === 0 && told.current === 0) {
      told.current = newest; // nothing above the marker
      return;
    }
    told.current = newest;
    mark.mutate({ task, upto: newest });
  }, [active, t, newest, task]); // eslint-disable-line react-hooks/exhaustive-deps
}
