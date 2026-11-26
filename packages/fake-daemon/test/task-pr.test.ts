import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { applySeeds } from '#src/seeds.ts';
import { setup } from '#src/helpers.ts';

test('task.pr is null for a task that names no pull request, and names a bad or unknown task', async () => {
  const s = setup({ client: 'boss' });
  await s.client.start();
  assert.equal(await s.client.call('task.pr', { task: 1688 }), null);
  const errs = await Promise.all(
    [{ task: 'T1' }, { task: 999999 }].map(
      async (p) => ((await s.client.call('task.pr', p as never).catch((e) => e)) as RpcError).data?.code,
    ),
  );
  assert.deepEqual(errs, ['bad_request', 'not_found']);
  s.client.close();
});

test('pull-requests: an open pull request with failed checks, a merged one, one gh cannot read, and the branches that show them', async () => {
  const s = setup({ client: 'boss' });
  applySeeds(s.daemon, 'pull-requests');
  await s.client.start();
  const open = (await s.client.call('task.pr', { task: 1688 }))!;
  assert.deepEqual(
    [open.number, open.repo, open.state, open.review, open.failing, open.waiting, open.passing, open.unavailable],
    [901, 'handline-dev/handline', 'open', 'changes_requested', ['build', 'lint'], 1, 5, null],
  );
  const merged = (await s.client.call('task.pr', { task: 1650 }))!;
  assert.deepEqual([merged.state, merged.merge_sha?.slice(0, 7), merged.failing], ['merged', '984c657', []]);
  const unread = (await s.client.call('task.pr', { task: 1690 }))!;
  assert.deepEqual(
    [unread.state, unread.unavailable?.reason, unread.url],
    [null, 'gh_logged_out', 'https://github.com/handline-dev/handline/pull/905'],
  );
  const task = await s.client.call('task.get', { task: 1688 });
  assert.deepEqual([task.pr, task.pr_repo], [901, 'handline-dev/handline']);
  assert.equal('pull' in task, false);
  const list = await s.client.call('worktrees.list', {});
  assert.equal(list.prs_unavailable, null);
  assert.deepEqual(
    list.worktrees.filter((w) => w.pr).map((w) => [w.owner, w.pr?.number, w.pr?.verdict, w.pr?.failing]),
    [
      ['handline.term-build', 901, 'checks_failing', 2],
      ['handline.architect', 880, 'merged', undefined],
    ],
  );
  s.client.close();
});

test('worktrees.list says why no branch shows a pull request when the daemon says so', async () => {
  const s = setup({ client: 'boss' });
  s.daemon.admin.worktrees.prsUnavailable = { reason: 'gh_missing', text: 'the gh CLI is not installed' };
  await s.client.start();
  assert.deepEqual((await s.client.call('worktrees.list', {})).prs_unavailable, {
    reason: 'gh_missing',
    text: 'the gh CLI is not installed',
  });
  s.client.close();
});
