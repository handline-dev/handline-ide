// An agent whose task is finished while its office is on screen celebrates for a few seconds (Cast.tsx hops it, Cheer.tsx throws the
// confetti). Which task counts is a plain sum (`finishedWork`); the store keeps who cheers now outside React, as the pops do, so React
// draws only when a cheer starts or ends and the motion itself is CSS.
import type { Task } from '@handline/client';
import { useStore } from '@handline/app-react';
import { useEffect, useState, useSyncExternalStore } from 'react';

/** How long a cheer lasts. */
export const CHEER_MS = 3000;
/** How new a finish must be to cheer for: an older one is only being changed. */
const FRESH_MS = 60_000;
/** Some slack for the daemon's clock against this screen's. */
const SLACK_MS = 5_000;

/** Whether a task heard was just finished by its agent: done, in the last minute and since `since`. */
export const finishedWork = (t: Pick<Task, 'to' | 'state' | 'done'>, now: number, since = 0) => {
  const at = t.done ? Date.parse(t.done) : NaN;
  return !!t.to && t.state === 'done' && now - at < FRESH_MS && at >= since - SLACK_MS;
};

/** Who cheers, by key, and when each began (ms). */
export type Cheering = ReadonlyMap<string, number>;

export type Cheers = {
  get: () => Cheering;
  subscribe: (l: () => void) => () => void;
  cheer: (key: string) => void;
  stop: () => void;
};

/** The cheers of one office: each ends CHEER_MS after it began; one cheering again starts over. */
export function cheersOf(clock: () => number = Date.now): Cheers {
  const all = new Set<() => void>();
  const timers = new Map<string, ReturnType<typeof setTimeout>>();
  let now: Cheering = new Map();
  const emit = () => all.forEach((l) => l());
  const end = (key: string) => {
    timers.delete(key);
    const next = new Map(now);
    next.delete(key);
    now = next;
    emit();
  };
  return {
    get: () => now,
    subscribe: (l) => (all.add(l), () => void all.delete(l)),
    cheer: (key) => {
      clearTimeout(timers.get(key));
      timers.set(
        key,
        setTimeout(() => end(key), CHEER_MS),
      );
      now = new Map(now).set(key, clock());
      emit();
    },
    stop: () => {
      timers.forEach(clearTimeout);
      timers.clear();
      now = new Map();
      emit();
    },
  };
}

/** How far into an animation begun at `began` (ms) it is when this is first drawn: one drawn late (a person sitting down mid-cheer)
 *  skips that much, so it ends on time. One that begins while this is drawn starts from the start. */
export function useElapsed(began: number | undefined): number {
  const [first] = useState(() => ({ began, elapsed: began === undefined ? 0 : Math.max(0, Date.now() - began) }));
  return first.began === began ? first.elapsed : 0;
}

/** Who in the office cheers now: each agent whose task is finished while this is on screen, once per finish. */
export function useCheers(): Cheering {
  const { client } = useStore();
  const [cheers] = useState(() => cheersOf());
  useEffect(() => {
    const [seen, since] = [new Set<string>(), Date.now()];
    const off = client.onEvent((e) => {
      if (e.type !== 'task.changed') return;
      const t = e.data as unknown as Task;
      const once = `${t.id}:${t.done}`;
      if (seen.has(once) || !finishedWork(t, Date.now(), since)) return;
      seen.add(once);
      cheers.cheer(t.to);
    });
    return () => {
      off();
      cheers.stop();
    };
  }, [client, cheers]);
  return useSyncExternalStore(cheers.subscribe, cheers.get);
}
