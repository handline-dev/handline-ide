import { test } from 'node:test';
import assert from 'node:assert/strict';
import { PICK_LIMITS, PICK_STYLES } from '@handline/app-core';
import { ElementPicker, type Caller, type PickPage, type PickResult } from '#src/pick.ts';

const ORIGIN = 'http://127.0.0.1:5000';
/** A PNG header that says 2078 x 1510, then nothing the tests read. */
const png = Buffer.alloc(32);
png.writeUInt32BE(0x49484452, 12);
png.writeUInt32BE(2078, 16);
png.writeUInt32BE(1510, 20);

const button = {
  nodeId: 7,
  backendNodeId: 17,
  nodeType: 1,
  localName: 'button',
  attributes: ['class', 'btn primary', 'id', 'pay'],
  children: [{ nodeId: 8, backendNodeId: 18, nodeType: 3, nodeValue: 'Pay now' }],
};
const body = { nodeId: 3, backendNodeId: 13, nodeType: 1, localName: 'body', attributes: [], children: [button] };
const tree = {
  root: {
    nodeId: 1,
    backendNodeId: 11,
    nodeType: 9,
    children: [{ nodeId: 2, backendNodeId: 12, nodeType: 1, localName: 'html', children: [body] }],
  },
};

type Reply = unknown | Error;
/** A web tab's page and its debugger, answering the protocol from `replies` and keeping what was asked of it, in order. */
function fake(replies: Record<string, Reply> = {}, o: { type?: string; attachThrows?: boolean } = {}) {
  const log: string[] = [];
  const sent: { method: string; params: object; session?: string }[] = [];
  const on = new Map<string, Set<(...args: never[]) => void>>();
  const listen = (event: string, f: (...args: never[]) => void) => void (on.get(event) ?? on.set(event, new Set()).get(event)!).add(f);
  const unlisten = (event: string, f: (...args: never[]) => void) => void on.get(event)?.delete(f);
  const emit = (event: string, ...args: unknown[]) => [...(on.get(event) ?? [])].forEach((f) => (f as (...a: unknown[]) => void)(...args));
  const s = { attached: false, destroyed: false, attaches: 0, detaches: 0 };
  const answers: Record<string, Reply> = {
    'DOM.getDocument': tree,
    'CSS.getComputedStyleForNode': {
      computedStyle: [
        { name: 'color', value: 'rgb(255, 255, 255)' },
        { name: 'cursor', value: 'pointer' },
        { name: 'display', value: 'inline-block' },
      ],
    },
    'DOM.getOuterHTML': { outerHTML: '<button class="btn primary" id="pay">Pay now</button>' },
    'DOM.getBoxModel': { model: { border: [41, 171.4375, 129.85, 171.4375, 129.85, 204.4375, 41, 204.4375] } },
    'Page.getLayoutMetrics': { layoutViewport: { clientWidth: 2048 }, cssLayoutViewport: { clientWidth: 1024 } },
    ...replies,
  };
  const host = {};
  const page: PickPage = {
    hostWebContents: host,
    debugger: {
      isAttached: () => s.attached,
      attach() {
        if (o.attachThrows) throw new Error('Another debugger is already attached to this target');
        s.attached = true;
        s.attaches++;
        log.push('attach');
      },
      detach() {
        s.attached = false;
        s.detaches++;
        log.push('detach');
      },
      async sendCommand(method, params = {}, session) {
        sent.push({ method, params, session });
        log.push(session ? `${method}@${session}` : method);
        if (!s.attached) throw new Error('not attached');
        const r = answers[method];
        if (r instanceof Error) throw r;
        return r ?? {};
      },
      on: listen,
      removeListener: unlisten,
    },
    getType: () => o.type ?? 'webview',
    getURL: () => 'http://localhost:5173/cart',
    isDestroyed: () => s.destroyed,
    async capturePage() {
      log.push('capturePage');
      return { isEmpty: () => false, toPNG: () => png };
    },
    on: listen,
    removeListener: unlisten,
  };
  const caller: Caller = { senderFrame: { url: `${ORIGIN}/`, parent: null }, sender: host };
  const listeners = () => [...on.values()].reduce((n, set) => n + set.size, 0);
  return { page, caller, s, log, sent, emit, listeners };
}
type Fake = ReturnType<typeof fake>;

const picker = (f: Fake, limitMs = 5000) => new ElementPicker(ORIGIN, { page: (id) => (id === 2 ? f.page : null), limitMs, settleMs: 0 });
const turn = () => new Promise((resolve) => setImmediate(resolve));
/** Waits until the page has been told to wait for a click. */
async function armed(f: Fake) {
  for (let i = 0; i < 100 && !f.sent.some((c) => c.method === 'Overlay.setInspectMode'); i++) await turn();
  assert.ok(f.sent.some((c) => c.method === 'Overlay.setInspectMode'));
}
const click = (f: Fake, session = '') => f.emit('message', {}, 'Overlay.inspectNodeRequested', { backendNodeId: 17 }, session);
/** Every exit leaves the page as it was found: detached once, and no listener left on it. */
function released(f: Fake) {
  assert.deepEqual([f.s.attached, f.s.attaches, f.s.detaches, f.listeners()], [false, 1, 1, 0]);
}
const failed = (r: PickResult) => (r.ok ? 'ok' : r.code);

test('a pick is refused, with the page untouched, unless the app window asks for one of its own web tab pages', async () => {
  const f = fake();
  const p = picker(f);
  const refused = async (r: Promise<PickResult>, message: RegExp) => {
    const got = await r;
    assert.equal(failed(got), 'bad_request');
    assert.match((got as { message: string }).message, message);
  };
  await refused(p.pick({ ...f.caller, senderFrame: { url: 'http://evil.example/', parent: null } }, 2), /not from the app window/);
  await refused(p.pick({ ...f.caller, senderFrame: { url: `${ORIGIN}/`, parent: {} } }, 2), /not from the app window/);
  await refused(p.pick(f.caller, '2'), /id is a number/);
  await refused(p.pick(f.caller, 2.5), /id is a number/);
  await refused(p.pick(f.caller, 3), /not a page of this window/);
  await refused(p.pick({ ...f.caller, sender: {} }, 2), /not a page of this window/);
  const windowPage = fake({}, { type: 'window' });
  await refused(picker(windowPage).pick(windowPage.caller, 2), /not a web tab page/);
  for (const x of [f, windowPage]) assert.deepEqual([x.s.attaches, x.sent.length, x.listeners()], [0, 0, 0]);
});

test('a click picks: a fixed run of commands, the highlight off before the picture, and the page let go', async () => {
  const f = fake();
  const going = picker(f).pick(f.caller, 2);
  await armed(f);
  assert.deepEqual(f.log, ['attach', 'DOM.enable', 'Overlay.enable', 'Target.setAutoAttach', 'Overlay.setInspectMode']);
  const inspect = f.sent[3]!.params as { mode: string; highlightConfig: object };
  assert.deepEqual([inspect.mode, typeof inspect.highlightConfig], ['searchForNode', 'object']);
  click(f);
  const r = await going;
  assert.deepEqual(f.log.slice(5), [
    'Overlay.setInspectMode',
    'Overlay.hideHighlight',
    'DOM.getDocument',
    'CSS.enable',
    'CSS.getComputedStyleForNode',
    'DOM.getOuterHTML',
    'DOM.getBoxModel',
    'Page.getLayoutMetrics',
    'capturePage',
    'detach',
  ]);
  assert.deepEqual(f.sent[4]!.params, { mode: 'none', highlightConfig: {} });
  assert.deepEqual(f.sent.find((c) => c.method === 'CSS.getComputedStyleForNode')!.params, { nodeId: 7 });
  assert.deepEqual(f.sent.find((c) => c.method === 'DOM.getOuterHTML')!.params, { backendNodeId: 17 });
  assert.deepEqual(r, {
    ok: true,
    png: png.toString('base64'),
    page: 'http://localhost:5173/cart',
    tag: 'button',
    id: 'pay',
    classes: ['btn', 'primary'],
    path: 'button#pay',
    within: [],
    text: 'Pay now',
    styles: { color: 'rgb(255, 255, 255)', display: 'inline-block' },
    html: '<button class="btn primary" id="pay">Pay now</button>',
    box: { x: 82, y: 343, w: 178, h: 66 },
  });
  released(f);
});

test('what the page wrote comes back cut to size, with only the listed styles', async () => {
  const long = 'x'.repeat(50_000);
  const big = {
    ...button,
    attributes: ['class', Array.from({ length: 40 }, (_, i) => `c${i}${long}`).join(' '), 'id', long],
    children: [{ nodeId: 8, backendNodeId: 18, nodeType: 3, nodeValue: long }],
  };
  const f = fake({
    'DOM.getDocument': { root: { ...tree.root, children: [{ ...tree.root.children[0]!, children: [{ ...body, children: [big] }] }] } },
    'DOM.getOuterHTML': { outerHTML: long },
    'CSS.getComputedStyleForNode': {
      computedStyle: [
        { name: 'font-family', value: long },
        { name: 'background-image', value: 'url(x)' },
      ],
    },
  });
  const going = picker(f).pick(f.caller, 2);
  await armed(f);
  click(f);
  const r = await going;
  assert.ok(r.ok);
  const L = PICK_LIMITS;
  assert.deepEqual([r.id.length, r.text.length, r.html.length, r.classes.length], [L.id, L.text, L.html, L.classes]);
  assert.ok(r.classes.every((c) => c.length <= L.className));
  assert.deepEqual(Object.keys(r.styles), ['font-family']);
  assert.equal(r.styles['font-family']!.length, L.style);
  assert.ok(Object.keys(r.styles).every((k) => (PICK_STYLES as readonly string[]).includes(k)));
  assert.deepEqual(Object.keys(r).sort(), ['box', 'classes', 'html', 'id', 'ok', 'page', 'path', 'png', 'styles', 'tag', 'text', 'within']);
  released(f);
});

test('one pick at a time on a page, and the next one can start when it ends', async () => {
  const f = fake();
  const p = picker(f);
  const first = p.pick(f.caller, 2);
  await armed(f);
  assert.equal(failed(await p.pick(f.caller, 2)), 'busy');
  assert.equal(f.s.attaches, 1);
  click(f);
  assert.equal(failed(await first), 'ok');
  const second = p.pick(f.caller, 2);
  await turn();
  assert.deepEqual(p.cancel(f.caller, 2), { ok: true });
  assert.equal(failed(await second), 'cancelled');
  assert.deepEqual([f.s.attached, f.s.attaches, f.s.detaches, f.listeners()], [false, 2, 2, 0]);
});

test('cancel stops a waiting pick, from the app window only', async () => {
  const f = fake();
  const p = picker(f);
  const going = p.pick(f.caller, 2);
  await armed(f);
  assert.equal(p.cancel({ senderFrame: { url: 'http://evil.example/', parent: null } }, 2).ok, false);
  assert.equal(p.cancel(f.caller, 'x').ok, false);
  assert.deepEqual(p.cancel(f.caller, 9), { ok: true }); // no pick on that page: nothing to stop
  assert.equal(f.s.attached, true);
  assert.deepEqual(p.cancel(f.caller, 2), { ok: true });
  assert.equal(failed(await going), 'cancelled');
  assert.ok(!f.log.includes('capturePage'));
  released(f);
});

test('a pick ends, and lets go of the page, when no click comes, the page moves on or closes, or the debugger is taken', async () => {
  const ends: [string, (f: Fake) => void, string, RegExp][] = [
    ['time', () => undefined, 'timeout', /in time/],
    ['Esc in the page', (f) => f.emit('message', {}, 'Overlay.inspectModeCanceled', {}, ''), 'cancelled', /cancelled/],
    ['navigation', (f) => f.emit('did-navigate'), 'unavailable', /page changed/],
    ['a crash', (f) => f.emit('render-process-gone'), 'unavailable', /page closed/],
    ['the developer tools', (f) => f.emit('detach', {}, 'replaced_with_devtools'), 'cancelled', /developer tools took over/],
  ];
  for (const [what, end, code, message] of ends) {
    const f = fake();
    const going = picker(f, what === 'time' ? 30 : 5000).pick(f.caller, 2);
    await armed(f);
    end(f);
    const r = await going;
    assert.equal(failed(r), code, what);
    assert.match((r as { message: string }).message, message, what);
    assert.ok(!f.log.includes('capturePage'), what);
    released(f);
  }
});

test('a page that closes is not detached from: it is gone, and nothing is left listening', async () => {
  const f = fake();
  const going = picker(f).pick(f.caller, 2);
  await armed(f);
  f.s.destroyed = true;
  f.emit('destroyed');
  assert.equal(failed(await going), 'unavailable');
  assert.deepEqual([f.s.detaches, f.listeners()], [0, 0]);
});

test('a page that changes after the click but before the picture is not answered with half an element', async () => {
  const f = fake();
  const going = picker(f).pick(f.caller, 2);
  await armed(f);
  click(f);
  f.emit('did-navigate');
  assert.equal(failed(await going), 'unavailable');
  await turn();
  released(f);
});

test('a frame from another site waits for the click too, and a click in it says it cannot be picked', async () => {
  const f = fake();
  const going = picker(f).pick(f.caller, 2);
  await armed(f);
  f.emit('message', {}, 'Target.attachedToTarget', { sessionId: 'W', targetInfo: { type: 'worker' } }, '');
  f.emit('message', {}, 'Target.attachedToTarget', { sessionId: 'F', targetInfo: { type: 'iframe' } }, '');
  await turn();
  await turn();
  assert.deepEqual(
    f.sent.filter((c) => c.session).map((c) => `${c.method}@${c.session}`),
    ['DOM.enable@F', 'Overlay.enable@F', 'Overlay.setInspectMode@F'],
  );
  click(f, 'F');
  const r = await going;
  assert.equal(failed(r), 'other_site');
  assert.match((r as { message: string }).message, /frame from another site/);
  assert.ok(!f.log.includes('DOM.getDocument') && !f.log.includes('capturePage'));
  assert.equal(f.log.at(-2), 'Overlay.setInspectMode@F'); // its inspect mode is turned off as well
  released(f);
});

test('a debugger that cannot attach, or a command that fails, is an answer and not a throw', async () => {
  const taken = fake({}, { attachThrows: true });
  const r = await picker(taken).pick(taken.caller, 2);
  assert.equal(failed(r), 'unavailable');
  assert.match((r as { message: string }).message, /developer tools/);
  assert.deepEqual([taken.sent.length, taken.s.detaches, taken.listeners()], [0, 0, 0]);

  const broken = fake({ 'DOM.getDocument': new Error('Document is not available') });
  const going = picker(broken).pick(broken.caller, 2);
  await armed(broken);
  click(broken);
  const b = await going;
  assert.equal(failed(b), 'unavailable');
  assert.match((b as { message: string }).message, /could not be read: Document is not available/);
  released(broken);

  const gone = fake({ 'DOM.getDocument': { root: { nodeId: 1, backendNodeId: 11, nodeType: 9, children: [] } } });
  const g = picker(gone).pick(gone.caller, 2);
  await armed(gone);
  click(gone);
  assert.match(((await g) as { message: string }).message, /no longer on the page/);
  released(gone);
});

test('an element with no box is picked without one', async () => {
  const f = fake({ 'DOM.getBoxModel': new Error('Could not compute box model.') });
  const going = picker(f).pick(f.caller, 2);
  await armed(f);
  click(f);
  const r = await going;
  assert.ok(r.ok);
  assert.equal(r.box, null);
  released(f);
});
