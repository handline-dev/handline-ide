import { test } from 'node:test';
import assert from 'node:assert/strict';
import { statusLine, trayIcon, trayMenu, type TrayActions, type TrayFollow, type TrayView } from '#src/tray.ts';

const calls: string[] = [];
const actions: TrayActions = {
  open: () => calls.push('open'),
  inbox: () => calls.push('inbox'),
  setOpenAtLogin: (on) => calls.push(`login ${on}`),
  quit: () => calls.push('quit'),
  copy: (t) => calls.push(`copy ${t}`),
};
type State = Parameters<typeof trayMenu>[0];
const view = (over: Partial<TrayView> = {}): TrayView => ({
  unread: 0,
  working: 3,
  openAtLogin: true,
  remote: null,
  details: false,
  ...over,
});
const labels = (s: State, v = view(), a = actions) => trayMenu(s, a, v).map((i) => i.label ?? '-');
const follow = (over: Partial<TrayFollow> = {}): TrayFollow => ({
  sha: 'aaa1111ffff',
  state: 'current',
  next: null,
  reason: '',
  check: () => calls.push('check'),
  restart: () => calls.push('restart'),
  ...over,
});

test('the menu, top to bottom: status, Open, Open at login, Quit; no daemon controls', () => {
  assert.deepEqual(labels('running'), ['Handline · 3 agents working', '-', 'Open Handline', '-', 'Open at login', '-', 'Quit Handline']);
  const items = trayMenu('running', actions, view());
  assert.equal(items[0]!.enabled, false);
  for (const l of ['Open Handline', 'Quit Handline']) items.find((i) => i.label === l)!.click!();
  assert.deepEqual(calls.splice(0), ['open', 'quit']);
});

test('the status line: how many agents work, starting, paired with another device', () => {
  const v = (over: Partial<TrayView> = {}) => ({ working: 1, remote: null, ...over });
  assert.equal(statusLine('running', v()), 'Handline · 1 agent working');
  assert.equal(statusLine('running', v({ working: null })), 'Handline');
  assert.equal(statusLine('starting', v()), 'Handline is starting…');
  assert.equal(statusLine('running', v({ remote: 'mac.ts.net:7443' })), 'Paired with mac.ts.net:7443');
});

test('Open at login is a checkbox that turns the setting the other way', () => {
  const on = trayMenu('running', actions, view()).find((i) => i.label === 'Open at login')!;
  assert.equal(on.type, 'checkbox');
  assert.equal(on.checked, true);
  on.click!();
  trayMenu('running', actions, view({ openAtLogin: false })).find((i) => i.label === 'Open at login')!.click!();
  assert.deepEqual(calls.splice(0), ['login false', 'login true']);
});

test('a stopped daemon or a failed update: "Handline needs attention" first, which opens the app, and the yellow icon', () => {
  for (const [s, v] of [
    ['stopped', view()],
    ['failed', view()],
    ['running', view({ update: { phase: 'failed' } })],
  ] as const) {
    const items = trayMenu(s, actions, v);
    assert.equal(items[0]!.label, 'Handline needs attention');
    items[0]!.click!();
    assert.equal(calls.pop(), 'open');
    assert.equal(trayIcon(s, { ...v, unread: 5 }), 'trouble'); // trouble outranks the rest
    assert.ok(!labels(s, v).some((x) => /daemon|Try again|not updated/i.test(x)));
  }
});

test('paired with another device: this Mac’s daemon being off is no trouble', () => {
  const v = view({ remote: 'mac.ts.net:7443' });
  assert.equal(labels('stopped', v)[0], 'Paired with mac.ts.net:7443');
  assert.equal(trayIcon('stopped', v), 'plain');
});

test('items waiting on the person add Needs you, which opens the Inbox, and a red dot', () => {
  assert.ok(labels('running', view({ unread: 3 })).includes('Needs you · 3'));
  assert.ok(labels('running', view({ unread: 250 })).includes('Needs you · 99+'));
  trayMenu('running', actions, view({ unread: 3 })).find((i) => i.label === 'Needs you · 3')!.click!();
  assert.equal(calls.pop(), 'inbox');
  assert.equal(trayIcon('running', view({ unread: 3 })), 'turn');
});

test('a ready update is a blue dot and no menu line: the app offers the restart', () => {
  assert.equal(trayIcon('running', view({ update: { phase: 'later' } })), 'update');
  assert.equal(trayIcon('running', view({ update: { phase: 'idle' } })), 'plain');
  assert.ok(!labels('running', view({ update: { phase: 'ready' } })).some((x) => /restart|update/i.test(x)));
});

test('Option adds the daemon’s address and build and the commit, which copy on a click; no mode words anywhere', () => {
  const v = view({ details: true, link: { address: '127.0.0.1:7443', version: 'abc1234' }, follow: follow() });
  const l = labels('running', v);
  assert.ok(l.includes('127.0.0.1:7443 · abc1234'));
  assert.ok(l.includes('Handline is on main aaa1111'));
  assert.ok(l.includes('Check for update'));
  const items = trayMenu('running', actions, v);
  items.find((i) => i.label === '127.0.0.1:7443 · abc1234')!.click!();
  items.find((i) => i.label?.startsWith('Handline is on main'))!.click!();
  assert.deepEqual(calls.splice(0), ['copy 127.0.0.1:7443', 'copy aaa1111ffff']);
  assert.ok(!l.some((x) => /mode|connect/i.test(x)));
  assert.ok(!labels('running', view({ follow: follow() })).includes('Check for update'));
});
