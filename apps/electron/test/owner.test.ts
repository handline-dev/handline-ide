import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {
  commands,
  detachedSpawn,
  lockPid,
  oldGatewayPid,
  Owner,
  portHolder,
  resolveClaude,
  daemonPath,
  markPathTimedOut,
  pathTimedOut,
  RESTART_WINDOW_MS,
  STOP_LIMIT_MS,
  WATCH_MS,
  type Cmd,
} from '#src/owner.ts';

const DAEMON: Cmd = { file: 'daemon', args: ['-terminal-tick', '-listen', '127.0.0.1:7443'] };

/** A home on a fake clock (wait() moves it). Its daemon, when there is one, holds the lock with a pid, answers unless deaf, and says
 * its build; SIGTERM ends it after exitsAfter ms (null: never). A spawned daemon answers, on the app's build, on the next poll. */
interface World {
  pid: number | null;
  answers: boolean;
  build: string | null;
  exitsAfter: number | null;
  stoppedAt?: number;
}

function rig(
  w: World,
  over: { appBuild?: string | null; gatewayStays?: boolean; neverUp?: boolean; doorUp?: () => Promise<boolean>; attachOnly?: boolean } = {},
) {
  const spawned: Cmd[] = [];
  const stopped: number[] = [];
  const clock = { now: 1_000_000 };
  const settle = () => {
    if (w.stoppedAt !== undefined && w.exitsAfter !== null && clock.now >= w.stoppedAt + w.exitsAfter) {
      w.pid = null;
      w.answers = false;
      w.stoppedAt = undefined;
    }
  };
  const owner = new Owner({
    socketUp: async () => (settle(), w.pid !== null && w.answers),
    daemonBuild: async () => (w.answers ? w.build : null),
    holder: async () => (settle(), w.pid),
    stopPid: (pid) => {
      stopped.push(pid);
      w.stoppedAt = clock.now;
      w.answers = false; // a stopping daemon closes its socket first
    },
    stopLeftovers: async () => {
      if (over.gatewayStays) throw new Error('the old gateway (pid 7) on /h/gateway.sock would not stop');
    },
    spawn: (cmd) => {
      spawned.push(cmd);
      Object.assign(w, { pid: 500 + spawned.length, answers: !over.neverUp, build: over.appBuild ?? 'aaaa111', exitsAfter: 250 });
    },
    daemon: DAEMON,
    wait: async (ms) => {
      clock.now += ms;
      await new Promise((r) => setImmediate(r));
    },
    now: () => clock.now,
    build: over.appBuild === undefined ? 'aaaa111' : over.appBuild,
    doorUp: over.doorUp,
    attachOnly: over.attachOnly,
    polls: 5,
  });
  return { owner, spawned, stopped, clock, w };
}

/** Lets the watch loop run until cond holds (each turn is one WATCH_MS on the fake clock). */
async function until(cond: () => boolean): Promise<void> {
  for (let i = 0; i < 500 && !cond(); i++) await new Promise((r) => setImmediate(r));
  assert.ok(cond(), 'the condition never held');
}

test('a daemon that answers with the app’s build is adopted: nothing is stopped or started', async () => {
  const r = rig({ pid: 42, answers: true, build: 'aaaa111fffffff', exitsAfter: 0 });
  await r.owner.start();
  r.owner.close();
  assert.equal(r.owner.state, 'running');
  assert.equal(r.owner.came, 'adopted');
  assert.deepEqual(r.stopped, []);
  assert.deepEqual(r.spawned, []);
});

test('a daemon of another build (the app was updated) is stopped and started again once, on the app’s build', async () => {
  const r = rig({ pid: 42, answers: true, build: 'bbbb222', exitsAfter: 1000 });
  await r.owner.start();
  r.owner.close();
  assert.equal(r.owner.state, 'running');
  assert.equal(r.owner.came, 'started');
  assert.deepEqual(r.stopped, [42]);
  assert.deepEqual(r.spawned, [DAEMON]);
  await r.owner.start(); // a second start meets the app's own build: nothing more
  assert.deepEqual(r.spawned, [DAEMON]);
});

test('a lock holder that does not answer within the stop limit is stopped and replaced; one that never exits fails with its pid', async () => {
  const r = rig({ pid: 42, answers: false, build: null, exitsAfter: 1000 });
  await r.owner.start();
  r.owner.close();
  assert.ok(r.clock.now - 1_000_000 >= STOP_LIMIT_MS, 'it waited the stop limit for an answer first');
  assert.deepEqual(r.stopped, [42]);
  assert.deepEqual(r.spawned, [DAEMON]);
  assert.equal(r.owner.state, 'running');

  const deaf = rig({ pid: 42, answers: false, build: null, exitsAfter: null });
  await deaf.owner.start();
  assert.equal(deaf.owner.state, 'failed');
  assert.match(deaf.owner.reason, /pid 42\) did not stop within 20s/);
  assert.deepEqual(deaf.spawned, []);
});

test('with nothing running the daemon is started on its own: detached, nothing on its stdin, no stdin pipe flag', async () => {
  const r = rig({ pid: null, answers: false, build: null, exitsAfter: 0 });
  await r.owner.start();
  r.owner.close();
  assert.equal(r.owner.came, 'started');
  assert.deepEqual(r.spawned, [DAEMON]);
  assert.deepEqual(detachedSpawn(9), { stdio: ['ignore', 9, 9], detached: true });
  assert.ok(!commands(() => undefined, '/repo', ':1').args.includes('-exit-with-stdin'), 'the daemon does not exit with the app');
});

test('a daemon that exits on its own is started again once; a second exit within a minute leaves it stopped with the reason', async () => {
  const r = rig({ pid: 42, answers: true, build: 'aaaa111', exitsAfter: 0 });
  await r.owner.start();
  Object.assign(r.w, { pid: null, answers: false }); // it exits
  await until(() => r.spawned.length === 1 && r.owner.state === 'running');
  Object.assign(r.w, { pid: null, answers: false }); // and again, within the minute
  await until(() => r.owner.state === 'stopped');
  assert.match(r.owner.reason, /stopped again within a minute/);
  assert.equal(r.spawned.length, 1);
  r.owner.close();

  const later = rig({ pid: 42, answers: true, build: 'aaaa111', exitsAfter: 0 });
  await later.owner.start();
  Object.assign(later.w, { pid: null, answers: false });
  await until(() => later.spawned.length === 1 && later.owner.state === 'running');
  later.clock.now += RESTART_WINDOW_MS + WATCH_MS;
  Object.assign(later.w, { pid: null, answers: false }); // past the minute: started again
  await until(() => later.spawned.length === 2 && later.owner.state === 'running');
  later.owner.close();
});

test('Stop daemon stops it and it stays stopped: the watch does not start it again', async () => {
  const r = rig({ pid: 42, answers: true, build: 'aaaa111', exitsAfter: 250 });
  await r.owner.start();
  await r.owner.stop();
  assert.equal(r.owner.state, 'stopped');
  assert.deepEqual(r.stopped, [42]);
  for (let i = 0; i < 20; i++) await new Promise((res) => setImmediate(res));
  assert.deepEqual(r.spawned, []);
  r.owner.close();
});

test('Restart daemon stops the one running and starts it again, also with another command (a listener that moves)', async () => {
  const r = rig({ pid: 42, answers: true, build: 'aaaa111', exitsAfter: 250 });
  await r.owner.start();
  const LOOPBACK: Cmd = { file: 'daemon', args: ['-listen', '127.0.0.1:7443'] };
  await r.owner.restart(LOOPBACK);
  r.owner.close();
  assert.deepEqual(r.stopped, [42]);
  assert.deepEqual(r.spawned, [LOOPBACK]);
  assert.equal(r.owner.state, 'running');
});

test('a daemon that never answers, or an old gateway that would not stop, is a failure with a reason', async () => {
  const silent = rig({ pid: null, answers: false, build: null, exitsAfter: 0 }, { neverUp: true });
  await silent.owner.start();
  assert.equal(silent.owner.state, 'failed');
  assert.match(silent.owner.reason, /did not answer in time/);
  const stuck = rig({ pid: null, answers: false, build: null, exitsAfter: 0 }, { gatewayStays: true });
  await stuck.owner.start();
  assert.match(stuck.owner.reason, /old gateway .* would not stop/);
  assert.deepEqual(stuck.spawned, []);
});

test('a run with no build stamp adopts whatever answers', async () => {
  const r = rig({ pid: 42, answers: true, build: 'cccc333', exitsAfter: 0 }, { appBuild: null });
  await r.owner.start();
  r.owner.close();
  assert.equal(r.owner.came, 'adopted');
});

test('commands: a checkout runs go/handlined with its listener; the home, the binary, the name and the page can be named', () => {
  assert.deepEqual(
    commands(() => undefined, '/repo', '127.0.0.1:7443'),
    {
      file: '/repo/go/handlined',
      args: ['-allow-real-home', '-claude', 'claude', '-terminal-tick', '-listen', '127.0.0.1:7443'],
    },
  );
  const named = commands((k) => ({ HOME: '/h', CLAUDE: '/x/claude', HANDLINED: '/x/h' })[k], '/repo', ':1');
  assert.deepEqual(named, {
    file: '/x/h',
    args: ['-home', '/h', '-allow-real-home', '-claude', '/x/claude', '-terminal-tick', '-listen', ':1'],
  });
  const ts = commands(() => undefined, '/repo', '100.101.2.3:7443', 'mac.tail1.ts.net:7443');
  assert.deepEqual(ts.args.slice(-4), ['-listen', '100.101.2.3:7443', '-advertise', 'mac.tail1.ts.net:7443']);
  assert.deepEqual(commands(() => undefined, '/repo', ':1', null, 'claude', '/app/web').args.slice(-2), ['-web', '/app/web']);
});

test('resolveClaude: the override wins; else the first executable claude on the login PATH, then the app PATH; none is a bare claude', () => {
  const have = new Set(['/home/u/.local/bin/claude', '/usr/bin/claude']);
  const isExec = (f: string) => have.has(f);
  assert.equal(resolveClaude('/x/claude', ['/usr/bin'], isExec), '/x/claude');
  assert.equal(resolveClaude(undefined, ['/home/u/.local/bin:/usr/bin', '/usr/bin'], isExec), '/home/u/.local/bin/claude');
  assert.equal(resolveClaude(undefined, [undefined, '/usr/bin'], isExec), '/usr/bin/claude');
  assert.equal(resolveClaude(undefined, ['/nowhere', undefined], isExec), 'claude');
});

test('daemonPath: the login PATH first, then the app PATH, each only when there is one', () => {
  assert.equal(daemonPath('/opt/homebrew/bin:/usr/bin', '/usr/bin:/bin'), '/opt/homebrew/bin:/usr/bin:/usr/bin:/bin');
  assert.equal(daemonPath(undefined, '/usr/bin:/bin'), '/usr/bin:/bin');
  assert.equal(daemonPath('/opt/homebrew/bin', undefined), '/opt/homebrew/bin');
});

test('a timed-out PATH is kept with the daemon: a later read, as a relaunch that adopts it makes, sees it until a start on the login PATH', () => {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), 'hl-owner-'));
  try {
    assert.equal(pathTimedOut(home), false);
    markPathTimedOut(home, true);
    assert.equal(pathTimedOut(home), true);
    markPathTimedOut(home, true);
    assert.equal(pathTimedOut(home), true);
    markPathTimedOut(home, false);
    assert.equal(pathTimedOut(home), false);
    markPathTimedOut(home, false);
    assert.equal(pathTimedOut(home), false);
  } finally {
    fs.rmSync(home, { recursive: true, force: true });
  }
});

test('lockPid reads the pid the daemon wrote in its lock file, and null when there is none', () => {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), 'hl-owner-'));
  try {
    assert.equal(lockPid(home), null);
    fs.mkdirSync(path.join(home, 'locks'));
    fs.writeFileSync(path.join(home, 'locks', 'daemon.lock'), '4242            \n');
    assert.equal(lockPid(home), 4242);
  } finally {
    fs.rmSync(home, { recursive: true, force: true });
  }
});

test('oldGatewayPid reads the pid an old gateway wrote in the protected store', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'hl-gw-'));
  try {
    assert.equal(oldGatewayPid(dir), null);
    fs.mkdirSync(path.join(dir, 'gateway'));
    fs.writeFileSync(path.join(dir, 'gateway', 'pid'), '87157\n');
    assert.equal(oldGatewayPid(dir), 87157);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

test('portHolder reads the listening process from lsof', () => {
  assert.deepEqual(portHolder('p87157\nchandlined\n'), { pid: 87157, command: 'handlined' });
  assert.equal(portHolder(''), null);
});

test('attach only (a dev run): whatever answers is adopted, whatever its build; nothing is started, stopped or replaced', async () => {
  const r = rig({ pid: 42, answers: true, build: 'bbbb222', exitsAfter: 0 });
  const attach = new Owner({ ...(r.owner as unknown as { d: ConstructorParameters<typeof Owner>[0] }).d, attachOnly: true });
  await attach.start();
  assert.equal(attach.came, 'adopted');
  assert.deepEqual([r.stopped, r.spawned], [[], []]);
  Object.assign(r.w, { pid: null, answers: false }); // it exits: not started again
  await until(() => attach.state === 'stopped');
  assert.deepEqual(r.spawned, []);
  attach.close();
  const none = new Owner({
    ...(rig({ pid: null, answers: false, build: null, exitsAfter: 0 }).owner as unknown as { d: ConstructorParameters<typeof Owner>[0] }).d,
    attachOnly: true,
  });
  await none.start();
  assert.equal(none.state, 'failed');
  assert.match(none.reason, /only attaches/);
});

test('a daemon whose loopback door does not answer is replaced; one attached to only fails with that reason', async () => {
  // The adopted daemon's door is dead; the one started in its place answers.
  const box: { spawned: Cmd[] } = { spawned: [] };
  const r = rig({ pid: 42, answers: true, build: 'aaaa111', exitsAfter: 0 }, { doorUp: async () => box.spawned.length > 0 });
  box.spawned = r.spawned;
  await r.owner.start();
  assert.deepEqual([r.stopped, r.owner.came], [[42], 'started']);
  r.owner.close();
  const attach = rig({ pid: 42, answers: true, build: 'aaaa111', exitsAfter: 0 }, { doorUp: async () => false, attachOnly: true }).owner;
  await attach.start();
  assert.equal(attach.state, 'failed');
  assert.match(attach.reason, /loopback door/);
  attach.close();
});
