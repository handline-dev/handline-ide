import { after, test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { joinLink, Link, offerFrom, SettingsFile } from '#src/mode.ts';

const dirs: string[] = [];
const tmp = () => path.join(dirs[dirs.push(fs.mkdtempSync(path.join(os.tmpdir(), 'mode-'))) - 1]!, 'settings.json');
after(() => dirs.forEach((d) => fs.rmSync(d, { recursive: true, force: true })));
const master = { host: 'mac.tail1.ts.net:7443', fingerprint: null, token: 't0k', device: 'd1' };
const FP = 'ab'.repeat(32);

test('the defaults: open at login on; login and quiet quit stay', () => {
  const f = new SettingsFile(tmp());
  assert.deepEqual(f.read(), { openAtLogin: true });
  f.write({ openAtLogin: false, quietQuit: true });
  assert.deepEqual(f.read(), { openAtLogin: false, quietQuit: true });
});

test('a pairing with another device kept by an older app is not used', () => {
  const file = tmp();
  fs.writeFileSync(file, JSON.stringify({ mode: 'connect', remote: master }));
  assert.deepEqual(new SettingsFile(file).read(), { openAtLogin: true });
});

test('the file holds the doors, so only this user reads it', () => {
  const file = tmp();
  new SettingsFile(file).write({ openAtLogin: true });
  assert.equal(fs.statSync(file).mode & 0o777, 0o600);
});

test('a broken settings file gives the defaults', () => {
  const file = tmp();
  fs.writeFileSync(file, '{nope');
  assert.deepEqual(new SettingsFile(file).read(), { openAtLogin: true });
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

test('hl device join gets a link: the one pasted, or one made from the address and the code', () => {
  assert.equal(
    joinLink({ link: ` handline://pair?host=192.168.1.5:7443&code=a%2Fb&fp=${FP.toUpperCase()} ` }),
    `handline://pair?host=192.168.1.5%3A7443&code=a%2Fb&fp=${FP}`,
  );
  assert.equal(joinLink({ address: 'mac.tail1.ts.net', code: 'abc' }), 'handline://pair?host=mac.tail1.ts.net%3A7443&code=abc');
  // Connect a device's Copy link: the page's invite, with the certificate after the code.
  assert.equal(
    joinLink({ link: `https://mac.local:7443/#pair=a%2Fb&fp=${FP}` }),
    `handline://pair?host=mac.local%3A7443&code=a%2Fb&fp=${FP}`,
  );
  assert.throws(() => joinLink({ link: 'https://192.168.1.5:7443/#pair=abc' }), /Tailscale/);
  // This machine's loopback is private as it is, as a plain door there is.
  assert.equal(joinLink({ address: '127.0.0.1:7461', code: 'abc' }), 'handline://pair?host=127.0.0.1%3A7461&code=abc');
  assert.throws(() => joinLink({ link: 'https://example.com' }));
});

test('the link remembers the version as the short sha', () => {
  const l = new Link();
  l.setVersion('0123456789abcdef');
  assert.equal(l.version, '0123456');
});
