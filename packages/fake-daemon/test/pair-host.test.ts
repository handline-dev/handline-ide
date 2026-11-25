import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';

test('device.pair names the address it is given, with that door scheme and fingerprint; one not served is refused', async () => {
  const { client } = setup();
  await client.start();
  const pick = await client.call('device.pair', { request_id: 'p1' });
  assert.match(pick.web_link, /^https:\/\/mac\.tail1\.ts\.net:7443\/#pair=/);
  const wifi = await client.call('device.pair', { host: '192.168.1.20', request_id: 'p2' });
  assert.match(wifi.web_link, new RegExp(`^https://192\\.168\\.1\\.20:7443/#pair=${wifi.code}&fp=${wifi.fingerprint}$`));
  assert.ok(wifi.fingerprint);
  await assert.rejects(client.call('device.pair', { host: 'mac.local', request_id: 'p3' }), (e: RpcError) => e.data?.gate === 'network');
});
