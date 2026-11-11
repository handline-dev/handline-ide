import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';

test('account.read_usage accepts a logged-in account, refuses one that needs a login or is STALLED (state), and an unknown one is not_found', async () => {
  const { client, daemon } = setup();
  await client.start();
  await client.call('account.add', { account: 'fresh', request_id: 'r0' });
  const ok = (await client.call('account.read_usage', { account: 'alpha', request_id: 'r1' })) as { accepted: boolean };
  assert.equal(ok.accepted, true);
  const out = (await client.call('account.read_usage', { account: 'fresh', request_id: 'r2' }).catch((e: RpcError) => e)) as RpcError;
  assert.equal((out.data as { gate?: string }).gate, 'state');
  daemon.accounts.stall('alpha', { reason: 'login expired', error: 'authentication_failed' });
  const stalled = (await client.call('account.read_usage', { account: 'alpha', request_id: 'r4' }).catch((e: RpcError) => e)) as RpcError;
  assert.equal((stalled.data as { gate?: string }).gate, 'state');
  const missing = (await client.call('account.read_usage', { account: 'nope', request_id: 'r3' }).catch((e: RpcError) => e)) as RpcError;
  assert.equal((missing.data as { code?: string }).code, 'not_found');
  client.close();
});
