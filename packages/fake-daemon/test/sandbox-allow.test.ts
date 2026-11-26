import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';

const AUD = 'handline.auditor';

test('agent.set takes sandbox_allow as one entry per line, agent.get reads it back, and a line that is no entry is bad_request', async () => {
  const { client } = setup();
  await client.start();
  assert.equal((await client.call('agent.get', { agent: AUD })).settings?.sandbox_allow, '');
  const list = '2026-11-26 host pypi.org\n2026-11-26 command pnpm install\n2026-11-26 write ~/.cargo\n2026-11-26 cert';
  await client.call('agent.set', { agent: AUD, values: { sandbox_allow: list }, request_id: 's1' });
  assert.equal((await client.call('agent.get', { agent: AUD })).settings?.sandbox_allow, list);
  for (const [n, bad] of ['weaker-network', '2026-11-26 host *', '2026-11-26 command *', 'host pypi.org'].entries())
    await assert.rejects(
      client.call('agent.set', { agent: AUD, values: { sandbox_allow: bad }, request_id: `s${n + 2}` }),
      (e: RpcError) => e.code === 'bad_request',
    );
  client.close();
});
