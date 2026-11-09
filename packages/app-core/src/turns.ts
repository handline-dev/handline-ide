// The turns on a task (docs/design/turns.md): who acts now, and the chain below it, for a row or a task page.
import type { Task } from '@handline/client';
import { isPerson } from '#src/chat.ts';

export type Turns = Pick<Task, 'to' | 'state' | 'acting' | 'turns'>;

/** The founder acts on the task now: the top turn is theirs, so Give back can pass it. */
export const foundersTurn = (t: Turns, me: string): boolean =>
  t.state !== 'done' && t.state !== 'discarded' && !!t.turns?.length && isPerson(t.acting ?? '', me);

/** `founder ← handline.ui`: the top turn first, then each one below down to the owner; empty when no turn is open. */
export function turnChain(t: Turns, me: string): string {
  if (!t.turns?.length) return '';
  const who = (n: string) => (isPerson(n, me) ? 'founder' : n);
  return [...t.turns.map((x) => x.agent).reverse(), t.to].map(who).join(' ← ');
}
