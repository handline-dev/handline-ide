import { after, test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import type { ConnectionState } from '@handline/client';
import { Clusters, engineHome, type ClusterDeps, type Engine, type MasterLink } from '#src/clusters.ts';
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
  const engines: (Engine & { home: string; closed: boolean; finishStart: () => void })[] = [];
  let slow = false;
  const links: (MasterLink & { calls: unknown[]; set: (s: ConnectionState) => void })[] = [];
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
    joined: (home) => joinedHomes.get(home) ?? null,
    store: (home) => path.join(root, 'store', path.basename(home)),
    engine: (home) => {
      let begun: () => void = () => undefined;
      const e = {
        home,
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
    link: () => {
      const fns: ((s: ConnectionState) => void)[] = [];
      const l = {
        state: 'connecting' as ConnectionState,
        hello: null as MasterLink['hello'],
        calls: [] as unknown[],
        start: async () => (l.set('open'), l.hello!),
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
  return { d, kept: () => kept, engines, links, refuse: (why: string) => (refuse = why), slowStart: () => (slow = true) };
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
  assert.deepEqual(c.list(), [{ id: got.id, name: 'Blue Harbor', daemon: `wss://mac.local:7443/rpc`, token: 'tok', reachable: true }]);
  assert.deepEqual(w.kept().clusters, [{ id: got.id, name: 'Blue Harbor' }]);
  assert.deepEqual(c.doors(), [
    { addr: 'mac.local:7443', fp: FP },
    { addr: '10.0.0.2:7443', fp: FP },
  ]);
  w.links[0]!.set('reconnecting');
  assert.equal(c.list()[0]!.reachable, false);
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
  w.d.file.write({ ...w.kept(), clusters: [...w.kept().clusters!, { id: '0000ffff', name: 'Gone' }] });
  const again = new Clusters(w.d);
  again.start();
  await settle();
  assert.deepEqual(
    again.list().map((x) => [x.id, x.name, x.reachable]),
    [[id, 'Blue Harbor', true]],
  );
  assert.deepEqual(
    w.engines.map((e) => e.state),
    ['running', 'running'],
  );
  assert.deepEqual(w.kept().clusters, [{ id, name: 'Blue Harbor' }]);
});
