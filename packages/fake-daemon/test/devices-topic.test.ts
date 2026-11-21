import { test } from 'node:test';
import { type Event } from '@handline/client';
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
