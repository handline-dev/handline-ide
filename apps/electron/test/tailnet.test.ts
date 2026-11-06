import { test } from 'node:test';
import assert from 'node:assert/strict';
import { addressesChosen, listenAddress, magicDnsName, onTailnet, tailscaleAddress, windowDaemon } from '#src/tailnet.ts';

const nic = (address: string) => [{ address, family: 'IPv4', internal: false }] as never;

test('only a 100.64.0.0/10 address on a utun or tailscale interface is the Tailscale address', () => {
  assert.equal(tailscaleAddress({ utun4: nic('100.101.2.3') }), '100.101.2.3');
  assert.equal(tailscaleAddress({ tailscale0: nic('100.64.0.1') }), '100.64.0.1');
  assert.equal(tailscaleAddress({ en0: nic('100.101.2.3') }), null); // the LAN may hand that range out
  assert.equal(tailscaleAddress({ utun4: nic('100.200.1.1') }), null);
  assert.equal(tailscaleAddress({ utun4: nic('10.0.0.5'), lo0: nic('127.0.0.1') }), null);
});

test('the listener is on the Tailscale address when it is up, loopback when not, and HANDLINE_LISTEN wins', () => {
  assert.equal(listenAddress(undefined, { utun4: nic('100.101.2.3') }), '100.101.2.3:7443');
  assert.equal(listenAddress(undefined, { en0: nic('192.168.0.4') }), '127.0.0.1:7443');
  assert.equal(listenAddress('127.0.0.1:17444', { utun4: nic('100.101.2.3') }), '127.0.0.1:17444');
});

test('the MagicDNS name is Self.DNSName without its trailing dot, and only a ts.net name counts', () => {
  assert.equal(magicDnsName('{"Self":{"DNSName":"mac.tail1234.ts.net."}}'), 'mac.tail1234.ts.net');
  assert.equal(magicDnsName('{"Self":{"DNSName":"mac.example.com."}}'), null);
  assert.equal(magicDnsName('{"Self":{}}'), null);
  assert.equal(magicDnsName('not json'), null);
  assert.equal(onTailnet('100.101.2.3:7443'), true);
  assert.equal(onTailnet('127.0.0.1:7443'), false);
});

test('addresses are chosen when config.yaml gives network_addresses a value', () => {
  assert.equal(addressesChosen('network_addresses: [192.168.0.4]\n'), true);
  assert.equal(addressesChosen('x: 1\nnetwork_addresses:\n  - a\n'), false); // a block list is not one the daemon reads
  assert.equal(addressesChosen('# network_addresses: [a]\nx: 1\n'), false);
  assert.equal(addressesChosen(''), false);
  assert.equal(listenAddress(undefined, { utun4: nic('100.101.2.3') }, 7443, true), '127.0.0.1:7443');
});

test('the window reaches the daemon on loopback, on the port of whatever address the listener binds', () => {
  assert.equal(windowDaemon('100.101.2.3:7443'), 'ws://127.0.0.1:7443/rpc');
  assert.equal(windowDaemon('127.0.0.1:9000'), 'ws://127.0.0.1:9000/rpc');
});
