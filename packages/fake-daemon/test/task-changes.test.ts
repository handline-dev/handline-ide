import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';

test("task.changes serves a seeded task's two commits, the whole task's files and one commit's, and names a bad or unknown one", async () => {
  const s = setup({ client: 'boss' });
  await s.client.start();
  const all = await s.client.call('task.changes', { task: 1688 });
  assert.deepEqual(
    all.commits.map((c) => [c.landed, c.branch, c.files, c.added]),
    [
      [true, 'main', 1, 2],
      [false, 'main', 2, 2],
    ],
  );
  assert.deepEqual(
    all.files.map((f) => [f.path, f.added, f.deleted]),
    [
      ['apps/web/src/term/Pane.tsx', 3, 1],
      ['apps/web/src/term/scroll.ts', 1, 0],
    ],
  );
  assert.equal(all.commit, null);
  assert.match(all.diff, /\+const source = bytes\(\);[\s\S]*\+export const keepScroll[\s\S]*scroll\.ts/);
  const one = await s.client.call('task.changes', { task: 1688, commit: 'f0e1d2c3' });
  assert.equal(one.commit, 'f0e1d2c3b4a5968778695a4b3c2d1e0f98765432');
  assert.deepEqual(
    one.files.map((f) => f.added),
    [1, 1],
  );
  assert.equal(one.commits.length, 2);
  const none = await s.client.call('task.changes', { task: 1690 });
  assert.deepEqual([none.commits, none.files, none.diff], [[], [], '']);
  const errs = await Promise.all(
    [{ task: 'T1' }, { task: 1688, commit: 'zz' }, { task: 999999 }, { task: 1690, commit: 'f0e1d2c3' }].map(
      async (p) => ((await s.client.call('task.changes', p as never).catch((e) => e)) as RpcError).data?.code,
    ),
  );
  assert.deepEqual(errs, ['bad_request', 'bad_request', 'not_found', 'not_found']);
  s.client.close();
});
