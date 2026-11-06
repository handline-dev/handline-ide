import { test } from 'node:test';
import assert from 'node:assert/strict';
import { applyOpenAtLogin, openedAtLogin, workingOf, type LoginItems } from '#src/login.ts';

const items = (got: { openAtLogin: boolean; wasOpenedAtLogin?: boolean }) => {
  const set: boolean[] = [];
  const app: LoginItems = {
    setLoginItemSettings: (s) => (set.push(s.openAtLogin), (got.openAtLogin = s.openAtLogin)),
    getLoginItemSettings: () => got,
  };
  return { app, set };
};

test('Open at login goes to the system both ways, only for the installed app, and only when it changes', () => {
  const r = items({ openAtLogin: false });
  applyOpenAtLogin(r.app, true, true);
  applyOpenAtLogin(r.app, true, true); // already on: nothing more
  applyOpenAtLogin(r.app, true, false);
  assert.deepEqual(r.set, [true, false]);
  const dev = items({ openAtLogin: false });
  applyOpenAtLogin(dev.app, false, true); // an unpackaged run would add Electron itself
  assert.deepEqual(dev.set, []);
});

test('a launch the system made at login keeps to the tray; one the person made opens the window', () => {
  assert.equal(openedAtLogin(items({ openAtLogin: true, wasOpenedAtLogin: true }).app, true), true);
  assert.equal(openedAtLogin(items({ openAtLogin: true, wasOpenedAtLogin: false }).app, true), false);
  assert.equal(openedAtLogin(items({ openAtLogin: true, wasOpenedAtLogin: true }).app, false), false);
});

test('the working count is the agents mid-turn', () => {
  assert.equal(workingOf({ agents: [{ state: 'running' }, { state: 'idle' }, { state: 'running' }, {}] }), 2);
  assert.equal(workingOf({}), 0);
});
