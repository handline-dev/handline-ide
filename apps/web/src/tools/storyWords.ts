// The words a story's sub-task is shown with: one word per state, on its node, its header and the Plan line.
import { foundersTurn } from '@handline/app-core';
import type { Task, TaskDetail } from '@handline/client';
import { look } from '#src/ui/taskLook.tsx';

/** A sub-task as its parent's task.get carries it. */
export type Sub = TaskDetail['subtasks'][number];

export const startsAfter = (t: Task, open: Set<number>) => (t.blocked_by ?? []).filter((n) => open.has(n));

/** What a sub-task's word says: for you (Ready, a question, a plan to read), waiting on another, or neither. */
export type Word = { text: string; kind: 'you' | 'waiting' | 'other' };

/** The word a node, the sub-task's header and the Plan line give a sub-task: one word per state everywhere. */
export function wordOf(x: Task, sub: Sub | undefined, me: string, open: Set<number>): Word {
  if (sub?.review?.ready) return { text: 'Ready for you', kind: 'you' };
  const asks = x.state !== 'done' && foundersTurn(x, me) ? (x.turns ?? []).filter((r) => r.agent === me).length || 1 : 0;
  if (asks) return { text: `${asks} ${asks === 1 ? 'question' : 'questions'} for you`, kind: 'you' };
  if (x.state === 'draft') {
    const plan = sub?.plan_state ?? 'none';
    const text = { none: 'No plan yet', asked: 'Writing plan…', writing: 'Writing plan…', written: 'Plan to read', ok: 'Plan OK' }[plan];
    return { text, kind: plan === 'written' ? 'you' : 'other' };
  }
  const after = startsAfter(x, open);
  if (x.state !== 'done' && x.state !== 'discarded' && after.length)
    return { text: `Waiting on ${after.map((n) => `T${n}`).join(', ')}`, kind: 'waiting' };
  if (x.state === 'doing' && sub?.review?.redo) return { text: `Working · Redo ${sub.review.redo}`, kind: 'other' };
  return { text: look(x).word, kind: 'other' };
}
