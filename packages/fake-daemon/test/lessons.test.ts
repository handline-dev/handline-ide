import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';
import { LEARNING_SEEDS } from '#src/seeds-learning.ts';

test('lessons.list shows an agent its live lessons, and lesson.remove takes one away', async () => {
  const { daemon, client } = setup();
  await client.start();
  daemon.tasks.lessons.set('a3', [
    {
      id: 1,
      agent: 'a3',
      text: 'show the screen in both themes',
      task: 12,
      created_at: '2026-11-18T00:00:00Z',
      count: 1,
      created_by: 'a3',
    },
    { id: 2, agent: 'a3', text: 'say what is left out', created_at: '2026-11-18T00:01:00Z', count: 1, created_by: 'a3' },
  ]);
  const before = await client.call('lessons.list', { agent: 'a3' });
  assert.deepEqual(
    before.items.map((l) => l.id),
    [1, 2],
  );
  assert.ok(before.bytes > 0 && before.cap === 3072);
  await client.call('lesson.remove', { id: 1, request_id: 'r1' });
  assert.deepEqual(
    (await client.call('lessons.list', { agent: 'a3' })).items.map((l) => l.id),
    [2],
  );
  await assert.rejects(client.call('lesson.remove', { id: 1, request_id: 'r2' }), (e: RpcError) => e.data?.what === 'lesson');
});

test("task.get's timeline has a row for each lesson added on the task, removed later or not", async () => {
  const { daemon, client } = setup();
  await client.start();
  const task = (await client.call('tasks.list', {})).items[0]!.id;
  daemon.tasks.lessons.set('a3', [
    { id: 1, agent: 'a3', text: 'show the screen in both themes', task, created_at: '2026-11-18T00:00:00Z', count: 1, created_by: 'a3' },
    { id: 2, agent: 'a3', text: 'on no task', created_at: '2026-11-18T00:01:00Z', count: 1, created_by: 'a3' },
  ]);
  const rows = async () =>
    (await client.call('task.get', { task })).timeline.filter((r) => r.kind === 'lesson').map((r) => [r.who, r.text]);
  assert.deepEqual(await rows(), [['a3', 'show the screen in both themes']]);
  await client.call('lesson.remove', { id: 1, request_id: 'r1' });
  assert.deepEqual(await rows(), [['a3', 'show the screen in both themes']], 'a removed lesson stays in the history');
});

test("lessons.list of human is the founder's preferences, measured under their own heading", async () => {
  const { daemon, client } = setup();
  await client.start();
  daemon.tasks.lessons.set('p1', [
    {
      id: 3,
      agent: 'p1',
      text: 'write in Korean (the founder, comment 7)',
      created_at: '2026-11-18T00:00:00Z',
      count: 1,
      created_by: 'a3',
    },
  ]);
  const got = await client.call('lessons.list', { agent: 'human' });
  assert.deepEqual(
    got.items.map((l) => l.id),
    [3],
  );
  assert.ok(got.bytes > "# The founder's preferences".length);
});

test("the learning seed's founder rules carry their count and a quote that opens its comment", async () => {
  const { daemon, client } = setup();
  await client.start();
  LEARNING_SEEDS.learning!(daemon);
  const items = (await client.call('lessons.list', { agent: 'human' })).items;
  const quoted = items.find((l) => l.quote?.comment != null)!;
  assert.ok(quoted.count > 1);
  const memos = (await client.call('task.get', { task: quoted.quote!.task! })).timeline.filter((r) => r.kind === 'memo');
  assert.ok(
    memos.some((r) => r.memo === quoted.quote!.comment),
    'the quote names a comment the task has',
  );
  assert.ok(items.some((l) => l.quote?.message?.startsWith('msg_')) && items.some((l) => !l.quote && l.count === 1));
});
