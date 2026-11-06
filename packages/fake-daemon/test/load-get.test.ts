import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';

test('load.get gives each agent its processes, the unowned ones apart, and the totals add up', async () => {
  const { client } = setup();
  await client.start();
  const got = await client.call('load.get', {});
  assert.ok(got);
  assert.equal(got.every_s, 5);
  assert.ok(got.agents.length > 0);
  const sum = got.agents.reduce((n, a) => n + a.processes, 0) + got.unowned.processes;
  assert.equal(got.total.processes, sum);
  assert.equal(got.unowned.top[0]?.agent, null);
  assert.equal(got.agents[0]?.top[0]?.found_by, 'env');
  const one = await client.call('load.get', { agent: got.agents[0]!.agent, minutes: 60 });
  assert.deepEqual(
    one?.agents.map((a) => a.agent),
    [got.agents[0]!.agent],
  );
  assert.ok(one?.series);
  client.close();
});

test('load.get refuses a window that is not 0 to 10080 whole minutes', async () => {
  const { client } = setup();
  await client.start();
  for (const minutes of [-1, 10081, 1.5]) {
    await assert.rejects(client.call('load.get', { minutes }), (e: RpcError) => e.code === 'bad_request');
  }
  client.close();
});
