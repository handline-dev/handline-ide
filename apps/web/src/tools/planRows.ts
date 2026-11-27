// The Plan strip's rows: every live sub-task in the order the plan runs, merged first.
import type { Task } from '@handline/client';
import type { Word } from '#src/tools/storyWords.ts';

/** More merged steps than this fold into one "N merged" row. */
export const FOLD_MERGED = 2;
/** A plan with more steps than this starts closed to one line. */
export const OPEN_UP_TO = 6;

export type Group = 'merged' | 'you' | 'working' | 'waiting' | 'draft';
export type PlanRow = { kind: 'step'; task: Task; word: Word; group: Group } | { kind: 'merged'; n: number };

const ORDER: Group[] = ['merged', 'you', 'working', 'waiting', 'draft'];

export function groupOf(t: Task, w: Word): Group {
  if (t.state === 'done') return 'merged';
  if (w.kind === 'you') return 'you';
  if (t.state === 'draft') return 'draft';
  if (w.kind === 'waiting') return 'waiting';
  return 'working';
}

/** Discarded steps drop out; merged ones fold to one row when there are many; the rest go yours, working, waiting, drafts, each by id. */
export function planRows(kids: Task[], word: (t: Task) => Word): PlanRow[] {
  const steps = kids
    .filter((t) => t.state !== 'discarded')
    .map((t) => ({ kind: 'step' as const, task: t, word: word(t), group: groupOf(t, word(t)) }))
    .sort((a, b) => ORDER.indexOf(a.group) - ORDER.indexOf(b.group) || a.task.id - b.task.id);
  const merged = steps.filter((r) => r.group === 'merged');
  const rest = steps.filter((r) => r.group !== 'merged');
  return merged.length > FOLD_MERGED ? [{ kind: 'merged', n: merged.length }, ...rest] : steps;
}

/** The closed line: how many merged of how many, and the step that matters now (the first not merged). */
export function planHead(rows: PlanRow[]) {
  const steps = rows.filter((r) => r.kind === 'step');
  const merged = rows.reduce((n, r) => n + (r.kind === 'merged' ? r.n : r.group === 'merged' ? 1 : 0), 0);
  const total = steps.length + rows.reduce((n, r) => n + (r.kind === 'merged' ? r.n : 0), 0);
  const left = steps.filter((r) => r.group !== 'merged');
  return { merged, total, now: left[0] ?? null, more: Math.max(0, left.length - 1) };
}
