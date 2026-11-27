import type { Task } from '@handline/client';
import { expect, it } from 'vitest';
import { planHead, planRows } from '#src/tools/planRows.ts';
import type { Word } from '#src/tools/storyWords.ts';

const task = (id: number, state: string) => ({ id, state, title: `T${id}` }) as Task;
const words: Record<number, Word> = { 5: { text: 'Ready for you', kind: 'you' }, 6: { text: 'Waiting on T4', kind: 'waiting' } };
const word = (t: Task): Word => words[t.id] ?? { text: t.state, kind: 'other' };

it('orders merged, yours, working, waiting, drafts and drops discarded', () => {
  const kids = [task(7, 'draft'), task(6, 'queued'), task(4, 'doing'), task(5, 'doing'), task(2, 'done'), task(3, 'discarded')];
  expect(planRows(kids, word).map((r) => (r.kind === 'step' ? r.task.id : 'm'))).toEqual([2, 5, 4, 6, 7]);
});

it('folds more than two merged steps into one row', () => {
  const kids = [task(1, 'done'), task(2, 'done'), task(3, 'done'), task(4, 'doing')];
  const rows = planRows(kids, word);
  expect(rows[0]).toEqual({ kind: 'merged', n: 3 });
  expect(rows).toHaveLength(2);
  expect(planHead(rows)).toMatchObject({ merged: 3, total: 4, more: 0 });
  expect(planHead(rows).now?.task.id).toBe(4);
});

it('keeps one or two merged steps as their own rows', () => {
  const rows = planRows([task(1, 'done'), task(2, 'done'), task(4, 'queued'), task(5, 'doing')], word);
  expect(rows.every((r) => r.kind === 'step')).toBe(true);
  expect(planHead(rows)).toMatchObject({ merged: 2, total: 4, more: 1 });
});
