import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';
import { applySeeds } from '#src/seeds.ts';

test('a calm machine says so in daemon.status, lists its runs on time and holds none', async () => {
  const { client } = setup();
  await client.start();
  const status = await client.call('daemon.status', {});
  assert.deepEqual(
    [status.run_load?.policy, status.run_load?.slow, status.run_load?.since, status.run_load?.cap, status.run_load?.waiting],
    ['observe', false, null, null, 0],
  );
  assert.equal(status.health, undefined);
  const load = await client.call('run.load', {});
  assert.ok(load);
  assert.equal(load.runs.length, status.run_load?.heavy);
  assert.ok(load.runs.every((r) => !r.late && r.heavy));
  assert.deepEqual(load.waiting, []);
  client.close();
});

test('the slow-mac seed makes the machine slow: late runs, health near, and a run waiting once a policy holds runs', async () => {
  const { client, daemon } = setup();
  applySeeds(daemon, 'slow-mac');
  await client.start();
  const status = await client.call('daemon.status', {});
  assert.equal(status.run_load?.slow, true);
  assert.deepEqual(status.run_load?.causes, ['runs', 'loop']);
  assert.equal(status.run_load?.cap, null);
  assert.equal(status.health?.level, 'near');
  const load = await client.call('run.load', {});
  assert.ok(load && load.late >= 1 && load.late === load.runs.filter((r) => r.late).length);
  assert.deepEqual(load.waiting, []);
  await client.call('settings.set', { changes: { heavy_runs_policy: 'turns' }, request_id: 'rl-1' });
  const held = await client.call('run.load', {});
  assert.equal(held?.cap, Math.max(2, load.runs.length - 1));
  assert.equal(held?.waiting.length, 1);
  assert.equal((await client.call('daemon.status', {})).run_load?.waiting, 1);
  await client.call('settings.set', { changes: { heavy_runs_policy: 'fixed', heavy_runs_at_once: 2 }, request_id: 'rl-2' });
  assert.equal((await client.call('run.load', {}))?.cap, 2);
  client.close();
});

test('the heavy runs settings take only their own values, and run.admit lets a run go', async () => {
  const { client } = setup();
  await client.start();
  for (const changes of [{ heavy_runs_policy: 'stop' as never }, { heavy_runs_at_once: -1 }, { heavy_runs_at_once: 1.5 }])
    await assert.rejects(
      client.call('settings.set', { changes, request_id: `rl-${JSON.stringify(changes)}` }),
      (e: RpcError) => e.code === 'bad_request',
    );
  assert.deepEqual(await client.call('run.admit', { agent: 'worker', id: 'toolu_1', shape: 'check.sh' }), { go: true, waited_s: 0 });
  await assert.rejects(client.call('run.admit', { agent: 'worker', id: '', shape: 'check.sh' }), (e: RpcError) => e.code === 'bad_request');
  client.close();
});
