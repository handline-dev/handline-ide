// Each task change heard while a screen is open, with the history rows that made it: the newest change of the task, all rows
// written at that moment. A chat draws them as the agent's task card.
import type { HistoryRow, Task } from '@handline/client';
import { useStore } from '@handline/app-react';
import { useEffect, useEffectEvent } from 'react';

export function useTaskChanges(take: (task: Task, rows: HistoryRow[]) => void): void {
  const { client } = useStore();
  const took = useEffectEvent(take);
  useEffect(
    () =>
      client.onEvent((e) => {
        if (e.type !== 'task.changed') return;
        const t = e.data as unknown as Task;
        void client.call('task.history', { task: t.id }).then(({ rows }) => {
          const at = rows[0]?.at;
          if (at)
            took(
              t,
              rows.filter((r) => r.at === at),
            );
        });
      }),
    [client],
  );
}
