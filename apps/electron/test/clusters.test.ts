import { after, test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import type { ConnectionState } from '@handline/client';
import { Clusters, engineCmd, engineHome, type ClusterDeps, type Engine, type MasterLink } from '#src/clusters.ts';
import type { Settings } from '#src/mode.ts';
import type { Joined } from '#src/joined.ts';

const roots: string[] = [];
after(() => roots.forEach((d) => fs.rmSync(d, { recursive: true, force: true })));
const FP = 'ab'.repeat(32);
const LINK = `handline://pair?host=mac.local%3A7443&code=c0de&fp=${FP}`;

/** A Mac with its own home, the settings file, and fakes for hl, the engines and the masters. */
function world() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'clusters-'));
  roots.push(root);
  let kept: Settings = { openAtLogin: true };
  const joinedHomes = new Map<string, Joined>();
  const engines: (Engine & { home: string; port: number | undefined; closed: boolean; finishStart: () => void })[] = [];
  const promoted: string[] = [];
  let ports = 50100;
  let handover: Promise<void> = Promise.resolve();
  let slow = false;
  let down = false;
  const links: (MasterLink & { calls: unknown[]; set: (s: ConnectionState) => void; current: () => Joined })[] = [];
  let refuse = '';
  const d: ClusterDeps = {
    home: path.join(root, 'home'),
    file: { read: () => structuredClone(kept), write: (s) => void (kept = structuredClone(s)) },
    join: async (link, home) => {
      if (refuse) throw new Error(refuse);
      assert.equal(link, LINK);
      joinedHomes.set(home, {
        master: { host: 'mac.local:7443', fingerprint: FP, token: 'tok', device: 'dev1' },
        floor: null,
        doors: [{ addr: '10.0.0.2:7443', fp: FP }],
      });
      fs.mkdirSync(path.join(root, 'store', path.basename(home)), { recursive: true });
    },
    joined: (home, port) => {
      const j = joinedHomes.get(home) ?? null;
      // Made the master: reached at its own door, with the token it joined with (ledOf).
      return j && promoted.includes(home) ? { ...j, master: { ...j.master, host: `127.0.0.1:${port}`, fingerprint: null }, led: true } : j;
    },
    port: async () => ports++,
    promote: async (home) => {
      if (!joinedHomes.has(home)) throw new Error('this machine keeps no copy of the board');
      await handover;
      promoted.push(home);
      return { number: 2, handed_over: false };
    },
    store: (home) => path.join(root, 'store', path.basename(home)),
    engine: (home, port) => {
      let begun: () => void = () => undefined;
      const e = {
        home,
        port,
        state: 'stopped',
        reason: '',
        closed: false,
        // A start ends when the test lets it (slowStart), as a daemon that takes its time to answer.
        start: () => (slow ? new Promise<void>((r) => (begun = r)) : Promise.resolve()).then(() => void (e.state = 'running')),
        stop: async () => void (e.state = 'stopped'),
        close: () => void (e.closed = true),
        finishStart: () => begun(),
      };
      engines.push(e);
      return e;
    },
    link: (current) => {
      const fns: ((s: ConnectionState) => void)[] = [];
      current(); // as the real one reads the token when it is made
      const l = {
        current,
        state: 'connecting' as ConnectionState,
        hello: null as MasterLink['hello'],
        calls: [] as unknown[],
        start: async () => {
          if (down) throw new Error('connect ECONNREFUSED');
          l.set('open');
          return l.hello!;
        },
        close: () => void (l.state = 'closed'),
        call: (async (method: string, params: unknown) => void l.calls.push([method, params])) as MasterLink['call'],
        onState: (f: (s: ConnectionState) => void) => (fns.push(f), () => undefined),
        set: (s: ConnectionState) => {
          l.state = s;
          if (s === 'open') l.hello = { capabilities: { cluster_name: 'Blue Harbor' } } as unknown as MasterLink['hello'];
          fns.forEach((f) => f(s));
        },
      };
      links.push(l);
      return l;
    },
    changed: () => undefined,
  };
  return {
    d,
    kept: () => kept,
    engines,
    links,
    refuse: (why: string) => (refuse = why),
    slowStart: () => (slow = true),
    goDown: () => (down = true),
    slowHandover: () => {
      let done: () => void = () => undefined;
      handover = new Promise<void>((ok) => (done = ok));
      return () => done();
    },
  };
}

const settle = () => new Promise((r) => setImmediate(r));

test('a join keeps the cluster, starts its engine on a home of its own and names it as its master does', async () => {
  const w = world();
  const c = new Clusters(w.d);
  const got = await c.join({ link: LINK });
  await settle();
  const home = engineHome(w.d.home, got.id);
  assert.equal(path.dirname(home), `${w.d.home}-clusters`);
  assert.deepEqual(
    w.engines.map((e) => [e.home, e.state]),
    [[home, 'running']],
  );
  assert.deepEqual(c.list(), [
    { id: got.id, name: 'Blue Harbor', daemon: `wss://mac.local:7443/rpc`, token: 'tok', reachable: true, master: false },
  ]);
  assert.deepEqual(w.kept().clusters, [{ id: got.id, name: 'Blue Harbor', port: 50100 }]);
  assert.equal(w.engines[0]!.port, 50100);
  assert.deepEqual(c.doors(), [
    { addr: 'mac.local:7443', fp: FP },
    { addr: '10.0.0.2:7443', fp: FP },
  ]);
  w.links[0]!.set('reconnecting');
  assert.equal(c.list()[0]!.reachable, false);
});

test("Let it run agents on this Mac sets this Mac to Run there, through the shell's link to the master", async () => {
  const w = world();
  const c = new Clusters(w.d);
  const got = await c.join({ link: LINK });
  await c.lend(got.id);
  assert.deepEqual(w.links[0]!.calls, [['device.set', { device: 'dev1', lending: 'run', request_id: `join-run-${got.id}` }]]);
  await assert.rejects(c.lend('ffff0000'), /not joined/);
});

test('a Run asked before the link to the master opens waits for it, and is refused when it does not connect', async () => {
  const w = world();
  w.goDown();
  const c = new Clusters(w.d);
  const got = await c.join({ link: LINK });
  await assert.rejects(c.lend(got.id), /the master did not answer: connect ECONNREFUSED/);
  assert.deepEqual(w.links[0]!.calls, []);
  w.links[0]!.set('reconnecting');
  const asked = c.lend(got.id);
  w.links[0]!.set('open');
  await asked;
  assert.equal(w.links[0]!.calls.length, 1);
  await c.leave(got.id); // its retries end once it is gone
});

test('a refused join leaves nothing behind; a bad link is refused before anything is made', async () => {
  const w = world();
  const c = new Clusters(w.d);
  w.refuse('the code is not valid');
  await assert.rejects(c.join({ link: LINK }), /the code is not valid/);
  await assert.rejects(c.join({ link: 'https://example.com' }));
  assert.deepEqual(fs.existsSync(`${w.d.home}-clusters`) ? fs.readdirSync(`${w.d.home}-clusters`) : [], []);
  assert.equal(w.kept().clusters, undefined);
  assert.deepEqual(c.list(), []);
});

test('leave sets this Mac Off there, revokes its device, stops the engine and removes its home and store', async () => {
  const w = world();
  const c = new Clusters(w.d);
  const { id } = await c.join({ link: LINK });
  await settle();
  const home = engineHome(w.d.home, id);
  await c.leave(id);
  assert.deepEqual(w.links[0]!.calls, [
    ['device.set', { device: 'dev1', lending: 'off', request_id: `leave-off-${id}` }],
    ['device.revoke', { device: 'dev1', request_id: `leave-revoke-${id}` }],
  ]);
  assert.equal(w.engines[0]!.state, 'stopped');
  assert.equal(w.engines[0]!.closed, true);
  assert.equal(fs.existsSync(home), false);
  assert.equal(fs.existsSync(w.d.store(home)), false);
  assert.deepEqual(w.kept().clusters, []);
  assert.deepEqual(c.list(), []);
  await assert.rejects(c.leave(id), /not joined/);
});

test('a Leave while the engine still starts waits for it, then stops it; a master that refuses does not keep this Mac', async () => {
  const w = world();
  w.slowStart();
  const c = new Clusters(w.d);
  const { id } = await c.join({ link: LINK });
  await settle();
  w.links[0]!.call = (async () => {
    throw new Error('refused');
  }) as MasterLink['call'];
  const leaving = c.leave(id);
  assert.deepEqual(c.list(), []); // gone at once: a second Leave finds nothing
  await assert.rejects(c.leave(id), /not joined/);
  w.engines[0]!.finishStart();
  await leaving;
  assert.equal(w.engines[0]!.state, 'stopped');
  assert.deepEqual(w.kept().clusters, []);
});

test('a revoke that ends the connection before it answers does not hold Leave up', async () => {
  const w = world();
  const c = new Clusters(w.d);
  const { id } = await c.join({ link: LINK });
  await settle();
  const l = w.links[0]!;
  let fail: (e: Error) => void = () => undefined;
  const close = l.close;
  l.close = () => (close(), fail(new Error('client closed')));
  l.call = (async (method: string) => {
    if (method !== 'device.revoke') return;
    // The master drops this device's connection with the revoke; the client keeps the call for a reconnect.
    setImmediate(() => l.set('reconnecting'));
    return new Promise((_, no) => (fail = no));
  }) as MasterLink['call'];
  await c.leave(id);
  assert.equal(w.engines[0]!.state, 'stopped');
  assert.deepEqual(w.kept().clusters, []);
});

test('a master that does not answer does not keep this Mac in its cluster', async () => {
  const w = world();
  const c = new Clusters(w.d);
  const { id } = await c.join({ link: LINK });
  await settle();
  w.links[0]!.set('reconnecting');
  await c.leave(id);
  assert.deepEqual(w.links[0]!.calls, []);
  assert.deepEqual(c.list(), []);
});

test('after a restart each kept cluster comes back with its engine; one whose home joined nothing is dropped', async () => {
  const w = world();
  const first = new Clusters(w.d);
  const { id } = await first.join({ link: LINK });
  await settle();
  w.d.file.write({
    ...w.kept(),
    clusters: [
      { id, name: 'Blue Harbor' },
      { id: '0000ffff', name: 'Gone', port: 50200 },
    ],
  });
  const again = new Clusters(w.d);
  await again.start();
  await settle();
  assert.deepEqual(
    again.list().map((x) => [x.id, x.name, x.reachable]),
    [[id, 'Blue Harbor', true]],
  );
  assert.deepEqual(
    w.engines.map((e) => e.state),
    ['running', 'running'],
  );
  // Kept without a port (joined before engines listened): it gets one, and its engine listens there.
  assert.deepEqual(w.kept().clusters, [{ id, name: 'Blue Harbor', port: 50101 }]);
  assert.equal(w.engines[1]!.port, 50101);
});

test("Make this Mac the master makes the cluster's engine its master: reached at its own door, kept as it, and not left", async () => {
  const w = world();
  const c = new Clusters(w.d);
  const { id } = await c.join({ link: LINK });
  await settle();
  assert.deepEqual(await c.promote(id), { number: 2, handed_over: false });
  const [got] = c.list();
  assert.equal(got!.master, true);
  assert.equal(got!.daemon, 'ws://127.0.0.1:50100/rpc');
  assert.equal(got!.token, 'tok');
  assert.equal(w.links[0]!.current().master.host, '127.0.0.1:50100'); // the next connect goes to it
  assert.deepEqual(c.doors()[0], { addr: '127.0.0.1:50100', fp: '' });
  await assert.rejects(c.promote(id), /master already/);
  await assert.rejects(c.leave(id), /make another of its machines the master first/);
  assert.equal(w.engines[0]!.state, 'running');
  const again = new Clusters(w.d); // after a restart it is still this cluster's master
  await again.start();
  await settle();
  assert.equal(again.list()[0]!.master, true);
  await assert.rejects(c.promote('0000ffff'), /not joined/);
});

test('while this Mac is being made the master, Leave and a second promote are refused', async () => {
  const w = world();
  const c = new Clusters(w.d);
  const { id } = await c.join({ link: LINK });
  await settle();
  const finish = w.slowHandover();
  const promoting = c.promote(id);
  await assert.rejects(c.leave(id), /being made/);
  await assert.rejects(c.promote(id), /being made/);
  finish();
  await promoting;
  assert.equal(c.list()[0]!.master, true);
});

test("an engine listens where this Mac's daemon does, on its own port, and serves the addresses chosen in this Mac's own home", () => {
  const o = { env: () => undefined, root: '/app', home: '/u/.handline-clusters/abcd0123', ownHome: '/u/.handline' };
  const cmd = engineCmd({ ...o, listen: '100.64.0.2:50100' }, '/usr/bin');
  assert.equal(cmd.file, '/app/go/handlined');
  assert.deepEqual(cmd.args, ['-home', o.home, '-listen', '100.64.0.2:50100', '-addresses-home', '/u/.handline']);
  assert.equal(cmd.env!.HANDLINE_HOME, o.home);
  assert.deepEqual(engineCmd({ ...o, listen: undefined }, '/usr/bin').args, ['-home', o.home]); // kept without a port: no door
});

test("each engine's home says what this app runs and has ready, and gives back what a master asked of it", async () => {
  const w = world();
  const c = new Clusters(w.d);
  c.tellApp({ version: '0.1.16', ready: '' });
  const { id } = await c.join({ link: LINK });
  const home = engineHome(w.d.home, id);
  const app = () => JSON.parse(fs.readFileSync(path.join(home, 'app.json'), 'utf8'));
  assert.deepEqual(app(), { version: '0.1.16', ready: '' }); // a join after the app said it
  c.tellApp({ version: '0.1.16', ready: '0.1.17' });
  assert.deepEqual(app(), { version: '0.1.16', ready: '0.1.17' });
  assert.equal(c.appUpdate(), null);
  fs.writeFileSync(path.join(home, 'app-update.json'), JSON.stringify({ version: '0.1.17', auto: true }));
  assert.deepEqual(c.appUpdate(), { version: '0.1.17', auto: true });
  fs.writeFileSync(path.join(home, 'app-update.json'), '{nope');
  assert.equal(c.appUpdate(), null);
});
