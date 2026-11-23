import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';

const AUD = 'handline.auditor';

test('agent.set takes sandbox_allow as allowance words, agent.get reads them back, and an unknown word is bad_request', async () => {
  const { client } = setup();
  await client.start();
  assert.equal((await client.call('agent.get', { agent: AUD })).settings?.sandbox_allow, '');
  await client.call('agent.set', { agent: AUD, values: { sandbox_allow: 'weaker-network pnpm-install' }, request_id: 's1' });
  assert.equal((await client.call('agent.get', { agent: AUD })).settings?.sandbox_allow, 'weaker-network pnpm-install');
  await assert.rejects(
    client.call('agent.set', { agent: AUD, values: { sandbox_allow: 'everything' }, request_id: 's2' }),
    (e: RpcError) => e.code === 'bad_request',
  );
  client.close();
});
