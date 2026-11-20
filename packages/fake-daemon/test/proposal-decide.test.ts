import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { RpcError } from '@handline/client';
import { applySeeds } from '#src/seeds.ts';
import { setup } from '#src/helpers.ts';

test('proposal.decide refuses an approve without the subject proposal.list showed, as the daemon does, and takes a reject with no note', async () => {
  const { client, daemon } = setup();
  applySeeds(daemon, 'proposals');
  await client.start();
  const { proposals } = await client.call('proposal.list', {});
  const [a, b] = proposals;
  await assert.rejects(
    client.call('proposal.decide', { proposal: a!.id, decision: 'approve' }),
    (e: RpcError) => e.code === 'bad_request' && /names the subject/.test(e.message),
  );
  await assert.rejects(
    client.call('proposal.decide', { proposal: a!.id, decision: 'approve', subject: 'stale' }),
    (e: RpcError) => e.gate === 'proposal_changed',
  );
  await client.call('proposal.decide', { proposal: a!.id, decision: 'approve', subject: a!.subject });
  await client.call('proposal.decide', { proposal: b!.id, decision: 'reject' });
  const now = (await client.call('proposal.list', {})).proposals;
  assert.equal(now.find((p) => p.id === a!.id)?.state, 'approved');
  assert.equal(now.find((p) => p.id === b!.id)?.state, 'rejected');
  client.close();
});
