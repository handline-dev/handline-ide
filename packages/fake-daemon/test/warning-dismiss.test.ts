import { test } from 'node:test';
import assert from 'node:assert/strict';
import { setup } from '#src/helpers.ts';

test('warning.dismiss takes a warning out of requests.pending, a second time too, and one it does not have is not_found', async () => {
  const s = setup({ client: 'boss' });
  await s.client.start();
  s.daemon.tasks.warnings.push({
    task: null,
    title: 'Account gamma needs a login.',
    kind: 'warning',
    source: 'machine',
    id: 'login:gamma',
    agent: null,
    ask: null,
    warning: { kind: 'login', account: 'gamma' },
    items: [],
    unread: 0,
    first_unread: null,
    last_at: new Date().toISOString(),
  });
  const ids = async () => (await s.client.call('requests.pending', {})).answer.map((r) => r.id).filter((id) => id === 'login:gamma');
  assert.deepEqual(await ids(), ['login:gamma']);
  await s.client.call('warning.dismiss', { id: 'login:gamma', request_id: 'd1' });
  assert.deepEqual(await ids(), []);
  await s.client.call('warning.dismiss', { id: 'login:gamma', request_id: 'd2' });
  assert.deepEqual(await ids(), []);
  await assert.rejects(s.client.call('warning.dismiss', { id: 'login:omega', request_id: 'd3' }), /no warning/);
});
