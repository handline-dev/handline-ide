// What an office says about its people beyond where they sit (Marks.tsx draws it): the mark over one on hold or stopped, the
// steps done of a task, and what the papers on a desk say on hover. Plain sums over what the town already holds, with no drawing in them.
import type { Task } from '@handline/client';
import type { Activity } from '#src/layout/agentList.ts';
import { WORDS, type Person } from '#src/town/model.ts';

/** The mark over a person: an hourglass on hold, a warning when stopped on a fault. */
export type Mark = 'held' | 'error';
export const markOf = (p: Pick<Person, 'activity'>): Mark | null => (p.activity === 'held' || p.activity === 'error' ? p.activity : null);

const STUCK: Activity[] = ['held', 'error'];

/** What hovering a person or its desk says: its name, its state when that needs a look, what it is doing and why it waits. */
export function tipOf(p: Person): string {
  const state = STUCK.includes(p.activity) || p.activity === 'you' ? WORDS[p.activity] : null;
  const parts = [p.name, state, p.doing?.trim() || (state ? null : WORDS[p.activity]), state ? p.why?.trim() : null];
  return parts.filter((t, i) => t && parts.indexOf(t) === i).join(' · ');
}

/** How many of a person's tasks wait behind the one it holds, in words; null when none does. */
export function waitingOf(p: Person): string | null {
  const n = Math.max(0, p.queue - (p.task ? 1 : 0));
  return n ? `${n} ${n === 1 ? 'task' : 'tasks'} waiting` : null;
}

/** A person's model and effort ("sonnet · high") and, when known, how full its context is ("Context 25%"); null with none of them. */
export function kindOf(p: Person): string | null {
  const parts = [p.model, p.effort, p.ctx == null ? null : `Context ${Math.round(p.ctx)}%`].filter(Boolean);
  return parts.length ? parts.join(' · ') : null;
}

export type At = { x: number; y: number };

/** `text` in at most `most` letters, the last an ellipsis when it was longer. */
export function cutTo(text: string, most: number): string {
  const letters = [...text.trim()];
  return letters.length <= most
    ? letters.join('')
    : `${letters
        .slice(0, most - 1)
        .join('')
        .trimEnd()}…`;
}

/** The steps done of a task as the bar shows them: the count, and the part done from 0 to 1. Null without a count. */
export function stepsOf(todo: { done: number; total: number } | null | undefined): { text: string; part: number } | null {
  if (!todo || !(todo.total >= 1)) return null;
  const done = Math.min(Math.max(0, todo.done), todo.total);
  return { text: `${done}/${todo.total}`, part: done / todo.total };
}

/** The titles of the tasks `key` owns that are not done: the one being done first, then the oldest first. */
export function queueOf(tasks: readonly Pick<Task, 'id' | 'to' | 'state' | 'title'>[], key: string): string[] {
  const rank = (t: Pick<Task, 'state'>) => (t.state === 'doing' ? 0 : 1);
  return tasks
    .filter((t) => t.to === key && (t.state === 'doing' || t.state === 'queued'))
    .sort((a, b) => rank(a) - rank(b) || a.id - b.id)
    .map((t) => t.title);
}

/** What the papers on a desk say on hover: whose tasks they are, the first `most` by title, and how many more there are. */
export function papersTip(name: string, titles: readonly string[], most = 5): string {
  const more = titles.length - most;
  return [`${name}'s tasks`, ...titles.slice(0, most).map((t) => `· ${cutTo(t, 60)}`), ...(more > 0 ? [`+${more} more`] : [])].join('\n');
}
