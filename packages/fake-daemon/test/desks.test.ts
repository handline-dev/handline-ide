import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';

test("desks.list serves a project's desks with how many it keeps; desks.keep and desk.pin change them", async () => {
  const s = setup({ client: 'boss' });
  await s.client.start();
  let got = await s.client.call('desks.list', { repo: '/work/handline' });
  assert.equal(got.desks.length, 10);
  assert.deepEqual(got.kept, { mode: 'auto', value: 9, auto_value: 9 });
  assert.deepEqual(
    got.desks.filter((d) => d.removed_tonight).map((d) => d.name),
    ['desk-10'],
  );
  assert.equal((await s.client.call('desks.list', {})).kept, null);

  await s.client.call('desks.keep', { repo: '/work/handline', mode: 'manual', value: 8, request_id: 'k1' });
  got = await s.client.call('desks.list', { repo: '/work/handline' });
  assert.deepEqual(got.kept, { mode: 'manual', value: 8, auto_value: 9 });
  assert.deepEqual(
    got.desks.filter((d) => d.removed_tonight).map((d) => d.name),
    ['desk-10', 'desk-9'], // eight kept: the two oldest free desks unused for three days by tonight
  );
  const bad = (await s.client
    .call('desks.keep', { repo: '/work/handline', mode: 'manual', value: 0, request_id: 'k2' })
    .catch((e) => e)) as RpcError;
  assert.deepEqual([bad.data?.code, bad.data?.field], ['bad_request', 'value']);
  const none = (await s.client.call('desks.keep', { repo: '/nope', mode: 'auto', request_id: 'k3' }).catch((e) => e)) as RpcError;
  assert.equal(none.data?.code, 'not_found');

  await s.client.call('desk.pin', { desk: 'desk-10', to: 'p1', request_id: 'p1' });
  got = await s.client.call('desks.list', { repo: '/work/handline' });
  const ten = got.desks.find((d) => d.name === 'desk-10');
  assert.deepEqual([ten?.pinned_to, ten?.removed_tonight], ['founder', false]);
  s.client.close();
});
