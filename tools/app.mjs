#!/usr/bin/env node
// An agent runs, looks at and presses a separate test instance of this worktree's desktop app, never the user's window: it has its own
// scratch home, data folder, free ports (never 9333) and a DevTools port only it knows. Clicks are the page's own (DevTools input).
// Usage: app.mjs start [--fake] [--no-build] | shot [--out FILE] | click <css | x,y> | type "<text>" | text <css> | eval "<js>"
//   | drag-check (clickable things whose middle is an `-webkit-app-region: drag` area)
//   | overflow [width ...] (what runs past the window's right edge at each width; default 480 768 1280) | status | stop [--all]
import { execFileSync, spawn } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { dispose, freePort, listening, scratch, startApp, until } from './app-instance.mjs';

const top = path.resolve(import.meta.dirname, '..');
const home = process.env.HANDLINE_HOME ?? path.join(os.homedir(), '.handline');
const agent = process.env.HANDLINE_AGENT ?? '';
/** One state file per instance: what `status` and `stop --all` read. */
const stateDir = path.join(home, 'app-instances');
const stateFile = path.join(stateDir, `${path.basename(top)}.json`);

const die = (msg) => {
  console.error(`app.mjs: ${msg}`);
  process.exit(1);
};
const readState = () => {
  try {
    return JSON.parse(fs.readFileSync(stateFile, 'utf8'));
  } catch {
    return null;
  }
};
const alive = (s) => s && listening(s.debugPort).length > 0;
const need = () => {
  const s = readState();
  if (!alive(s)) die('no test app is running for this worktree: node tools/app.mjs start');
  return s;
};

/** Ends an instance and forgets it. */
function end(s, file) {
  dispose({ ...s, pids: [...(s.pids ?? [])] });
  fs.rmSync(file, { force: true });
}

// ---- over the DevTools port ----------------------------------------------------------------------------------------------------------

/** One DevTools socket on the app's page, for the calls `use` makes on it: what a call sets up (an emulated width) lasts as long as its
 *  socket. These helpers throw, never exit: start's catch must end the instance. */
async function session(s, use) {
  const pages = await (await fetch(`http://127.0.0.1:${s.debugPort}/json`)).json();
  const page = pages.find((p) => p.type === 'page' && p.url.startsWith(`http://127.0.0.1:${s.webPort}/`));
  if (!page) throw new Error('the app has no page yet (still starting?)');
  const ws = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((res, rej) => ((ws.onopen = res), (ws.onerror = () => rej(new Error('devtools socket')))));
  let id = 0;
  const call = (method, params = {}) =>
    new Promise((res, rej) => {
      const mine = ++id;
      const t = setTimeout(() => rej(new Error('the app did not answer in 15s')), 15_000);
      const on = (m) => {
        const r = JSON.parse(m.data);
        if (r.id !== mine) return;
        clearTimeout(t);
        ws.removeEventListener('message', on);
        r.error ? rej(new Error(r.error.message)) : res(r.result);
      };
      ws.addEventListener('message', on);
      ws.send(JSON.stringify({ id: mine, method, params }));
    });
  try {
    return await use(call);
  } finally {
    ws.close();
  }
}
/** One DevTools call on the app's page; returns its result. */
const cdp = (s, method, params = {}) => session(s, (call) => call(method, params));
async function evaluate(s, expression) {
  const r = await cdp(s, 'Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true });
  if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description ?? r.exceptionDetails.text);
  return r.result.value;
}

/** The point to press for a selector or "x,y". */
async function pointOf(s, target) {
  const xy = /^(-?\d+(?:\.\d+)?),(-?\d+(?:\.\d+)?)$/.exec(target);
  if (xy) return { x: Number(xy[1]), y: Number(xy[2]) };
  const p = await evaluate(
    s,
    `(() => { const e = document.querySelector(${JSON.stringify(target)}); if (!e) return null; e.scrollIntoView({ block: 'center', inline: 'center' });
      const r = e.getBoundingClientRect(); return { x: r.left + r.width / 2, y: r.top + r.height / 2, w: r.width, h: r.height }; })()`,
  );
  if (!p) throw new Error(`nothing matches ${target}`);
  if (!p.w && !p.h) throw new Error(`${target} has no size on screen`);
  return p;
}

/** The outermost elements whose box runs past the window's right edge: content a narrow window hides instead of wrapping or cutting. */
const OVERFLOW_CHECK = `(() => {
  const w = document.documentElement.clientWidth;
  const over = new Set([...document.querySelectorAll('body *')].filter((e) => {
    const r = e.getBoundingClientRect();
    return r.width > 0 && r.height > 0 && r.right > w + 1;
  }));
  const inner = (e) => {
    for (let p = e.parentElement; p; p = p.parentElement) if (over.has(p)) return true;
    return false;
  };
  return [...over].filter((e) => !inner(e)).map((e) => ({
    what: e.tagName.toLowerCase() + (e.className && typeof e.className === 'string' ? '.' + e.className.trim().split(/\\s+/).slice(0, 4).join('.') : ''),
    right: Math.round(e.getBoundingClientRect().right),
    text: (e.textContent ?? '').trim().replace(/\\s+/g, ' ').slice(0, 50),
  }));
})()`;

/** The clickable things whose middle point a window-drag area covers. Chromium builds the drag area from the elements in document order: a
 *  `drag` one adds its box, a `no-drag` one takes its box away, and the last box over a point decides. The same rule, run in the page. */
const DRAG_CHECK = `(() => {
  const region = (e) => getComputedStyle(e).getPropertyValue('-webkit-app-region').trim();
  const boxes = [];
  for (const e of document.querySelectorAll('*')) {
    const v = region(e);
    if (v !== 'drag' && v !== 'no-drag') continue;
    const r = e.getBoundingClientRect();
    if (r.width && r.height) boxes.push({ e, v, r });
  }
  const name = (e) => {
    const t = e.tagName.toLowerCase();
    const id = e.id ? '#' + e.id : '';
    const cls = typeof e.className === 'string' && e.className.trim() ? '.' + e.className.trim().split(/\\s+/).slice(0, 2).join('.') : '';
    return t + id + cls;
  };
  const dragAt = (x, y) => {
    let hit = null;
    for (const b of boxes)
      if (x >= b.r.left && x < b.r.right && y >= b.r.top && y < b.r.bottom) hit = b.v === 'drag' ? b.e : null;
    return hit;
  };
  const out = [];
  const sel = 'button, a[href], [role=button], [role=tab], [role=link], [role=menuitem], input, select, textarea, summary, [onclick]';
  for (const e of document.querySelectorAll(sel)) {
    const r = e.getBoundingClientRect();
    const cs = getComputedStyle(e);
    if (!r.width || !r.height || cs.visibility === 'hidden' || cs.pointerEvents === 'none') continue;
    const x = r.left + r.width / 2;
    const y = r.top + r.height / 2;
    if (x < 0 || y < 0 || x >= innerWidth || y >= innerHeight) continue;
    const top = document.elementFromPoint(x, y);
    if (top && top !== e && !e.contains(top)) continue; // covered by something else
    const d = dragAt(x, y);
    if (!d) continue;
    out.push({
      what: name(e), label: (e.getAttribute('aria-label') || e.textContent || e.title || '').trim().replace(/\\s+/g, ' ').slice(0, 40),
      at: Math.round(x) + ',' + Math.round(y), underDrag: name(d),
    });
  }
  return out;
})()`;

// ---- commands ------------------------------------------------------------------------------------------------------------------------

const [verb, ...rest] = process.argv.slice(2);
const flag = (n) => rest.includes(n);
const opt = (n) => (rest.includes(n) ? rest[rest.indexOf(n) + 1] : undefined);
// Only the flags this tool knows are dropped, so an argument like "--foo" reaches click, type and eval.
const positional = () => rest.filter((a, i) => !['--fake', '--no-build', '--all', '--out'].includes(a) && rest[i - 1] !== '--out');
if (os.platform() !== 'darwin') die('the desktop app is run on a Mac');

const commands = {
  async start() {
    const old = readState();
    if (alive(old)) die(`already running (app ${listening(old.debugPort)[0]}); node tools/app.mjs stop first`);
    if (old) end(old, stateFile); // what a crash left
    const sh = (file, args) => execFileSync(file, args, { cwd: top, stdio: ['ignore', 2, 2] });
    if (!flag('--no-build')) {
      console.error('building the web app and the shell');
      sh('pnpm', ['--filter', '@handline/web', 'build']);
      sh('pnpm', ['--filter', '@handline/electron', 'build']);
      if (!flag('--fake')) sh('bash', ['tools/build-go.sh']);
    }
    const s = await scratch('hl-app-');
    fs.mkdirSync(stateDir, { recursive: true });
    const state = { ...s, top, owner: agent, pids: [], fake: flag('--fake') };
    fs.writeFileSync(stateFile, JSON.stringify(state, null, 2)); // before anything runs, so a failure below still finds it
    try {
      // HOME is the daemon's stand-in user home, where its founder key is kept: the app reads the key there to pair itself.
      const env = { ...s.env, HOME: s.env.HANDLINE_TEST_ROOT };
      if (flag('--fake')) {
        const wsPort = await freePort();
        const fake = spawn(
          'node',
          ['packages/fake-daemon/src/serve.ts', '--seed', 'all', '--ws', String(wsPort), '--socket', path.join(s.root, 'fake.sock')],
          {
            cwd: top,
            stdio: 'ignore',
            detached: true,
          },
        );
        fake.unref();
        state.pids.push(String(fake.pid));
        env.HANDLINE_WS = `ws://127.0.0.1:${wsPort}`;
        delete env.HANDLINE_LISTEN;
        fs.writeFileSync(stateFile, JSON.stringify(state, null, 2));
      }
      startApp(path.join(top, 'apps', 'electron'), env, s.log);
      await until('the app window', 120_000, async () => {
        const pages = await (await fetch(`http://127.0.0.1:${s.debugPort}/json`)).json();
        return pages.some((p) => p.type === 'page' && p.url.startsWith(`http://127.0.0.1:${s.webPort}/`));
      });
      await until('the page loaded', 60_000, async () => (await evaluate(state, 'document.readyState')) === 'complete');
    } catch (e) {
      end(state, stateFile);
      die(`${e.message} (the app's output was ${s.log}, now removed with the instance)`);
    }
    console.log(
      `test app up: app ${listening(s.debugPort)[0]}, debug port ${s.debugPort}, page http://127.0.0.1:${s.webPort}/, files ${s.root}`,
    );
  },
  async shot() {
    const s = need();
    const out = path.resolve(opt('--out') ?? path.join(s.root, `shot-${Date.now()}.png`));
    const r = await cdp(s, 'Page.captureScreenshot', { format: 'png' });
    fs.writeFileSync(out, Buffer.from(r.data, 'base64'));
    console.log(out);
  },
  async click() {
    const s = need();
    const [target] = positional();
    if (!target) die('usage: node tools/app.mjs click <css | x,y>');
    const { x, y } = await pointOf(s, target);
    for (const type of ['mouseMoved', 'mousePressed', 'mouseReleased'])
      await cdp(s, 'Input.dispatchMouseEvent', { type, x, y, button: 'left', clickCount: type === 'mouseMoved' ? 0 : 1 });
    console.log(`clicked ${Math.round(x)},${Math.round(y)}`);
  },
  async type() {
    const s = need();
    const [text] = positional();
    if (text === undefined) die('usage: node tools/app.mjs type "<text>"');
    await cdp(s, 'Input.insertText', { text });
  },
  async text() {
    const s = need();
    const [css] = positional();
    if (!css) die('usage: node tools/app.mjs text <css>');
    const t = await evaluate(s, `document.querySelector(${JSON.stringify(css)})?.innerText ?? null`);
    if (t === null) die(`nothing matches ${css}`);
    console.log(t);
  },
  async eval() {
    const s = need();
    const [js] = positional();
    if (!js) die('usage: node tools/app.mjs eval "<js>"');
    const v = await evaluate(s, js);
    console.log(typeof v === 'string' ? v : JSON.stringify(v));
  },
  async 'drag-check'() {
    const s = need();
    const found = await evaluate(s, DRAG_CHECK);
    if (!found.length) return console.log('no clickable thing sits under a window-drag area');
    for (const f of found) console.log(`${f.what}${f.label ? ` "${f.label}"` : ''} at ${f.at} is under the drag area of ${f.underDrag}`);
    console.log(
      `${found.length} found: a click there starts a window drag, not the button. Release it with -webkit-app-region: no-drag on an element that comes after the drag area in the document: the last one over a point decides.`,
    );
    process.exitCode = 1;
  },
  async overflow() {
    const s = need();
    const widths = positional().map(Number);
    if (widths.some((w) => !Number.isInteger(w) || w <= 0)) die('usage: node tools/app.mjs overflow [width ...] (whole pixels)');
    let bad = 0;
    await session(s, async (call) => {
      for (const width of widths.length ? widths : [480, 768, 1280]) {
        await call('Emulation.setDeviceMetricsOverride', { width, height: 900, deviceScaleFactor: 0, mobile: false });
        await new Promise((r) => setTimeout(r, 500));
        const r = await call('Runtime.evaluate', { expression: OVERFLOW_CHECK, returnByValue: true });
        if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description ?? r.exceptionDetails.text);
        const found = r.result.value;
        bad += found.length;
        console.log(found.length ? `${width}px: ${found.length} past the right edge` : `${width}px: nothing past the right edge`);
        for (const f of found) console.log(`  ${f.what} reaches ${f.right}px: "${f.text}"`);
      }
    });
    if (bad) process.exitCode = 1;
  },
  status() {
    const s = readState();
    console.log(
      alive(s) ? `running: app ${listening(s.debugPort)[0]}, page http://127.0.0.1:${s.webPort}/, files ${s.root}` : 'not running',
    );
  },
  stop() {
    if (flag('--all') && !agent) die('stop --all needs HANDLINE_AGENT: without it every instance with no owner would match');
    const files = flag('--all')
      ? fs.existsSync(stateDir)
        ? fs.readdirSync(stateDir).map((f) => path.join(stateDir, f))
        : []
      : [stateFile];
    let n = 0;
    for (const f of files) {
      let s;
      try {
        s = JSON.parse(fs.readFileSync(f, 'utf8'));
      } catch {
        continue;
      }
      if (flag('--all') && s.owner !== agent) continue;
      end(s, f);
      n++;
    }
    console.log(n ? `stopped ${n} test app(s)` : 'none was running');
  },
};

if (!commands[verb]) die(`usage: node tools/app.mjs <${Object.keys(commands).join('|')}> ...`);
try {
  await commands[verb]();
} catch (e) {
  die(e.message); // a helper's failure: say it and exit non-zero, after the command's own cleanup
}
process.exit(process.exitCode ?? 0);
