import { after, test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import type { Transport } from '@handline/client';
import { findMaster, joinedOf, ownConnection } from '#src/joined.ts';
import { protectedDir } from '#src/pair.ts';

const FP = 'ab'.repeat(32);

/** A runner's home and its stand-in user home: the credential it joined with, its node id and the doors its heartbeat listed. */
function runnerHome(credential: object | null, doors: object[] = []) {
  const user = fs.mkdtempSync(path.join(os.tmpdir(), 'runner-user-'));
  const home = fs.mkdtempSync(path.join(os.tmpdir(), 'runner-home-'));
  after(() => [user, home].forEach((d) => fs.rmSync(d, { recursive: true, force: true })));
  fs.writeFileSync(path.join(home, 'node_id'), 'self\n');
  fs.writeFileSync(path.join(home, 'master-doors.json'), JSON.stringify(doors));
  if (credential) {
    const dir = path.join(protectedDir(user, home), 'master');
    fs.mkdirSync(dir, { recursive: true });
    fs.writeFileSync(path.join(dir, 'credential'), JSON.stringify(credential));
  }
  return { user, home };
}

test('a home that joined no master is not a runner Mac', () => {
  const { user, home } = runnerHome(null);
  assert.equal(joinedOf(user, home), null);
});

test('a runner Mac knows the master it obeys, its number, and the owner’s other machines, less itself and doors it may not trust', () => {
  const { user, home } = runnerHome(
    { master: 'studio.tail1.ts.net:7443', fp: '', token: 'hld_x', device: 'dev_1', node: 'm1', number: 2 },
    [
      { node: 'm1', addr: 'studio.tail1.ts.net:7443', fp: '' },
      { node: 'self', addr: '127.0.0.1:7443', fp: '' },
      { node: 'm2', addr: '203.0.113.5:7443', fp: FP },
      { node: 'm3', addr: '203.0.113.6:7443', fp: '' }, // plain on the open network: not trusted
    ],
  );
  const j = joinedOf(user, home);
  assert.deepEqual(j, {
    master: { host: 'studio.tail1.ts.net:7443', fingerprint: null, token: 'hld_x', device: 'dev_1' },
    floor: { node: 'm1', number: 2 },
    doors: [
      { addr: 'studio.tail1.ts.net:7443', fp: '' },
      { addr: '203.0.113.5:7443', fp: FP },
    ],
  });
});

test('the master found is the highest that answers, asked with the device token and not below the number obeyed', async () => {
  const { user, home } = runnerHome(
    { master: 'studio.tail1.ts.net:7443', fp: '', token: 'hld_x', device: 'dev_1', node: 'm1', number: 2 },
    [{ node: 'm2', addr: '203.0.113.5:7443', fp: FP }],
  );
  const j = joinedOf(user, home)!;
  let asked: unknown[] = [];
  const m = await findMaster(j, async (urls, _open, token, client, floor) => {
    asked = [urls, token, client, floor];
    return { url: 'wss://203.0.113.5:7443/rpc', master: { node: 'm2', number: 3 } };
  });
  assert.deepEqual(asked, [
    ['ws://studio.tail1.ts.net:7443/rpc', 'wss://203.0.113.5:7443/rpc'],
    'hld_x',
    'electron',
    { node: 'm1', number: 2 },
  ]);
  assert.deepEqual(m, { host: '203.0.113.5:7443', fingerprint: FP, token: 'hld_x', device: 'dev_1' });
  assert.equal(await findMaster(j, async () => null), null);
});

test("the shell's own connection is this Mac's daemon with the founder's key, and the master it obeys once it is a runner", async () => {
  const local = { kind: 'local' } as unknown as Transport;
  const remote = { kind: 'remote' } as unknown as Transport;
  const asked: unknown[] = [];
  const own = (h: { user: string; home: string }) =>
    ownConnection({
      userHome: h.user,
      home: h.home,
      local: async () => local,
      key: () => 'founder-key',
      founder: true,
      remote: (j) => async () => (asked.push(j.master.host), remote),
    });
  const master = runnerHome(null);
  const c = own(master);
  assert.equal(await c.connect(), local);
  assert.equal(c.key(), 'founder-key');
  assert.equal(c.token(), undefined);
  assert.equal(c.founder(), true);
  const runner = runnerHome({ master: 'studio.tail1.ts.net:7443', fp: '', token: 'hld_x', device: 'dev_1', node: 'm1', number: 2 });
  const r = own(runner);
  assert.equal(await r.connect(), remote);
  assert.deepEqual(asked, ['studio.tail1.ts.net:7443']);
  assert.equal(r.key(), null);
  assert.equal(r.token(), 'hld_x');
  assert.equal(r.founder(), false, 'a remote master takes this Mac as its device person, who may be no owner');
});
