import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';

test('lessons.list shows an agent its live lessons, and lesson.remove takes one away', async () => {
  const { daemon, client } = setup();
  await client.start();
  daemon.tasks.lessons.set('a3', [
    { id: 1, agent: 'a3', text: 'show the screen in both themes', task: 12, created_at: '2026-11-18T00:00:00Z', created_by: 'a3' },
    { id: 2, agent: 'a3', text: 'say what is left out', created_at: '2026-11-18T00:01:00Z', created_by: 'a3' },
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
