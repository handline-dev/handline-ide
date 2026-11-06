import { after, test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { helloOf, Link, offerFrom, pairRemote, plainRemoteHost, SettingsFile, type Open } from '#src/mode.ts';

const dirs: string[] = [];
const tmp = () => path.join(dirs[dirs.push(fs.mkdtempSync(path.join(os.tmpdir(), 'mode-'))) - 1]!, 'settings.json');
after(() => dirs.forEach((d) => fs.rmSync(d, { recursive: true, force: true })));
const remote = { host: 'mac.tail1.ts.net:7443', fingerprint: null, token: 't0k', device: 'd1' };

test('the defaults: this Mac’s daemon, open at login on; a kept remote, login and quiet quit stay', () => {
  const f = new SettingsFile(tmp());
  assert.deepEqual(f.read(), { remote: null, openAtLogin: true });
  f.write({ remote, openAtLogin: false, quietQuit: true });
  assert.deepEqual(f.read(), { remote, openAtLogin: false, quietQuit: true });
});

test('a file from before the mode went: own mode stays on this Mac’s daemon, a connect mode pairing is still used', () => {
  const file = tmp();
  fs.writeFileSync(file, JSON.stringify({ mode: 'own', remote }));
  assert.equal(new SettingsFile(file).read().remote, null);
  fs.writeFileSync(file, JSON.stringify({ mode: 'connect', remote }));
  assert.deepEqual(new SettingsFile(file).read().remote, remote);
});

test('the file holds a token, so only this user reads it', () => {
  const file = tmp();
  new SettingsFile(file).write({ remote, openAtLogin: true });
  assert.equal(fs.statSync(file).mode & 0o777, 0o600);
});

test('a broken settings file gives the defaults', () => {
  const file = tmp();
  fs.writeFileSync(file, '{nope');
  assert.deepEqual(new SettingsFile(file).read(), { remote: null, openAtLogin: true });
});

test('an address and a code make an offer; a plain address that is not Tailscale is refused', () => {
  assert.deepEqual(offerFrom({ address: 'mac.tail1.ts.net', code: 'abc' }), {
    host: 'mac.tail1.ts.net:7443',
    fingerprint: null,
    code: 'abc',
  });
  assert.throws(() => offerFrom({ address: '192.168.1.5:7443', code: 'abc' }), /Tailscale/);
  assert.throws(() => offerFrom({}), /pairing link/);
});

test('a plain Tailscale remote is the one host the page may reach over ws', () => {
  assert.equal(plainRemoteHost(remote), 'mac.tail1.ts.net');
  assert.equal(plainRemoteHost({ ...remote, fingerprint: 'ab' }), null);
});

const never: Open = () => new Promise(() => undefined);

test('a daemon that never answers fails with a sentence instead of hanging', async () => {
  await assert.rejects(
    pairRemote({ address: 'mac.tail1.ts.net', code: 'abc' }, { open: never, name: 'x', timeoutMs: 20 }),
    /mac\.tail1\.ts\.net:7443 cannot be paired with: .*nothing answered/,
  );
});

test('a refused connection says why', async () => {
  const open: Open = () => Promise.reject(new Error('cannot connect: ECONNREFUSED'));
  await assert.rejects(pairRemote({ address: 'mac.tail1.ts.net', code: 'abc' }, { open, name: 'x' }), /ECONNREFUSED/);
});

test('the link remembers the version as the short sha', () => {
  const l = new Link();
  l.setVersion('0123456789abcdef');
  assert.equal(l.version, '0123456');
});

const answering =
  (reply: unknown): Open =>
  async () => {
    const t = {
      onmessage: null as ((m: string) => void) | null,
      onclose: null as (() => void) | null,
      send() {
        setTimeout(() => t.onmessage?.(JSON.stringify(reply)), 0);
      },
      close() {},
    };
    return t as never;
  };

test('connected means a hello with our token was accepted, and the version comes from it', async () => {
  assert.equal(await helloOf(remote, answering({ jsonrpc: '2.0', id: 1, result: { daemon: 'abc1234def' } })), 'abc1234def');
  await assert.rejects(
    helloOf(remote, answering({ jsonrpc: '2.0', id: 1, error: { message: 'refused', data: { reason: 'this device is not paired' } } })),
    /this device is not paired/,
  );
});
