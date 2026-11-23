import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';

const A = 'handline.term-build';

test('agent.jobs lists what a test set; a shell stops alone, another job only with all of them', async () => {
  const { client, daemon } = setup();
  await client.start();
  await client.startSession(A);
  const s = daemon.sessions.get(A)!;
  s.jobs = [
    { id: 'b1', type: 'shell', name: 'shell: build', started: 10, output: ['ok'] },
    { id: 'a1', type: 'agent', name: 'agent: review', started: null, output: null },
  ];
  assert.deepEqual(
    (await client.call('agent.jobs', { agent: A })).jobs.map((j) => j.id),
    ['b1', 'a1'],
  );
  await assert.rejects(
    client.call('agent.job.stop', { agent: A, job: 'a1', request_id: 'aj-1' }),
    (e: RpcError) => e.code === 'refused' && e.gate === 'type',
  );
  await client.call('agent.job.stop', { agent: A, job: 'b1', request_id: 'aj-2' });
  assert.deepEqual(
    (await client.call('agent.jobs', { agent: A })).jobs.map((j) => j.id),
    ['a1'],
  );
  await client.call('agent.jobs.stop', { agent: A, request_id: 'aj-3' });
  assert.deepEqual(
    (await client.call('agent.jobs', { agent: A })).jobs.map((j) => j.id),
    ['a1'],
  );
  s.jobs = [];
  await assert.rejects(
    client.call('agent.jobs.stop', { agent: A, request_id: 'aj-4' }),
    (e: RpcError) => e.code === 'refused' && e.gate === 'state',
  );
  client.close();
});
