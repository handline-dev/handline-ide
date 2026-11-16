import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Task } from '@handline/client';
import { taskAfter } from '#src/tasks.ts';

const NOW = Date.parse('2026-01-01T12:00:00Z');
const ago = (min: number) => new Date(NOW - min * 60_000).toISOString();
const task = (id: number, over: Partial<Task> = {}): Task => ({
  id,
  from: 'founder',
  to: 'a',
  title: `task ${id}`,
  state: 'queued',
  time_box: '2h',
  parent: null,
  created: ago(600),
  delivered: null,
  done: null,
  hold_reason: null,
  block_reason: null,
  evidence: null,
  ...over,
});

test('editing the priority shows it at once', () => {
  assert.equal(taskAfter('edit', task(1), 'now', { priority: 1 })?.priority, 1);
  assert.equal(taskAfter('edit', task(1, { priority: 3 }), 'now', { title: 'x' })?.priority, 3);
});
