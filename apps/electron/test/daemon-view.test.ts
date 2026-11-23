import { test } from 'node:test';
import assert from 'node:assert/strict';
import { CHANNELS, DaemonView } from '#src/bridge.ts';
import { makeApi, type Ipc } from '#src/api.ts';

const ORIGIN = 'http://127.0.0.1:17443';
const win = { senderFrame: { url: `${ORIGIN}/`, parent: null } };
const frame = { senderFrame: { url: `${ORIGIN}/`, parent: {} } };

test('the page reads the daemon state and presses Start or Stop only from the window, and only when the app starts a daemon', async () => {
  let started = 0;
  let stopped = 0;
  let canStart = true;
  const d = new DaemonView(ORIGIN, {
    view: () => ({ state: 'failed', reason: 'the daemon exited at start (exit 1)', canStart }),
    start: () => void started++,
    stop: async () => void stopped++,
    keepDoors: () => {},
  });
  assert.deepEqual(d.view(win), { ok: true, state: 'failed', reason: 'the daemon exited at start (exit 1)', canStart: true });
  assert.equal(d.view(frame).ok, false);
  assert.equal(d.start(frame).ok, false);
  assert.equal(started, 0);
  assert.deepEqual(d.start(win), { ok: true });
  assert.equal(started, 1);
  canStart = false;
  assert.equal(d.start(win).ok, false);
  assert.equal(started, 1);
  assert.equal((await d.stop(frame)).ok, false);
  assert.equal(stopped, 0);
  canStart = true;
  assert.deepEqual(await d.stop(win), { ok: true });
  assert.equal(stopped, 1);
  canStart = false;
  assert.equal((await d.stop(win)).ok, false);
  assert.equal(stopped, 1);
});

test('the doors the page hands over are checked, and only from the window', () => {
  const kept: unknown[] = [];
  const d = new DaemonView(ORIGIN, {
    view: () => ({ state: 'running', reason: '', canStart: true }),
    start: () => {},
    stop: async () => {},
    keepDoors: (doors) => void kept.push(doors),
  });
  const fp = 'ab'.repeat(32);
  assert.deepEqual(d.keepDoors(win, [{ addr: '10.0.0.2:7443', fp }]), { ok: true });
  assert.equal(d.keepDoors(win, [{ addr: '10.0.0.2', fp }]).ok, false);
  assert.deepEqual(d.keepDoors(win, [{ addr: '127.0.0.1:7462', fp: '' }]), { ok: true });
  assert.equal(d.keepDoors(win, [{ addr: 'evil.example:443', fp: '' }]).ok, false);
  assert.equal(d.keepDoors(win, [{ addr: '10.0.0.2:7443', fp: 'zz' }]).ok, false);
  assert.equal(d.keepDoors(frame, [{ addr: '10.0.0.2:7443', fp }]).ok, false);
  assert.deepEqual(kept, [[{ addr: '10.0.0.2:7443', fp }], [{ addr: '127.0.0.1:7462', fp: '' }]]);
});

test('the preload keeps the last daemon state for a listener that registers after it, and sends every change', async () => {
  const on = new Map<string, (e: unknown, a: unknown) => void>();
  const ipc: Ipc = { invoke: async () => ({ ok: false }), on: (c, f) => void on.set(c, f) };
  const api = makeApi(ipc);
  const seen: string[] = [];
  on.get(CHANNELS.daemon)!({}, { state: 'starting', reason: '', canStart: true });
  api.onDaemon!((v) => seen.push(v.state));
  on.get(CHANNELS.daemon)!({}, { state: 'running', reason: '', canStart: true });
  assert.deepEqual(seen, ['starting', 'running']);
});
