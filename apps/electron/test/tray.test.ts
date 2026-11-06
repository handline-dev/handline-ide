import { test } from 'node:test';
import assert from 'node:assert/strict';
import { statusLine, trayIcon, trayMenu, type TrayActions, type TrayFollow, type TrayView } from '#src/tray.ts';

const calls: string[] = [];
const actions: TrayActions = {
  open: () => calls.push('open'),
  inbox: () => calls.push('inbox'),
  startDaemon: () => calls.push('start'),
  restartDaemon: () => calls.push('restart-daemon'),
  stopDaemon: () => calls.push('stop-daemon'),
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
const labels = (s: State, v = view(), reason = '', a = actions) => trayMenu(s, reason, a, v).map((i) => i.label ?? '-');
const follow = (over: Partial<TrayFollow> = {}): TrayFollow => ({
  sha: 'aaa1111ffff',
  state: 'current',
  next: null,
  reason: '',
  check: () => calls.push('check'),
  restart: () => calls.push('restart'),
  ...over,
});

test('the menu, top to bottom: status, Open, Open at login, Restart and Stop daemon, Quit', () => {
  assert.deepEqual(labels('running'), [
    'Daemon running · 3 agents working',
    '-',
    'Open Handline',
    '-',
    'Open at login',
    '-',
    'Restart daemon',
    'Stop daemon…',
    '-',
    'Quit Handline',
  ]);
  const items = trayMenu('running', '', actions, view());
  assert.equal(items[0]!.enabled, false);
  for (const l of ['Open Handline', 'Restart daemon', 'Stop daemon…', 'Quit Handline']) items.find((i) => i.label === l)!.click!();
  assert.deepEqual(calls.splice(0), ['open', 'restart-daemon', 'stop-daemon', 'quit']);
});

test('the status line: running with how many agents work, starting, stopped with why, paired with another device', () => {
  assert.equal(statusLine('running', '', { working: 1, remote: null }), 'Daemon running · 1 agent working');
  assert.equal(statusLine('running', '', { working: null, remote: null }), 'Daemon running');
  assert.equal(statusLine('starting', '', { working: 0, remote: null }), 'Starting daemon…');
  assert.equal(statusLine('stopped', '', { working: 0, remote: null }), 'Daemon stopped');
  assert.equal(statusLine('failed', 'claude was not found', { working: 0, remote: null }), 'Daemon stopped: claude was not found');
  assert.equal(statusLine('running', '', { working: 2, remote: 'mac.ts.net:7443' }), 'Paired with mac.ts.net:7443');
});

test('Open at login is a checkbox that turns the setting the other way', () => {
  const on = trayMenu('running', '', actions, view()).find((i) => i.label === 'Open at login')!;
  assert.equal(on.type, 'checkbox');
  assert.equal(on.checked, true);
  on.click!();
  trayMenu('running', '', actions, view({ openAtLogin: false })).find((i) => i.label === 'Open at login')!.click!();
  assert.deepEqual(calls.splice(0), ['login false', 'login true']);
});

test('Restart and Stop daemon only while it runs; a stopped or failed daemon offers Start and shows the yellow icon', () => {
  for (const s of ['stopped', 'failed', 'starting'] as const)
    assert.ok(!labels(s).includes('Stop daemon…') && !labels(s).includes('Restart daemon'));
  assert.ok(labels('stopped').includes('Start daemon'));
  assert.ok(labels('failed', view(), 'it did not start').includes('Start daemon'));
  assert.ok(!labels('starting').includes('Start daemon'));
  trayMenu('failed', '', actions, view()).find((i) => i.label === 'Start daemon')!.click!();
  assert.equal(calls.pop(), 'start');
  assert.equal(trayIcon('stopped', view({ unread: 5 })), 'trouble'); // trouble outranks the rest
});

test('paired with another device: no Start, Restart or Stop, and no yellow icon for this Mac', () => {
  const l = labels('stopped', view({ remote: 'mac.ts.net:7443' }));
  assert.ok(!l.some((x) => /Start daemon|Restart daemon|Stop daemon/.test(x)));
  assert.equal(trayIcon('stopped', view({ remote: 'mac.ts.net:7443' })), 'plain');
});

test('items waiting on the person add Needs you, which opens the Inbox, and a red dot', () => {
  assert.ok(labels('running', view({ unread: 3 })).includes('Needs you · 3'));
  assert.ok(labels('running', view({ unread: 250 })).includes('Needs you · 99+'));
  trayMenu('running', '', actions, view({ unread: 3 })).find((i) => i.label === 'Needs you · 3')!.click!();
  assert.equal(calls.pop(), 'inbox');
  assert.equal(trayIcon('running', view({ unread: 3 })), 'turn');
});

test('the update flow: updating, ready to restart (a blue dot), not updated', () => {
  const upd = (phase: 'idle' | 'preparing' | 'ready' | 'later' | 'failed', reason = '') =>
    view({ update: { phase, reason, restart: () => calls.push('restart'), retry: () => calls.push('retry') } });
  assert.ok(labels('running', upd('preparing')).includes('Updating Handline…'));
  const ready = trayMenu('running', '', actions, upd('ready'));
  assert.ok(ready.some((i) => i.label === 'Handline is ready to restart'));
  ready.find((i) => i.label === 'Restart now')!.click!();
  assert.equal(calls.pop(), 'restart');
  assert.ok(labels('running', upd('failed', 'vite failed')).includes('Handline was not updated: vite failed'));
  assert.equal(trayIcon('running', upd('later')), 'update');
  assert.equal(trayIcon('running', upd('idle')), 'plain');
});

test('Option adds the daemon’s address and build and the commit, which copy on a click; no mode words anywhere', () => {
  const v = view({ details: true, link: { address: '127.0.0.1:7443', version: 'abc1234' }, follow: follow() });
  const l = labels('running', v);
  assert.ok(l.includes('127.0.0.1:7443 · abc1234'));
  assert.ok(l.includes('Handline is on main aaa1111'));
  assert.ok(l.includes('Check for update'));
  const items = trayMenu('running', '', actions, v);
  items.find((i) => i.label === '127.0.0.1:7443 · abc1234')!.click!();
  items.find((i) => i.label?.startsWith('Handline is on main'))!.click!();
  assert.deepEqual(calls.splice(0), ['copy 127.0.0.1:7443', 'copy aaa1111ffff']);
  assert.ok(!l.some((x) => /mode|connect/i.test(x)));
  assert.ok(!labels('running', view({ follow: follow() })).includes('Check for update'));
});
