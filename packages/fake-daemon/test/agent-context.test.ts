import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { second, setup } from '#src/helpers.ts';

const at = new Date('2026-11-25T10:00:00Z');

test('agent.context.set replaces a part as the founder, for the next fresh start; an agent is refused', async () => {
  const s = setup({ daemon: { now: () => at } });
  await s.client.start();
  assert.deepEqual(await s.client.call('agent.context', { agent: 'a3' }), { agent: 'a3', role: null, base: null });
  const r = await s.client.call('agent.context.set', { agent: 'a3', role: ' Builds the daemon.\n', request_id: 'r1' });
  assert.match(r.said, /role replaced; applies when it next starts fresh/);
  const got = await s.client.call('agent.context', { agent: 'a3' });
  assert.equal(got.role?.text, 'Builds the daemon.');
  assert.equal(got.role?.at, '2026-11-25T10:00:00Z');
  assert.equal(got.base, null);
  assert.match((await s.client.call('agent.context.set', { agent: 'a3', role: 'Builds the daemon.', request_id: 'r2' })).said, /already/);
  await assert.rejects(s.client.call('agent.context.set', { agent: 'a3', request_id: 'r3' }), (e: RpcError) => e.data?.field === 'role');
  await assert.rejects(
    s.client.call('agent.context.set', { agent: 'a3', base: 'a\u0007b', request_id: 'r4' }),
    (e: RpcError) => e.data?.field === 'base',
  );
  const agent = second(s.daemon, s.net, { role: 'agent', client: 'handline.architect' });
  await agent.start();
  await assert.rejects(
    agent.call('agent.context.set', { agent: 'a3', role: 'anything', request_id: 'r5' }),
    (e: RpcError) => e.data?.gate === 'role',
  );
});

test('agent.memory.remove takes one note and its MEMORY.md line, leaving the other lines as they were', async () => {
  const s = setup();
  await s.client.start();
  const note = (name: string, file: string) => ({ name, file, description: '', type: 'project', body: 'text\n' });
  s.daemon.context.memory.set('a3', {
    index: '# Memory\n- [One](./one.md) — first\n- [Two](two.md) — second\n- [Old](archive/one.md) — elsewhere\nKept by hand.\n',
    notes: [note('one', 'one.md'), note('two', 'two.md'), note('two', 'twin.md')],
  });
  assert.deepEqual(
    (await s.client.call('agent.memory', { agent: 'a3' })).notes.map((n) => n.name),
    ['one', 'two', 'two'],
  );
  await s.client.call('agent.memory.remove', { agent: 'a3', name: 'one', request_id: 'r1' });
  await assert.rejects(
    s.client.call('agent.memory.remove', { agent: 'a3', name: 'two', request_id: 'r1b' }),
    (e: RpcError) => e.data?.field === 'name',
  );
  await s.client.call('agent.memory.remove', { agent: 'a3', name: 'twin.md', request_id: 'r1c' });
  assert.deepEqual(
    (await s.client.call('agent.memory', { agent: 'a3' })).notes.map((n) => n.file),
    ['two.md'],
  );
  assert.equal(
    s.daemon.context.memory.get('a3')?.index,
    '# Memory\n- [Two](two.md) — second\n- [Old](archive/one.md) — elsewhere\nKept by hand.\n',
  );
  await assert.rejects(
    s.client.call('agent.memory.remove', { agent: 'a3', name: 'one', request_id: 'r2' }),
    (e: RpcError) => e.data?.what === 'memory',
  );
  const agent = second(s.daemon, s.net, { role: 'agent', client: 'handline.architect' });
  await agent.start();
  await assert.rejects(agent.call('agent.memory', { agent: 'a3' }), (e: RpcError) => e.data?.gate === 'role');
});
