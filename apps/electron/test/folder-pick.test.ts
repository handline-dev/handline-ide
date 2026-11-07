import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Sender } from '#src/bridge.ts';
import { FolderPick } from '#src/folder-pick.ts';

const ORIGIN = 'http://127.0.0.1:5000';
const top: Sender = { senderFrame: { url: `${ORIGIN}/`, parent: null } };
const frame: Sender = { senderFrame: { url: `${ORIGIN}/`, parent: {} } };
const pick = (r: { canceled: boolean; filePaths: string[] }) => {
  let asked = 0;
  const p = new FolderPick(ORIGIN, { choose: async () => (asked++, r) });
  return { p, asked: () => asked };
};

test('pickFolder answers the chosen folder, null on cancel, and asks nothing for a call not from the app window', async () => {
  assert.deepEqual(await pick({ canceled: false, filePaths: ['/Users/u/code/app'] }).p.pick(top), { ok: true, path: '/Users/u/code/app' });
  assert.deepEqual(await pick({ canceled: true, filePaths: ['/Users/u/code/app'] }).p.pick(top), { ok: true, path: null });
  assert.deepEqual(await pick({ canceled: false, filePaths: [] }).p.pick(top), { ok: true, path: null });
  const framed = pick({ canceled: false, filePaths: ['/x'] });
  const r = await framed.p.pick(frame);
  assert.equal(r.ok === false && r.code, 'bad_request');
  assert.equal(framed.asked(), 0);
});
