import { test } from 'node:test';
import assert from 'node:assert/strict';
import { setup } from '#src/helpers.ts';

test("desks.list serves a project's desks; desk.pin changes them", async () => {
  const s = setup({ client: 'boss' });
  await s.client.start();
  let got = await s.client.call('desks.list', { repo: '/work/handline' });
  assert.equal(got.desks.length, 10);

  await s.client.call('desk.pin', { desk: 'desk-10', to: 'p1', request_id: 'p1' });
  got = await s.client.call('desks.list', { repo: '/work/handline' });
  assert.equal(got.desks.find((d) => d.name === 'desk-10')?.pinned_to, 'founder');
  s.client.close();
});
