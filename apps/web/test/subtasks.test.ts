import { expect, it } from 'vitest';
import { outsideBlockers, subtaskCount } from '#src/tools/subtasks.ts';

it('leaves the sub-tasks out of what a task waits on', () => {
  const b = (task: number) => ({
    task,
    reason: '',
    owner: 'a',
    title: `T${task}`,
    state: 'queued',
    waiting: null,
    hold_reason: null,
    block_reason: null,
    evidence: null,
  });
  expect(outsideBlockers({ blockers: [b(1), b(2), b(3)], children: [2, 3] }).map((x) => x.task)).toEqual([1]);
  expect(outsideBlockers({ blockers: [b(2)], children: [2] })).toEqual([]);
});

it('counts sub-tasks done of those not discarded', () => {
  expect(subtaskCount([{ state: 'done' }, { state: 'doing' }, { state: 'discarded' }, { state: 'done' }])).toBe('2/3 done');
  expect(subtaskCount([{ state: 'discarded' }])).toBeNull();
});
