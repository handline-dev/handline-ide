import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';

const AUD = 'handline.auditor';

test('agent.get reads session_mode as terminal, agent.set takes chat and terminal, and refuses any other mode', async () => {
  const { client } = setup();
  await client.start();
  const mode = async () => (await client.call('agent.get', { agent: AUD })).settings?.session_mode;
  assert.equal(await mode(), 'terminal');
  await client.call('agent.set', { agent: AUD, values: { session_mode: 'chat' }, request_id: 'm1' });
  assert.equal(await mode(), 'chat');
  await client.call('agent.set', { agent: AUD, values: { session_mode: 'terminal' }, request_id: 'm2' });
  assert.equal(await mode(), 'terminal');
  await assert.rejects(
    client.call('agent.set', { agent: AUD, values: { session_mode: 'headless' as 'chat' }, request_id: 'm3' }),
    (e: RpcError) => e.code === 'bad_request',
  );
  client.close();
});
