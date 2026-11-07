#!/usr/bin/env node
// The app against a real daemon, on a scratch home (never the live one): start it, do a follow-main restart, quit it, kill -9 it and
// start it again. After each start: exactly one handlined holds the home, the app says the daemon is running, and the app answers (an
// error dialog would hold its main process); no "another daemon holds this home". The daemon outlives the app: after the quit and the
// kill -9 the same daemon still runs, and the next start adopts it; the follow restart, a new build, starts it again once. A local gate on macOS, run for a change to
// apps/electron owner, main or follow, because the fakes the unit tests use passed T8410 and T8439 and still broke live (T8426, T8429, T8438).
//   tools/app-smoke.mjs [--keep]      tests the commit HEAD (commit first); about 3 minutes, most of it the follow restart's builds
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { daemonsOf, dispose, listening, scratch, sleep, startApp, until } from './app-instance.mjs';

const top = path.resolve(import.meta.dirname, '..');
const keep = process.argv.includes('--keep');
if (os.platform() !== 'darwin') {
  console.error('tools/app-smoke.mjs: the app is run on a Mac; this is a local gate, not a CI job');
  process.exit(2);
}
const sh = (cwd, file, args, env = {}) =>
  execFileSync(file, args, { cwd, encoding: 'utf8', env: { ...process.env, ...env }, stdio: ['ignore', 'pipe', 'inherit'] });
const { root, home, env, listenPort, webPort, debugPort, log } = await scratch('hl-smoke-');
const source = path.join(root, 'source');
const checkout = path.join(root, 'checkout');
env.HANDLINE_FOLLOW = '1';
let failure = null;
const pids = new Set();

const daemons = () => daemonsOf(root);

/** One call in the page, over the DevTools port: the app's own answer, as the window would get it. */
async function inPage(expression) {
  const pages = await (await fetch(`http://127.0.0.1:${debugPort}/json`)).json();
  const page = pages.find((p) => p.type === 'page' && p.url.startsWith(`http://127.0.0.1:${webPort}/`));
  if (!page) throw new Error('no page yet');
  const ws = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((res, rej) => ((ws.onopen = res), (ws.onerror = () => rej(new Error('devtools socket')))));
  try {
    return await new Promise((res, rej) => {
      const t = setTimeout(() => rej(new Error('the app did not answer in 10s (a dialog holding its main process?)')), 10_000);
      ws.onmessage = (m) => {
        const r = JSON.parse(m.data);
        if (r.id !== 1) return;
        clearTimeout(t);
        r.result?.exceptionDetails ? rej(new Error(r.result.exceptionDetails.text)) : res(r.result?.result?.value);
      };
      ws.send(JSON.stringify({ id: 1, method: 'Runtime.evaluate', params: { expression, awaitPromise: true, returnByValue: true } }));
    });
  } finally {
    ws.close();
  }
}

const start = () => startApp(path.join(checkout, 'apps', 'electron'), env, log);
const appPid = () => listening(debugPort)[0];

/** The app is up and the daemon runs: one handlined, the tray's state running, the app answering, and no complaint in its output. */
/** The last word the app wrote in daemon.log about how its daemon came: adopted, or started. */
const came = () =>
  /the daemon is running \((adopted|started)\)[^\n]*\n(?![\s\S]*the daemon is running \()/.exec(
    fs.readFileSync(path.join(env.HANDLINE_DATA, 'daemon.log'), 'utf8'),
  )?.[1];

async function expectRunning(what) {
  await until(`${what}: the daemon running`, 120_000, async () => (await inPage('handline.daemon()'))?.state === 'running');
  const d = await inPage('handline.daemon()');
  const n = daemons();
  n.forEach((p) => pids.add(p));
  if (n.length !== 1) throw new Error(`${what}: ${n.length} handlined for this home (${n.join(', ')}), expected 1`);
  if (listening(listenPort).length !== 1)
    throw new Error(`${what}: ${listening(listenPort).length} processes on the daemon's port, expected 1`);
  const text = fs.readFileSync(log, 'utf8');
  if (/another daemon holds this home/i.test(text + d.reason)) throw new Error(`${what}: "another daemon holds this home"`);
  console.log(`ok  ${what}: handlined ${n[0]}, daemon ${d.state}, app ${appPid()}`);
}

try {
  console.log(`scratch ${root}`);
  // GitHub (github), the repo the app's checkout was cloned from (source, where the app looks at origin/main), and the checkout it runs from.
  // main moves by a push to github; the commit under test is its first main.
  const github = path.join(root, 'github.git');
  sh(root, 'git', ['init', '-q', '--bare', '-b', 'main', github]);
  sh(top, 'git', ['push', '-q', github, 'HEAD:refs/heads/main']);
  sh(root, 'git', ['clone', '-q', github, source]);
  sh(root, 'git', ['clone', '-q', source, checkout]);
  // The installed packages of this checkout, cloned file by file on APFS: no install, no network.
  for (const rel of execFileSync('git', ['ls-files', '-z', '--', '**/package.json', 'package.json'], { cwd: top, encoding: 'utf8' })
    .split('\0')
    .filter(Boolean)) {
    const nm = path.join(path.dirname(rel), 'node_modules');
    if (fs.existsSync(path.join(top, nm))) sh(root, 'cp', ['-Rc', path.join(top, nm), path.join(checkout, nm)]);
  }
  console.log('building the checkout');
  sh(checkout, 'pnpm', ['--filter', '@handline/web', 'build']);
  sh(checkout, 'pnpm', ['--filter', '@handline/electron', 'build']);
  sh(checkout, 'bash', ['tools/build-go.sh']);

  start();
  await expectRunning('first start');
  if (came() !== 'started') throw new Error(`first start: the daemon was ${came()}, expected started`);

  // Main moves: the app prepares the update by itself, and Restart now builds, quits and relaunches.
  const first = appPid();
  const before = await inPage('handline.update()');
  if (before.phase === 'ready') throw new Error('an update was ready before main moved');
  fs.mkdirSync(path.join(source, 'docs'), { recursive: true });
  fs.writeFileSync(path.join(source, 'docs', 'app-smoke.md'), `moved ${Date.now()}\n`);
  sh(source, 'git', ['add', '-A']);
  sh(source, 'git', ['-c', 'user.name=smoke', '-c', 'user.email=smoke@localhost', 'commit', '-q', '-m', 'main moves']);
  sh(source, 'git', ['push', '-q', 'origin', 'HEAD:main']);
  await until('the update ready', 300_000, async () => (await inPage('handline.update()'))?.phase === 'ready');
  // The app quits at once to relaunch, so this call may get no answer: the next step checks a new app came up.
  await inPage("handline.updateAct('restart')").catch(() => undefined);
  await until('the old app gone and a new one up', 120_000, () => appPid() && appPid() !== first);
  await expectRunning('after the follow restart');
  if (came() !== 'started') throw new Error(`follow restart: the daemon of the old build was ${came()}, expected started again`);
  const [moved, runs] = [sh(source, 'git', ['rev-parse', 'HEAD']), sh(checkout, 'git', ['rev-parse', 'HEAD'])];
  if (moved !== runs) throw new Error(`the follow restart left the checkout at ${runs.trim()}, main is ${moved.trim()}`);

  // Quit the way the system asks an app to (SIGTERM): the app goes, its daemon keeps running, and the next start adopts it.
  const kept = daemons();
  process.kill(Number(appPid()), 'SIGTERM');
  await until('the app gone after quit', 60_000, () => !listening(debugPort).length);
  await sleep(3000);
  if (daemons().join() !== kept.join())
    throw new Error(`after quit the daemon is ${daemons().join() || 'gone'}, expected ${kept.join()} still running`);
  console.log(`ok  quit: the app is gone, handlined ${kept.join()} still runs`);
  start();
  await expectRunning('start after quit');
  if (came() !== 'adopted' || daemons().join() !== kept.join())
    throw new Error(`start after quit: the daemon was ${came()} (${daemons().join()}), expected ${kept.join()} adopted`);

  // A kill -9 leaves no chance to clean up: the daemon still runs, and the next start adopts it too.
  process.kill(Number(appPid()), 'SIGKILL');
  await until('the killed app gone', 30_000, () => !listening(debugPort).length);
  start();
  await expectRunning('start after kill -9');
  if (came() !== 'adopted' || daemons().join() !== kept.join())
    throw new Error(`start after kill -9: the daemon was ${came()} (${daemons().join()}), expected ${kept.join()} adopted`);
  console.log('PASS app-smoke');
} catch (e) {
  failure = e;
  console.error(`FAIL app-smoke: ${e.message}\nthe app's output: ${log}`);
} finally {
  dispose({ root, home, debugPort, pids: [...pids] }, { keepFiles: true });
  if (!keep && !failure) fs.rmSync(root, { recursive: true, force: true });
  else console.error(`kept ${root}`);
}
process.exit(failure ? 1 : 0);
