import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type Event, type RpcError } from '@handline/client';
import { applySeeds } from '#src/seeds.ts';
import { setup, until } from '#src/helpers.ts';

test('a devices event reaches a client that follows devices with other topics, after logged events moved its cursor', async () => {
  const { client } = setup();
  await client.start();
  const seen: Event[] = [];
  client.onEvent((e) => seen.push(e));
  await client.subscribe(['operations']);
  await client.subscribe(['devices']);
  await client.call('workspace.create', { name: 'garden', folder: '~/code/garden/src', request_id: 'dt-1' });
  await until(() => seen.some((e) => e.topic === 'operations' && e.cursor > 0), 'operations');
  await client.call('device.set', { device: 'd1', lending: 'pause', request_id: 'dt-2' });
  await until(() => seen.some((e) => e.topic === 'devices' && e.type === 'device.changed'), 'device.changed');
  client.close();
});

test("device.update has a Mac on another build take the master's: its lack goes, nodes changes, and a second Update is refused", async () => {
  const { daemon, client } = setup();
  applySeeds(daemon, 'devices');
  await client.start();
  const seen: Event[] = [];
  client.onEvent((e) => seen.push(e));
  await client.subscribe(['nodes']);
  const laptop = async () => {
    const r = (await client.call('node.status', {})) as { nodes: { device?: string | null; version?: string; lacks?: { why: string }[] }[] };
    return r.nodes.find((n) => n.device === 'd-laptop')!;
  };
  assert.match((await laptop()).lacks![0]!.why, /starting again on the master's build/);
  await client.call('settings.set', { changes: { node_build_auto: false }, request_id: 'du-1' });
  assert.match((await laptop()).lacks![0]!.why, /update to take work/);
  await until(() => seen.some((e) => e.type === 'nodes.changed'), 'nodes.changed');
  await client.call('device.update', { device: 'd-laptop', request_id: 'du-2' });
  assert.deepEqual((await laptop()).lacks, []);
  await assert.rejects(client.call('device.update', { device: 'd-laptop', request_id: 'du-3' }), (e: RpcError) => e.data?.gate === 'build');
  await assert.rejects(client.call('device.update', { device: 'd-mini', request_id: 'du-4' }), (e: RpcError) => e.data?.gate === 'build');
  client.close();
});
