// The pieces tools/app-smoke.mjs and tools/app.mjs (hl app) share to run a separate test instance of the desktop app: a scratch home, free
// ports (never a fixed one), the app started from a checkout on them, and the processes it leaves found and ended.
import { spawn, spawnSync } from 'node:child_process';
import fs from 'node:fs';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

/** Calls f until it returns something true, for at most ms; `what` names the wait in the error. */
export const until = async (what, ms, f) => {
  const end = Date.now() + ms;
  let last = null;
  while (Date.now() < end) {
    try {
      const v = await f();
      if (v) return v;
    } catch (e) {
      last = e;
    }
    await sleep(500);
  }
  throw new Error(`timed out waiting for ${what}${last ? `: ${last.message}` : ''}`);
};

/** A port nothing listens on now, chosen by the system. */
export const freePort = () =>
  new Promise((resolve) => {
    const s = net.createServer().listen(0, '127.0.0.1', () => {
      const { port } = s.address();
      s.close(() => resolve(port));
    });
  });

/** The pids listening on a TCP port. */
export const listening = (port) => {
  const r = spawnSync('lsof', ['-nP', `-iTCP:${port}`, '-sTCP:LISTEN', '-t'], { encoding: 'utf8' });
  return [...new Set(r.stdout.split('\n').filter(Boolean))];
};

/** The handlined processes of a scratch root: those whose command line names it. */
export const daemonsOf = (root) =>
  spawnSync('ps', ['-axo', 'pid=,command='], { encoding: 'utf8' })
    .stdout.split('\n')
    .filter((l) => /\/handlined( |$)/.test(l) && l.includes(root))
    .map((l) => l.trim().split(/\s+/)[0]);

/** A fresh scratch root with the three ports the app uses, and the environment that keeps the app off everything of the user's. */
export async function scratch(prefix) {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), prefix)));
  const [listenPort, webPort, debugPort] = [await freePort(), await freePort(), await freePort()];
  const home = path.join(root, 'home');
  const env = {
    HANDLINE_HOME: home,
    HANDLINE_TEST_ROOT: path.join(root, 'user'), // the daemon's stand-in for the user's home: its protected store is kept here, not in the real one
    HANDLINE_DATA: path.join(root, 'data'),
    HANDLINE_BUNDLE_DIR: path.join(root, 'bundles'),
    HANDLINE_LISTEN: `127.0.0.1:${listenPort}`,
    HANDLINE_WEB_PORT: String(webPort),
    HANDLINE_DEBUG_PORT: String(debugPort),
    HANDLINE_BACKGROUND: '1', // no window to the front, no dock icon
    HANDLINE_AGENT: '',
    TMPDIR: path.join(root, 'tmp') + '/',
  };
  fs.mkdirSync(env.TMPDIR, { recursive: true });
  return { root, home, env, listenPort, webPort, debugPort, log: path.join(root, 'app.log') };
}

/** Starts the app of `appDir` (apps/electron) with env added to ours, its output to log; returns the child, which outlives this process. */
export function startApp(appDir, env, log) {
  const out = fs.openSync(log, 'a');
  const child = spawn('node', ['launch.ts'], { cwd: appDir, env: { ...process.env, ...env }, stdio: ['ignore', out, out], detached: true });
  child.unref();
  return child;
}

/** Ends the scratch root's processes (the app, its daemon, any more in pids) and takes its files and the daemon's protected store away. */
export function dispose({ root, home, debugPort, pids = [] }, { keepFiles = false } = {}) {
  for (const p of [...listening(debugPort), ...pids, ...daemonsOf(root)]) {
    try {
      process.kill(Number(p), 'SIGKILL');
    } catch {
      /* gone */
    }
  }
  // The daemon keeps its protected store outside the home, named for it: the one of this scratch home goes with it.
  fs.rmSync(path.join(os.homedir(), '.config', 'handline', 'protected', home.replaceAll('/', '-')), { recursive: true, force: true });
  if (!keepFiles) fs.rmSync(root, { recursive: true, force: true });
}
