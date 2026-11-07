import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { decide, Follower, peekMain, watchMain, type Main, type Watch } from '#src/follow.ts';
import { trayMenu } from '#src/tray.ts';

test('a web-only change rebuilds the page and nothing of the app; an app change rebuilds it too', () => {
  assert.deepEqual(decide(['apps/web/src/App.tsx', 'docs/x.md', 'tests/a.py']), {
    install: false,
    web: true,
    electron: false,
  });
  assert.deepEqual(decide(['packages/app-data/src/writes.ts']), { install: false, web: true, electron: false });
  assert.deepEqual(decide(['apps/electron/src/main.ts']), { install: false, web: true, electron: true });
  assert.equal(decide(['packages/client/src/client.ts']).electron, true); // the app bundles it
  assert.deepEqual(decide(['pnpm-lock.yaml']), { install: true, web: true, electron: true });
  assert.deepEqual(decide(['go/cmd/handlined/main.go', 'handline/daemon/x.py']), { install: false, web: false, electron: false });
  assert.deepEqual(decide(['docs/a.md']), { install: false, web: false, electron: false });
});

function rig(files: string[]) {
  const main: Main = { sha: 'aaa1111', title: 'first' };
  const calls: string[] = [];
  const ran: string[] = [];
  const f = new Follower({
    run: async (file, args) => {
      ran.push(`${file} ${args.join(' ')}`);
      if (args[0] === 'rev-parse') return 'aaa1111\n';
      if (args[0] === 'diff') return files.join('\n');
      return '';
    },
    peek: async () => main,
    clearWeb: () => calls.push('clear'),
    swapWeb: () => calls.push('swap'),
    relaunch: () => calls.push('relaunch'),
  });
  const fresh: boolean[] = [];
  f.onChange((x) => fresh.push(x));
  return { f, main, calls, ran, fresh };
}

test('main moving is news once; a main that moved again is news again, and one that went back is nothing', async () => {
  const r = rig([]);
  await r.f.init();
  assert.equal(r.f.state, 'current');
  r.main.sha = 'bbb2222';
  await r.f.look();
  assert.equal(r.f.state, 'available');
  assert.equal(r.fresh.at(-1), true);
  await r.f.look(); // the same main again is not news
  assert.equal(r.fresh.at(-1), false);
  r.main.sha = 'ccc3333';
  await r.f.look();
  assert.equal(r.fresh.at(-1), true);
  r.main.sha = 'aaa1111'; // main went back to what the app runs
  await r.f.look();
  assert.equal(r.f.state, 'current');
  assert.equal(r.f.next, null);
});

test('prepare fetches, moves the checkout to that main and builds the page beside the running one, then waits; nothing restarts', async () => {
  const r = rig(['apps/web/src/App.tsx']);
  await r.f.init();
  r.main.sha = 'bbb2222';
  await r.f.look();
  const steps: string[] = [];
  r.f.onChange(() => r.f.state === 'updating' && steps.push(`${r.f.step} ${Math.round(r.f.fraction * 100)}`));
  await r.f.prepare();
  assert.equal(r.f.state, 'ready');
  assert.deepEqual(
    r.ran.filter((c) => !c.startsWith('git rev-parse')),
    [
      'git fetch origin +refs/remotes/origin/main:refs/remotes/origin/main',
      'git diff --name-only aaa1111 bbb2222',
      'git checkout -B main bbb2222',
      'pnpm --filter @handline/web build --outDir dist-next',
      'pnpm --filter @handline/electron build',
      'bash tools/build-go.sh',
      'bash tools/build-hl.sh',
    ],
  );
  assert.deepEqual(steps, ['Preparing 0', 'Building (1 of 4) 0', 'Building (2 of 4) 25', 'Building (3 of 4) 50', 'Building (4 of 4) 75']);
  assert.deepEqual(r.calls, ['clear']);
  await r.f.look(); // the same main is not built again
  assert.equal(r.f.state, 'ready');
  assert.equal(r.ran.filter((c) => c.startsWith('git checkout -B main')).length, 1);
});

test('a ready update is built again when main moves past it, so Restart now lands on the newest main', async () => {
  const r = rig(['apps/electron/src/main.ts']);
  await r.f.init();
  r.main.sha = 'bbb2222';
  await r.f.look();
  await r.f.prepare();
  assert.equal(r.f.state, 'ready');
  r.main.sha = 'ccc3333';
  await r.f.look();
  assert.equal(r.f.state, 'ready');
  const moves = r.ran.filter((c) => c.startsWith('git checkout -B main'));
  assert.deepEqual(moves, ['git checkout -B main bbb2222', 'git checkout -B main ccc3333']);
  assert.ok(r.ran.includes('git diff --name-only aaa1111 ccc3333'), 'the diff is from the commit the app runs');
  await r.f.restartNow();
  assert.equal(r.ran.filter((c) => c === 'bash tools/build-go.sh').length, 2, 'the daemon is built for the newest main too');
  assert.equal(r.ran.filter((c) => c === 'bash tools/build-hl.sh').length, 2, 'and hl beside it');
  assert.deepEqual(r.calls.slice(-2), ['swap', 'relaunch']);
});

test('Restart now puts the built page in place and relaunches; it calls nothing on the running daemon', async () => {
  const r = rig(['apps/electron/src/main.ts']);
  await r.f.init();
  r.main.sha = 'ddd4444';
  await r.f.prepare();
  assert.ok(r.ran.includes('pnpm --filter @handline/electron build'));
  const built = r.ran.length;
  await r.f.restartNow();
  assert.equal(r.ran.length, built); // the daemon was built in prepare; the relaunched app starts it
  assert.deepEqual(r.calls, ['clear', 'swap', 'relaunch']);
});

test('a retry after a failed web build builds the page again, from where the first try began, and clears the half-built one', async () => {
  const ran: string[] = [];
  const calls: string[] = [];
  let head = 'aaa1111'; // the merge moves HEAD to the target, as git does
  let broken = true;
  const f = new Follower({
    run: async (file, args) => {
      ran.push(`${file} ${args.join(' ')}`);
      if (args[0] === 'rev-parse') return `${head}\n`;
      if (args[0] === 'diff') return args.includes('aaa1111') ? 'apps/web/src/App.tsx' : ''; // from the moved HEAD there is nothing to see
      if (args[0] === 'checkout') head = 'bbb2222';
      if (file === 'pnpm' && args.includes('@handline/web') && broken) throw new Error('vite failed');
      return '';
    },
    peek: async () => ({ sha: 'bbb2222', title: 't' }),
    clearWeb: () => calls.push('clear'),
    swapWeb: () => calls.push('swap'),
    relaunch: () => undefined,
  });
  await f.init();
  await f.prepare();
  assert.equal(f.state, 'failed');
  broken = false;
  ran.length = 0;
  await f.retry();
  assert.equal(f.state, 'ready');
  assert.ok(ran.includes('git diff --name-only aaa1111 bbb2222'));
  assert.ok(ran.includes('pnpm --filter @handline/web build --outDir dist-next'));
  assert.deepEqual(calls, ['clear', 'clear']); // each try starts from no half-built page
});

test('Restart now before anything is built does nothing', async () => {
  const r = rig([]);
  await r.f.init();
  await r.f.restartNow();
  assert.deepEqual(r.calls, []);
});

for (const step of ['swap', 'relaunch'] as const) {
  test(`a ${step} that throws ends Restart now in failed with its reason, and Try again repeats the restart, not the build`, async () => {
    const calls: string[] = [];
    let broken = true;
    const ran: string[] = [];
    const at = (name: string) => () => {
      if (name === step && broken) throw new Error(`${name} failed`);
      calls.push(name);
    };
    const f = new Follower({
      run: async (file, args) => (ran.push(`${file} ${args[0]}`), args[0] === 'diff' ? 'go/x.go' : 'aaa1111'),
      peek: async () => ({ sha: 'bbb2222', title: 't' }),
      clearWeb: () => calls.push('clear'),
      swapWeb: at('swap'),
      relaunch: at('relaunch'),
    });
    await f.init();
    await f.prepare();
    const built = ran.length;
    await f.restartNow();
    assert.equal(f.state, 'failed', 'never left at updating');
    assert.equal(f.reason, `${step} failed`);
    broken = false;
    calls.length = 0;
    await f.retry();
    assert.equal(ran.length, built); // nothing was built again
    assert.deepEqual(calls.slice(-1), ['relaunch']);
  });
}

test('a failed daemon build ends Preparing in failed with the build-go.sh output, and nothing relaunches', async () => {
  const calls: string[] = [];
  const f = new Follower({
    run: async (file, args) => {
      if (file === 'bash') throw new Error('build-go.sh: undefined: x');
      return args[0] === 'diff' ? 'go/x.go' : 'aaa1111';
    },
    peek: async () => ({ sha: 'bbb2222', title: 't' }),
    clearWeb: () => undefined,
    swapWeb: () => calls.push('swap'),
    relaunch: () => calls.push('relaunch'),
  });
  await f.init();
  await f.prepare();
  assert.equal(f.state, 'failed');
  assert.equal(f.reason, 'build-go.sh: undefined: x');
  await f.restartNow();
  assert.deepEqual(calls, []);
});

test('a failed build is shown with the command output behind it, nothing relaunches, and Try again builds again', async () => {
  const calls: string[] = [];
  let broken = true;
  const f = new Follower({
    run: async (file, args) => {
      if (file === 'pnpm' && broken) throw new Error('pnpm build: vite failed');
      return args[0] === 'diff' ? 'apps/web/a.ts' : 'aaa1111';
    },
    peek: async () => ({ sha: 'bbb2222', title: 't' }),
    clearWeb: () => undefined,
    swapWeb: () => undefined,
    relaunch: () => calls.push('relaunch'),
  });
  await f.init();
  await f.prepare();
  assert.equal(f.state, 'failed');
  assert.equal(f.reason, 'pnpm build: vite failed');
  assert.match(f.log, /\$ pnpm --filter @handline\/web build --outDir dist-next\npnpm build: vite failed/);
  assert.deepEqual(calls, []);
  broken = false;
  await f.retry();
  assert.equal(f.state, 'ready');
});

const actions = {
  open: () => undefined,
  inbox: () => undefined,
  startDaemon: () => undefined,
  setOpenAtLogin: () => undefined,
  quit: () => undefined,
  copy: () => undefined,
};

test('the menu says what the update flow says: updating, ready to restart, not updated; the commit is a detail', () => {
  const base = { sha: 'aaa1111ffff', next: null, reason: '', check: () => undefined, restart: () => undefined };
  const upd = (phase: 'idle' | 'preparing' | 'ready' | 'later' | 'failed', reason = '') => ({
    phase,
    reason,
    restart: () => undefined,
    retry: () => undefined,
  });
  const labels = (update: ReturnType<typeof upd>, details = false) =>
    trayMenu('running', '', actions, {
      unread: 0,
      working: 0,
      openAtLogin: true,
      remote: null,
      details,
      update,
      follow: { ...base, state: 'current' },
    }).map((i) => i.label ?? '-');
  assert.ok(!labels(upd('idle')).some((l) => /main|update|restart/i.test(l)));
  assert.ok(labels(upd('idle'), true).includes('Handline is on main aaa1111'));
  assert.ok(labels(upd('preparing')).includes('Updating Handline…'));
  assert.ok(labels(upd('ready')).includes('Handline is ready to restart') && labels(upd('ready')).includes('Restart now'));
  assert.ok(labels(upd('later')).includes('Restart now'));
  assert.ok(
    labels(upd('failed', 'vite failed')).includes('Handline was not updated: vite failed') && labels(upd('failed')).includes('Try again'),
  );
});

// fs.watch can start and then deliver nothing (macOS with an overloaded fseventsd); a bare watch on the same folder tells that from a bug in watchMain.
async function watchDelivers(dir: string): Promise<boolean> {
  let seen = false;
  const w = fs.watch(dir, () => (seen = true));
  await new Promise((r) => setTimeout(r, 100));
  fs.writeFileSync(path.join(dir, 'probe'), 'x');
  for (let i = 0; i < 20 && !seen; i++) await new Promise((r) => setTimeout(r, 50));
  w.close();
  return seen;
}

test('watchMain calls back once per pause for main, main.lock, packed-refs or an unnamed event, and never for another branch', (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const listeners = new Map<string, (event: string, name: string | null) => void>();
  const watch: Watch = (dir, f) => (listeners.set(dir, f), { on: () => undefined, close: () => undefined });
  const refs = path.join('/git', 'refs', 'remotes', 'origin');
  let n = 0;
  const stop = watchMain('/git', () => n++, assert.fail, { pause: 50, watch });
  t.after(stop);
  const ping = (dir: string, name: string | null) => listeners.get(dir)!('rename', name);
  ping(refs, 'other');
  t.mock.timers.tick(50);
  assert.equal(n, 0);
  ping(refs, 'main.lock'); // git writes main.lock and renames it onto main: one call for the pair
  t.mock.timers.tick(20);
  ping(refs, 'main');
  t.mock.timers.tick(50);
  assert.equal(n, 1);
  ping('/git', 'packed-refs');
  t.mock.timers.tick(50);
  assert.equal(n, 2);
  ping('/git', null); // some platforms give no name: it may be main
  t.mock.timers.tick(50);
  assert.equal(n, 3);
});

test('on the real file system, replacing the main ref calls back within 3 s', async (t) => {
  const git = fs.mkdtempSync(path.join(os.tmpdir(), 'watch-'));
  const refs = path.join(git, 'refs', 'remotes', 'origin');
  fs.mkdirSync(refs, { recursive: true });
  let n = 0;
  const errors: Error[] = [];
  const stop = watchMain(
    git,
    () => n++,
    (e) => errors.push(e),
    { pause: 50 },
  );
  t.after(stop);
  t.after(() => fs.rmSync(git, { recursive: true, force: true }));
  if (errors.length) return t.skip(`fs.watch is not available here: ${errors[0]!.message}`);
  if (!(await watchDelivers(git))) return t.skip('fs.watch delivers no events on this host');
  fs.writeFileSync(path.join(refs, 'main.lock'), 'x');
  fs.renameSync(path.join(refs, 'main.lock'), path.join(refs, 'main'));
  for (let i = 0; i < 60 && n === 0; i++) await new Promise((r) => setTimeout(r, 50));
  assert.ok(n >= 1, 'no call within 3 s');
});

test('main is GitHub main: the source repo fetches origin main first and reads origin/main, not its own main branch', async () => {
  const ran: string[] = [];
  const git = async (...args: string[]) => {
    ran.push(args.join(' '));
    return args[0] === 'rev-parse' ? 'ccc3333' : 'Merge pull request #1';
  };
  assert.deepEqual(await peekMain(git), { sha: 'ccc3333', title: 'Merge pull request #1' });
  assert.deepEqual(ran, ['fetch origin main', 'rev-parse origin/main', 'log -1 --format=%s origin/main']);
});

test('offline, the last fetched origin/main still answers', async () => {
  const git = async (...args: string[]) => {
    if (args[0] === 'fetch') throw new Error('git fetch: could not resolve host');
    return args[0] === 'rev-parse' ? 'ddd4444' : 't';
  };
  assert.equal((await peekMain(git)).sha, 'ddd4444');
});

test('a watch that cannot start or breaks is reported, not thrown', async () => {
  const errors: string[] = [];
  const stop = watchMain(
    path.join(os.tmpdir(), `no-such-${Date.now()}`),
    () => undefined,
    (e) => errors.push(e.message),
  );
  stop();
  assert.equal(errors.length, 2); // refs/remotes/origin and the folder itself

  const f = new Follower({
    run: async () => '',
    peek: async () => ({ sha: 'x', title: '' }),
    clearWeb: () => undefined,
    swapWeb: () => undefined,
    relaunch: () => undefined,
  });
  f.watchFailed(new Error('EMFILE'));
  assert.equal(f.state, 'failed');
  assert.equal(f.reason, 'cannot watch main: EMFILE');
});
