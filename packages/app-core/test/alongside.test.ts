import { test } from 'node:test';
import assert from 'node:assert/strict';
import { runningAlongside } from '#src/index.ts';

const at = (n: number) => `2026-01-01T00:00:0${n}Z`;
const side = (n: number, task: number, phase: 'started' | 'ended') => ({ at: at(n), concurrent_work: { task_id: task, seat: 's', phase } });

test('only work started and not yet ended runs alongside, oldest first', () => {
  const items = [side(1, 7, 'started'), { at: at(2) }, side(3, 8, 'started'), side(4, 7, 'ended'), side(5, 9, 'started')];
  assert.deepEqual(runningAlongside(items as never), [
    { task: 8, at: at(3) },
    { task: 9, at: at(5) },
  ]);
});

test('a task started again after it ended runs again, from its new start', () => {
  const items = [side(1, 7, 'started'), side(2, 7, 'ended'), side(3, 7, 'started')];
  assert.deepEqual(runningAlongside(items as never), [{ task: 7, at: at(3) }]);
});
