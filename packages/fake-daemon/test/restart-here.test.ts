import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';

test('agent.restart_here is refused (gate state) for an agent on this machine, and not_found for no agent', async () => {
  const { client } = setup();
  await client.start();
  const agent = (await client.call('agents.list', {})).agents[0]!.key;
  await assert.rejects(
    client.call('agent.restart_here', { agent, request_id: 'rh-1' }),
    (e: RpcError) => e.code === 'refused' && (e.data as { gate?: string }).gate === 'state',
  );
  await assert.rejects(
    client.call('agent.restart_here', { agent: 'handline.nobody', request_id: 'rh-2' }),
    (e: RpcError) => e.code === 'not_found',
  );
  client.close();
});
