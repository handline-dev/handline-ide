import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { after, test } from 'node:test';
import { OLD_LABEL, removeOldService } from '#src/old-service.ts';

const homes: string[] = [];
after(() => homes.forEach((h) => fs.rmSync(h, { recursive: true, force: true })));

const rig = () => {
  const userHome = fs.mkdtempSync(path.join(os.tmpdir(), 'old-service-'));
  homes.push(userHome);
  const calls: string[][] = [];
  return { userHome, calls, deps: { userHome, uid: 501, launchctl: async (a: string[]) => void calls.push(a) } };
};

test('the old launchd job is unloaded and its file removed', async () => {
  const r = rig();
  const plist = path.join(r.userHome, 'Library', 'LaunchAgents', `${OLD_LABEL}.plist`);
  fs.mkdirSync(path.dirname(plist), { recursive: true });
  fs.writeFileSync(plist, '<plist/>');
  assert.equal(await removeOldService(r.deps), true);
  assert.deepEqual(r.calls, [['bootout', `gui/501/${OLD_LABEL}`]]);
  assert.equal(fs.existsSync(plist), false);
});

test('with no old job nothing is asked of launchd', async () => {
  const r = rig();
  assert.equal(await removeOldService(r.deps), false);
  assert.deepEqual(r.calls, []);
});
