// Who in the whole town is doing what, for the town's top strip (StatusBar.tsx): the people at work, those waiting for the founder, those
// stopped on a fault and those held by a wait by design, each with its house; and the work finished today. Pure.
import type { Task } from '@handline/client';
import type { House, Person } from '#src/town/model.ts';

/** A group of the strip, in the order it shows them. */
export const GROUPS = ['working', 'you', 'error', 'held'] as const;
export type Group = (typeof GROUPS)[number];
export type Called = { person: Person; house: House };
export type RollCall = Record<Group, Called[]>;

/** One starting counts as at work: it will be in a moment. */
const groupOf = (p: Person): Group | null =>
  p.activity === 'working' || p.activity === 'waking'
    ? 'working'
    : p.activity === 'you' || p.activity === 'error' || p.activity === 'held'
      ? p.activity
      : null;

/** Everyone of `houses` by group, house by house in the town's order; one resting or asleep is in none. */
export function rollCallOf(houses: readonly House[]): RollCall {
  const out: RollCall = { working: [], you: [], error: [], held: [] };
  for (const house of houses)
    for (const person of house.people) {
      const g = groupOf(person);
      if (g) out[g].push({ person, house });
    }
  return out;
}

/** What a group's people are doing, in words, after their number: "3 working", "1 waiting for you". */
export const GROUP_WORDS: Record<Group, string> = { working: 'working', you: 'waiting for you', error: 'stopped', held: 'on hold' };
export const groupWords = (g: Group, n: number): string => `${n} ${GROUP_WORDS[g]}`;

export type Finished = Pick<Task, 'id' | 'title' | 'to' | 'state' | 'done'>;

/** The tasks of `tasks` done since the midnight before `now` (this screen's own clock and day), newest first; a discarded one is not
 *  work finished. */
export function doneToday<T extends Finished>(tasks: readonly T[], now: number): T[] {
  const midnight = new Date(now).setHours(0, 0, 0, 0);
  const at = (t: T) => (t.done ? Date.parse(t.done) : NaN);
  return tasks.filter((t) => t.state === 'done' && at(t) >= midnight).sort((a, b) => at(b) - at(a));
}
