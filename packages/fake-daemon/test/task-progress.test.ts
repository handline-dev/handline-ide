import { test } from 'node:test';
import assert from 'node:assert/strict';
import { setup } from '#src/helpers.ts';
import { applySeeds } from '#src/seeds.ts';

test("task.get carries the working session's to-do list; tasks.list does not, and a task with no list has none", async () => {
  const { client, daemon } = setup({ client: 'boss' });
  applySeeds(daemon, 'task-progress');
  await client.start();
  const p = (await client.call('task.get', { task: 8871 })).progress!;
  assert.deepEqual([p.done, p.total, p.current], [3, 5, 'Write the grouping test']);
  assert.equal(typeof p.updated_at, 'string');
  const { items } = await client.call('tasks.list', {});
  assert.equal('progress' in items.find((t) => t.id === 8871)!, false);
  const { task } = await client.call('task.new', { owner: 'handline.architect', title: 'no list', request_id: 'p1' });
  assert.equal('progress' in (await client.call('task.get', { task })), false);
  client.close();
});
