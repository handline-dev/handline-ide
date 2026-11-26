// Work one agent gives another, as the daemon reports it: a task heard for the first time, just opened, by one agent for another. The
// office (Inside.tsx) shows each as an errand: the giver walks over, talks and walks back (crowd.ts).
import type { Task } from '@handline/client';
import { useStore } from '@handline/app-react';
import { useEffect, useEffectEvent } from 'react';

/** How new a task must be to count as just given: older ones are only being changed. */
const FRESH_MS = 60_000;

/** Some slack for the daemon's clock against this screen's. */
const SLACK_MS = 5_000;

/** Whether a task heard is work just given by one agent to another: made since `since` and in the last minute, waiting or being done,
 *  and from someone else. */
export const givenWork = (t: Pick<Task, 'from' | 'to' | 'state' | 'created'>, now: number, since = 0) => {
  const made = Date.parse(t.created);
  return (
    !!t.from &&
    !!t.to &&
    t.from !== t.to &&
    (t.state === 'queued' || t.state === 'doing') &&
    now - made < FRESH_MS &&
    made >= since - SLACK_MS
  );
};

/** Calls `take(from, to)` once for each task given while the screen is open. */
export function useGivenWork(take: (from: string, to: string) => void): void {
  const { client } = useStore();
  const took = useEffectEvent(take);
  useEffect(() => {
    // Only work given since the screen opened: one made before is only being changed now.
    const [seen, since] = [new Set<number>(), Date.now()];
    return client.onEvent((e) => {
      if (e.type !== 'task.changed') return;
      const t = e.data as unknown as Task;
      if (seen.has(t.id)) return;
      seen.add(t.id);
      if (givenWork(t, Date.now(), since)) took(t.from, t.to);
    });
  }, [client]);
}
