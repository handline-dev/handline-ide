import { test } from 'node:test';
import assert from 'node:assert/strict';
import { PICK_LIMITS, PICK_STYLES } from '@handline/app-core';
import { ElementPicker, NODE_CAP, type Caller, type PickPage, type PickResult } from '#src/pick.ts';

const ORIGIN = 'http://127.0.0.1:5000';
/** A PNG header that says 2078 x 1510, then nothing the tests read. */
const png = Buffer.alloc(32);
png.writeUInt32BE(0x49484452, 12);
png.writeUInt32BE(2078, 16);
png.writeUInt32BE(1510, 20);

const button = { nodeId: 7, backendNodeId: 17, nodeType: 1, localName: 'button', attributes: ['class', 'btn primary', 'id', 'pay'] };
const body = { nodeId: 3, backendNodeId: 13, nodeType: 1, localName: 'body', attributes: [] };
const html = { nodeId: 2, backendNodeId: 12, nodeType: 1, localName: 'html' };
const root = { nodeId: 1, backendNodeId: 11, nodeType: 9 };
/** A parent's id and the children the page sends for it. */
type Sent = [number, object[]];
/** What the page sends when the way to the button is asked for: the children of the document, of html, of body. */
const wayTo = (picked: object, ...beside: object[]): Sent[] => [
  [1, [html]],
  [2, [body]],
  [3, [picked, ...beside]],
];

/** An answer to a command, an error to fail it with, or what makes the answer when the command comes. */
type Reply = unknown | Error | (() => unknown);
/** A web tab's page and its debugger, answering the protocol from `replies` and keeping what was asked of it, in order. Asked for the
 * way to a node, it sends `way` as the page does: each list of children as an event of `from` (the page's own session, unless given). */
function fake(replies: Record<string, Reply> = {}, o: { type?: string; attachThrows?: boolean; way?: Sent[]; from?: string } = {}) {
  const log: string[] = [];
  const sent: { method: string; params: object; session?: string }[] = [];
  const on = new Map<string, Set<(...args: never[]) => void>>();
  const listen = (event: string, f: (...args: never[]) => void) => void (on.get(event) ?? on.set(event, new Set()).get(event)!).add(f);
  const unlisten = (event: string, f: (...args: never[]) => void) => void on.get(event)?.delete(f);
  const emit = (event: string, ...args: unknown[]) => [...(on.get(event) ?? [])].forEach((f) => (f as (...a: unknown[]) => void)(...args));
  const s = { attached: false, destroyed: false, attaches: 0, detaches: 0 };
  const answers: Record<string, Reply> = {
    'Memory.getDOMCounters': { documents: 1, nodes: 1200, jsEventListeners: 0 },
    'DOM.getDocument': () => ({ root: structuredClone(root) }),
    'DOM.pushNodesByBackendIdsToFrontend': () => {
      for (const [parentId, nodes] of structuredClone(o.way ?? wayTo(button)))
        emit('message', {}, 'DOM.setChildNodes', { parentId, nodes }, o.from ?? '');
      return { nodeIds: [7] };
    },
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
        return (typeof r === 'function' ? (r as () => unknown)() : r) ?? {};
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

const picker = (f: Fake, limitMs = 5000, nodeCap?: number) =>
  new ElementPicker(ORIGIN, { page: (id) => (id === 2 ? f.page : null), limitMs, settleMs: 0, nodeCap });
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
    'Memory.getDOMCounters',
    'DOM.getDocument',
    'DOM.pushNodesByBackendIdsToFrontend',
    'CSS.enable',
    'CSS.getComputedStyleForNode',
    'DOM.getOuterHTML',
    'DOM.getBoxModel',
    'Page.getLayoutMetrics',
    'capturePage',
    'detach',
  ]);
  assert.deepEqual(f.sent[4]!.params, { mode: 'none', highlightConfig: {} });
  // Of the page's tree, the document alone is asked for, then the way to the clicked node: never the page whole.
  assert.deepEqual(f.sent.find((c) => c.method === 'DOM.getDocument')!.params, { depth: 0 });
  assert.deepEqual(f.sent.find((c) => c.method === 'DOM.pushNodesByBackendIdsToFrontend')!.params, { backendNodeIds: [17] });
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
  const big = { ...button, attributes: ['class', Array.from({ length: 40 }, (_, i) => `c${i}${long}`).join(' '), 'id', long] };
  const f = fake(
    {
      'DOM.getOuterHTML': { outerHTML: long },
      'CSS.getComputedStyleForNode': {
        computedStyle: [
          { name: 'font-family', value: long },
          { name: 'background-image', value: 'url(x)' },
        ],
      },
    },
    { way: wayTo(big) },
  );
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

test('cancel stops a waiting pick only for the window the page is in, checked as the pick was', async () => {
  const f = fake();
  const p = picker(f);
  const going = p.pick(f.caller, 2);
  await armed(f);
  const refused = (e: Caller, id: unknown, message: RegExp) => {
    const got = p.cancel(e, id);
    assert.equal(got.ok, false);
    assert.match((got as { message: string }).message, message);
  };
  refused({ ...f.caller, senderFrame: { url: 'http://evil.example/', parent: null } }, 2, /not from the app window/);
  refused({ ...f.caller, senderFrame: { url: `${ORIGIN}/`, parent: {} } }, 2, /not from the app window/);
  refused(f.caller, 'x', /id is a number/);
  refused(f.caller, 2.5, /id is a number/);
  refused(f.caller, 9, /not a page of this window/);
  // Another window of the app, on the same origin: the page is not one of its own.
  refused({ ...f.caller, sender: {} }, 2, /not a page of this window/);
  // None of them stopped it.
  assert.equal(await Promise.race([going.then(() => 'ended'), turn().then(() => 'waiting')]), 'waiting');
  assert.equal(f.s.attached, true);
  assert.deepEqual(p.cancel(f.caller, 2), { ok: true });
  assert.equal(failed(await going), 'cancelled');
  assert.ok(!f.log.includes('capturePage'));
  released(f);
  // With no pick waiting there is nothing to stop.
  assert.deepEqual(p.cancel(f.caller, 2), { ok: true });
  const windowPage = fake({}, { type: 'window' });
  assert.equal(picker(windowPage).cancel(windowPage.caller, 2).ok, false);
});

test('a page with more nodes than the cap is refused before anything of it is read', async () => {
  const f = fake({ 'Memory.getDOMCounters': { nodes: 101 } });
  const going = picker(f, 5000, 100).pick(f.caller, 2);
  await armed(f);
  click(f);
  const r = await going;
  assert.equal(failed(r), 'unavailable');
  assert.match((r as { message: string }).message, /too large to pick from: it holds 101 nodes, and picking reads pages of up to 100\./);
  // Counted, collected once in case a page left behind was counted too, counted again: nothing else.
  assert.deepEqual(f.log.slice(7), ['Memory.getDOMCounters', 'HeapProfiler.collectGarbage', 'Memory.getDOMCounters', 'detach']);
  released(f);

  // At the cap it is read, and nothing is collected.
  const at = fake({ 'Memory.getDOMCounters': { nodes: 100 } });
  const fine = picker(at, 5000, 100).pick(at.caller, 2);
  await armed(at);
  click(at);
  assert.equal(failed(await fine), 'ok');
  assert.ok(!at.log.includes('HeapProfiler.collectGarbage'));

  // The app's own cap, with a count in the millions.
  const vast = fake({ 'Memory.getDOMCounters': { nodes: 3_000_000 } });
  const no = picker(vast).pick(vast.caller, 2);
  await armed(vast);
  click(vast);
  const most = NODE_CAP.toLocaleString('en-US');
  assert.ok(((await no) as { message: string }).message.endsWith(`holds 3,000,000 nodes, and picking reads pages of up to ${most}.`));
  assert.ok(!vast.log.includes('DOM.getDocument') && !vast.log.includes('DOM.getOuterHTML') && !vast.log.includes('capturePage'));
});

test('nodes of a page left behind stop counting once they are collected, and the pick goes on', async () => {
  const counts = [500, 60];
  const f = fake({ 'Memory.getDOMCounters': () => ({ nodes: counts.shift() }) });
  const going = picker(f, 5000, 100).pick(f.caller, 2);
  await armed(f);
  click(f);
  assert.equal(failed(await going), 'ok');
  assert.deepEqual(f.log.slice(7, 11), [
    'Memory.getDOMCounters',
    'HeapProfiler.collectGarbage',
    'Memory.getDOMCounters',
    'DOM.getDocument',
  ]);
  released(f);
});

test('a page that does not say how large it is, or grows past the cap while it is read, is refused', async () => {
  const silent = fake({ 'Memory.getDOMCounters': {} });
  const s = picker(silent).pick(silent.caller, 2);
  await armed(silent);
  click(silent);
  const quiet = await s;
  assert.equal(failed(quiet), 'unavailable');
  assert.match((quiet as { message: string }).message, /did not say how large/);
  assert.ok(!silent.log.includes('DOM.getDocument') && !silent.log.includes('capturePage'));
  released(silent);

  // Counted small, then the way down brings more children than the cap: no more is taken, and no element comes of it.
  const beside = Array.from({ length: 50 }, (_, i) => ({ nodeId: 100 + i, backendNodeId: 200 + i, nodeType: 1, localName: 'span' }));
  const grown = fake({ 'Memory.getDOMCounters': { nodes: 5 } }, { way: wayTo(button, ...beside) });
  const g = picker(grown, 5000, 10).pick(grown.caller, 2);
  await armed(grown);
  click(grown);
  const r = await g;
  assert.equal(failed(r), 'unavailable');
  assert.match((r as { message: string }).message, /too large to pick from: picking reads pages of up to 10\./);
  assert.ok(!grown.log.includes('DOM.getOuterHTML') && !grown.log.includes('capturePage'));
  released(grown);
});

test("only the page's own session says where its nodes are", async () => {
  const f = fake({}, { from: 'F' });
  const going = picker(f).pick(f.caller, 2);
  await armed(f);
  click(f);
  assert.match(((await going) as { message: string }).message, /no longer on the page/);
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

  const gone = fake({ 'DOM.pushNodesByBackendIdsToFrontend': { nodeIds: [0] } });
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
