import { test } from 'node:test';
import assert from 'node:assert/strict';
import { FakeDaemon } from '#src/daemon.ts';
import { applySeeds } from '#src/seeds.ts';

test('second-cluster is another cluster: its own name, the shop agents only, one of them asking the founder', () => {
  const d = new FakeDaemon();
  applySeeds(d, 'second-cluster');
  assert.equal(d.admin.cluster(d.iso()).name, 'Amber Fox');
  assert.deepEqual([...d.sessions.keys()], ['shop.backend', 'shop.web']);
  assert.equal(d.tasks.find(41).to, 'shop.backend');
});
