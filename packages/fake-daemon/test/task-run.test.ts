import { test } from 'node:test';
import assert from 'node:assert/strict';
import { applySeeds } from '#src/seeds.ts';
import { setup } from '#src/helpers.ts';

test("a task's timeline carries the command its agent asked while doing it, and how it ended", async () => {
  const { client, daemon } = setup();
  applySeeds(daemon, 'proposals');
  await client.start();
  const runRow = async () =>
    (await client.call('task.get', { task: 1684 })).timeline.find((r) => r.kind === 'run' && r.proposal?.id === 910);
  const asked = await runRow();
  assert.equal(asked?.text, 'gh auth refresh -s workflow && git push origin HEAD:main');
  assert.equal(asked?.proposal?.state, 'open');
  const { proposals } = await client.call('proposal.list', {});
  const subject = proposals.find((p) => p.id === 910)!.subject;
  await client.call('proposal.decide', { proposal: 910, decision: 'reject', subject, request_id: 'r1' });
  assert.equal((await runRow())?.proposal?.state, 'rejected');
  assert.equal(
    (await client.call('task.get', { task: 1688 })).timeline.some((r) => r.kind === 'run'),
    false,
    "another task's",
  );
});
