import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';

const at = new Date('2026-11-12T10:00:00Z');

test('workspace.rules: a workspace value beats the machine default, a key set nowhere is not listed, and null clears', async () => {
  const { client } = setup({ daemon: { now: () => at } });
  await client.start();
  const [a] = (await client.call('workspaces.list', {})).workspaces;
  assert.ok(a, 'a seeded workspace');
  const set = (key: string, value: unknown, workspace?: string) =>
    client.call('workspace.rule.set', {
      ...(workspace ? { workspace } : {}),
      key,
      value,
      request_id: `${workspace}-${key}-${String(value)}`,
    } as never);
  const rules = async (workspace?: string) =>
    (await client.call('workspace.rules', workspace ? { workspace } : {})).rules.map((r) => [r.key, r.value, r.on, r.source]);

  assert.deepEqual(await rules(a.key), []);
  const { text_bytes, limit_bytes } = await client.call('workspace.rules', { workspace: a.key });
  assert.deepEqual([text_bytes, limit_bytes], [0, 2048]);
  assert.equal('text_bytes' in (await client.call('workspace.rules', {})), false);
  await set('agent_cap', 3);
  await set('agent_cap', 1, a.key);
  await set('review_required', false, a.name);
  assert.deepEqual(await rules(a.key), [
    ['agent_cap', 1, true, 'workspace'],
    ['review_required', false, false, 'workspace'],
  ]);
  assert.deepEqual(await rules(), [['agent_cap', 3, true, 'machine']]);
  const [row] = (await client.call('workspace.rules', {})).rules;
  assert.equal(row?.set_at, at.getTime() / 1000);

  await set('agent_cap', null, a.key);
  assert.deepEqual((await rules(a.key))[0], ['agent_cap', 3, true, 'machine']);
  client.close();
});

test('workspace.rule.set refuses an unknown key, a value of the wrong kind, an account no login has and an unknown workspace', async () => {
  const { client } = setup();
  await client.start();
  const bad = (p: Record<string, unknown>, code: string, field?: string) =>
    assert.rejects(client.call('workspace.rule.set', { request_id: JSON.stringify(p), ...p } as never), (e: RpcError) => {
      assert.equal(e.code, code);
      if (field) assert.equal((e.data as { field?: string }).field, field);
      return true;
    });
  await bad({ key: 'nope', value: 1 }, 'bad_request', 'key');
  await bad({ key: 'agent_cap', value: -1 }, 'bad_request', 'value');
  await bad({ key: 'mode_max', value: 'yolo' }, 'bad_request', 'value');
  await bad({ key: 'accounts', value: ['no-such-account'] }, 'bad_request', 'value');
  await bad({ workspace: 'w999', key: 'agent_cap', value: 1 }, 'not_found');
  client.close();
});
