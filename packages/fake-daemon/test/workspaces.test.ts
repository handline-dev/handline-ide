import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type Event, type RpcError } from '@handline/client';
import { setup, until } from '#src/helpers.ts';

const refusedWith = (code: string, gate?: string) => (e: RpcError) => e.code === code && (gate === undefined || e.gate === gate);
type Client = ReturnType<typeof setup>['client'];
const names = async (client: Client) => (await client.call('workspaces.list', {})).workspaces.map((w) => w.name);
const named = async (client: Client, name: string) => {
  const ws = (await client.call('workspaces.list', {})).workspaces.find((w) => w.name === name);
  assert.ok(ws, `no workspace ${name}`);
  return ws;
};

test('workspace.create starts its lead as agent.create does, and the workspace is listed at its folder', async () => {
  const { client, daemon } = setup();
  await client.start();
  const seen: Event[] = [];
  client.onEvent((e) => e.topic === 'operations' && seen.push(e));
  await client.subscribe(['operations']);
  const r = await client.call('workspace.create', { name: 'garden', folder: '~/code/garden/src', request_id: 'wc-1' });
  await until(() => seen.some((e) => (e.data as { state: string }).state === 'done'), 'done');
  const op = await client.call('operation.get', { operation: r.operation });
  assert.deepEqual([op.kind, op.agent, op.state], ['workspace.create', 'garden', 'done']);
  assert.equal(daemon.sessions.get('garden')?.state, 'idle');
  const ws = await named(client, 'garden');
  assert.deepEqual([ws.folder, ws.lead], ['/Users/me/code/garden', 'garden']);
  client.close();
});

test('workspace.create refuses a bad name, a taken one, a folder in no checkout and a missing one, before any operation', async () => {
  const { client } = setup();
  await client.start();
  const make = (name: string, folder = '/work/handline') => client.call('workspace.create', { name, folder, request_id: `wc-${name}` });
  await assert.rejects(make('a.b'), (e: RpcError) => e.code === 'bad_request' && e.data?.field === 'name');
  await assert.rejects(make('handline'), refusedWith('refused', 'state'));
  await assert.rejects(make('recipes', '/Users/me/Documents/recipe-site'), refusedWith('refused', 'not_git'));
  await assert.rejects(make('nothing', '/nowhere'), refusedWith('not_found'));
  client.close();
});

test('a folder another workspace has fails the operation, and the name stays free', async () => {
  const { client, daemon } = setup();
  await client.start();
  const seen: Event[] = [];
  client.onEvent((e) => e.topic === 'operations' && seen.push(e));
  await client.subscribe(['operations']);
  await client.call('workspace.create', { name: 'garden', folder: '/Users/me/code/garden', request_id: 'wc-1' });
  await until(() => seen.some((e) => (e.data as { state: string }).state === 'done'), 'first');
  const r = await client.call('workspace.create', { name: 'garden-two', folder: '/Users/me/code/garden', request_id: 'wc-2' });
  await until(() => seen.some((e) => (e.data as { state: string }).state === 'failed'), 'failed');
  const op = await client.call('operation.get', { operation: r.operation });
  assert.deepEqual(
    [op.message, op.result],
    ['/Users/me/code/garden is the folder of workspace garden', { agent: 'garden-two', registered: false }],
  );
  assert.equal(daemon.sessions.has('garden-two'), false);
  const seeded = await client.call('workspace.create', { name: 'again', folder: '/work/handline', request_id: 'wc-3' });
  await until(() => seen.filter((e) => (e.data as { state: string }).state === 'failed').length === 2, 'seeded folder');
  assert.equal(
    (await client.call('operation.get', { operation: seeded.operation })).message,
    '/work/handline is the folder of workspace handline',
  );
  client.close();
});

test('workspace.set renames by key or name, keeps the keys, and is refused while an agent runs', async () => {
  const { client, daemon } = setup();
  await client.start();
  const events: Event[] = [];
  client.onEvent((e) => e.topic === 'workspaces' && events.push(e));
  await client.subscribe(['workspaces']);
  const ws = await named(client, 'handline');
  for (const s of daemon.sessions.values()) s.open = false;
  daemon.sessions.get('handline.auditor')!.open = true;
  await assert.rejects(
    client.call('workspace.set', { workspace: 'handline', name: 'hl', request_id: 'ws-1' }),
    refusedWith('refused', 'state'),
  );
  daemon.sessions.get('handline.auditor')!.open = false;
  await client.call('workspace.set', { workspace: ws.key, name: 'hl', request_id: 'ws-2' });
  assert.ok((await names(client)).includes('hl'));
  assert.equal((await named(client, 'hl')).key, ws.key);
  assert.ok(daemon.sessions.has('handline.auditor'), 'keys do not change');
  await until(() => events.some((e) => e.type === 'workspaces.changed'), 'workspaces.changed');
  await assert.rejects(client.call('workspace.set', { workspace: 'handline', name: 'x', request_id: 'ws-3' }), refusedWith('not_found'));
  await assert.rejects(
    client.call('workspace.create', { name: 'handline', folder: '/work/docs-site', request_id: 'wc-old' }),
    refusedWith('refused', 'state'),
  );
  const same = await client.call('workspace.set', { workspace: 'hl', name: 'hl', request_id: 'ws-4' });
  assert.equal(same.said, 'hl is its name already.');
  client.close();
});

test('workspace.delete refuses a running agent and held work, then retires every agent and the workspace goes', async () => {
  const { client, daemon } = setup();
  await client.start();
  await client.call('workspace.create', { name: 'garden', folder: '/Users/me/code/garden', request_id: 'wc-1' });
  await until(() => daemon.sessions.get('garden')?.state === 'idle', 'lead up');
  const lead = daemon.sessions.get('garden')!;
  lead.open = true;
  await assert.rejects(client.call('workspace.delete', { workspace: 'garden', request_id: 'wd-1' }), refusedWith('refused', 'state'));
  lead.open = false;
  daemon.held.set('garden', [{ path: '/Users/me/code/garden.garden', files_changed: 2, commits_ahead: 1 }]);
  await assert.rejects(client.call('workspace.delete', { workspace: 'garden', request_id: 'wd-2' }), refusedWith('refused', 'dirty'));
  await assert.rejects(
    client.call('workspace.delete', { workspace: 'garden', worktree: 'keep' as 'save', request_id: 'wd-3' }),
    (e: RpcError) => e.code === 'bad_request' && e.data?.field === 'worktree',
  );
  const r = await client.call('workspace.delete', { workspace: 'garden', worktree: 'save', request_id: 'wd-4' });
  assert.equal(r.said, 'garden is deleted.');
  assert.equal(daemon.sessions.has('garden'), false);
  assert.ok(!(await names(client)).includes('garden'));
  client.close();
});
