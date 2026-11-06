import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';

test('tasks.list ids returns only those tasks, newest id first, leaving out an id it does not know', async () => {
  const { client } = setup();
  await client.start();
  const all = (await client.call('tasks.list', { all: true, limit: 200 })).items;
  const [a, b] = [all[0]!, all[all.length - 1]!];
  const page = await client.call('tasks.list', { ids: [b.id, 999999999, a.id] });
  assert.deepEqual(
    page.items.map((t) => t.id),
    [a.id, b.id].sort((x, y) => y - x),
  );
  assert.equal(page.more, false);
});

test('tasks.list ids refuses what goes with it', async () => {
  const { client } = setup();
  await client.start();
  const list = (p: Record<string, unknown>) => client.call('tasks.list', p as never);
  for (const [p, field] of [
    [{ ids: [1], before: 5 }, 'before'],
    [{ ids: [1], cursor: 'x' }, 'cursor'],
    [{ ids: [1], order: 'closed', state: 'done' }, 'order'],
    [{ ids: 'x' }, 'ids'],
    [{ ids: [1, 'x'] }, 'ids'],
    [{ ids: [1.5] }, 'ids'],
    [{ ids: Array.from({ length: 201 }, (_, i) => i + 1) }, 'ids'],
  ] as const)
    await assert.rejects(list(p), (e: RpcError) => e.data?.field === field);
});

test('tasks.list ids with no limit returns all of them, up to 200', async () => {
  const { client } = setup();
  await client.start();
  const ids: number[] = [];
  for (let n = 0; n < 60; n++) {
    const made = await client.call('task.new', { owner: 'handline.architect', title: `t${n}`, body: 'b', request_id: `n${n}` });
    ids.push(made.task as number);
  }
  const page = await client.call('tasks.list', { ids });
  assert.equal(page.items.length, 60);
  assert.equal(page.more, false);
});
