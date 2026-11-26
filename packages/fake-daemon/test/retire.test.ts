import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';
import { newSession } from '#src/session.ts';

test('agent.retire refuses an open session, then retires the agent and it is gone', async () => {
  const { client, daemon } = setup();
  await client.start();
  const name = 'handline.auditor';
  const s = daemon.sessions.get(name)!;
  s.open = true;
  await assert.rejects(
    client.call('agent.retire', { agent: name, request_id: 'ret-1' }),
    (e: RpcError) => e.code === 'refused' && e.gate === 'state',
  );
  s.open = false;
  const r = await client.call('agent.retire', { agent: name, request_id: 'ret-2' });
  assert.equal(r.said, `${name} is retired.`);
  const names = (await client.call('agents.list', {})).agents.map((a: { key: string }) => a.key);
  assert.ok(!names.includes(name), names.join(', '));
  client.close();
});

test('agent.retire of a lead hands its workspace to next_lead, and its seats go with it', async () => {
  const { client, daemon } = setup();
  await client.start();
  const fx = daemon.sessions.get('handline.auditor')!.fx;
  const add = (name: string, view = {}) =>
    daemon.sessions.set(name, newSession({ ...fx, name, view, session: null }, '2026-11-26T00:00:00Z'));
  add('farm');
  add('farm.seat', { hired_from: 'farm' });
  add('farm.b');
  add('farm.c');
  const farm = async () => (await client.call('workspaces.list', {})).workspaces.find((w) => w.name === 'farm');
  assert.deepEqual([(await farm())?.lead, (await farm())?.next_lead], ['farm', 'farm.b']);
  await client.call('agent.retire', { agent: 'farm', request_id: 'ret-lead' });
  assert.deepEqual([(await farm())?.lead, (await farm())?.next_lead], ['farm.b', 'farm.c']);
  assert.ok(!daemon.sessions.has('farm.seat'));
  await client.call('agent.retire', { agent: 'farm.b', request_id: 'ret-next' });
  assert.deepEqual([(await farm())?.lead, (await farm())?.next_lead], ['farm.c', null]);
  client.close();
});
