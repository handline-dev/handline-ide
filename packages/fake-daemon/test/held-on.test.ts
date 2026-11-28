import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { holdPlaced } from '#src/heldon.ts';
import { applySeeds } from '#src/seeds.ts';
import { setup } from '#src/helpers.ts';

const TERM = 'handline.term-build';
const ARCH = 'handline.architect';

test('held_on names the lent machine an agent is on, and agent.restart_here starts it here only while that machine is silent', async () => {
  const { daemon, client } = setup();
  applySeeds(daemon, 'devices');
  daemon.admin.placed.set(ARCH, 'n-d-mini'); // on old-mini, offline
  holdPlaced(daemon);
  await client.start();
  const held = async (agent: string) => (await client.call('agent.get', { agent })).held_on;
  assert.equal(await held('handline.auditor'), null);
  assert.deepEqual(await held(TERM), { node: 'n-d-studio', machine: 'Studio', silent_since: null, restart_here: false });
  const mini = await held(ARCH);
  assert.deepEqual([mini?.machine, mini?.restart_here, typeof mini?.silent_since], ['old-mini', true, 'number']);

  const press = (agent: string, id: string) => client.call('agent.restart_here', { agent, request_id: id });
  await assert.rejects(press(TERM, 'rh-1'), (e: RpcError) => e.data?.gate === 'state' && /Studio is online/.test(e.message));
  await assert.rejects(press('handline.auditor', 'rh-2'), (e: RpcError) => e.data?.gate === 'state');
  assert.equal((await press(ARCH, 'rh-3')).said, 'started on mac');
  const after = await client.call('agent.get', { agent: ARCH });
  assert.deepEqual([after.held_on, after.session_open], [null, true]);
  const listed = (await client.call('agents.list', {})).agents.find((a) => a.key === ARCH);
  assert.equal(listed?.node, 'm-fake');
  const items = (await client.call('chat.transcript', { agent: ARCH })).items;
  assert.equal(items.at(-1)?.machine_moved?.from, 'old-mini');
  client.close();
});
