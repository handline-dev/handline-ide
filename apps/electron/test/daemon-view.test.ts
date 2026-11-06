import { test } from 'node:test';
import assert from 'node:assert/strict';
import { CHANNELS, DaemonView } from '#src/bridge.ts';
import { makeApi, type Ipc } from '#src/api.ts';

const ORIGIN = 'http://127.0.0.1:17443';
const win = { senderFrame: { url: `${ORIGIN}/`, parent: null } };
const frame = { senderFrame: { url: `${ORIGIN}/`, parent: {} } };

test('the page reads the daemon state and presses Start only from the window, and only when the app starts a daemon', () => {
  let started = 0;
  let canStart = true;
  const d = new DaemonView(ORIGIN, {
    view: () => ({ state: 'failed', reason: 'the daemon exited at start (exit 1)', canStart }),
    start: () => void started++,
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
