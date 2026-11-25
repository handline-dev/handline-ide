import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';

test('shell.open with no agent opens a shell in the home folder, and a command without an agent is refused', async () => {
  const { client } = setup();
  await client.start();
  const got = await client.call('shell.open', { request_id: 'h1' });
  assert.equal(got.agent, null);
  assert.equal(got.cwd, '/fake/home');
  await client.call('shell.close', { terminal: got.terminal, request_id: 'h2' });
  await assert.rejects(
    client.call('shell.open', { command: 'ls', block: '1:0', request_id: 'h3' }),
    (e: RpcError) => e.data?.gate === 'state',
  );
});
