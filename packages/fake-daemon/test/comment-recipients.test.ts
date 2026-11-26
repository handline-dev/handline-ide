import { test } from 'node:test';
import assert from 'node:assert/strict';
import { second, setup } from '#src/helpers.ts';

const ARCH = 'handline.architect';

// The same cases as contract/fixtures/task_comments.json (task.comment.recipients), for the fake's own rule.
test('a comment that names agents is told to them only, one that names no one to the holder; the founder’s answer to a turn tells the holder below too', async () => {
  const s = setup({ client: 'boss' });
  s.daemon.tasks.verbs = true; // task.comment.add is a verb of the daemon's plan
  await s.client.start();
  const owner = second(s.daemon, s.net, { client: ARCH });
  await owner.start();
  const task = (await s.client.call('task.new', { owner: ARCH, title: 'a task', body: 'build it', request_id: 'r1' })).task as number;
  const who = async (c: typeof owner, text: string) =>
    ((await c.call('task.comment.recipients', { task, text })).recipients as { agent: string; why: string }[]).map(
      (r) => `${r.agent}:${r.why}`,
    );
  assert.deepEqual(await who(s.client, `@${ARCH} look`), [`${ARCH}:asked`]);
  assert.deepEqual(await who(s.client, 'no one named'), [`${ARCH}:assignee`]);
  assert.deepEqual(await who(owner, 'no one named'), []);
  const other = second(s.daemon, s.net, { client: 'handline.auditor' });
  await other.start();
  assert.deepEqual(await who(other, 'no one named'), [`${ARCH}:assignee`]); // another agent's comment still reaches the holder
  other.close();
  const held = (await owner.call('task.new', { owner: 'handline.auditor', title: 'held', body: 'build it', request_id: 'r3' }))
    .task as number;
  const founders = async (id: number) =>
    (
      (await s.client.call('task.comment.recipients', { task: id, text: 'no one named' })).recipients as { agent: string; why: string }[]
    ).map((r) => `${r.agent}:${r.why}`);
  assert.deepEqual(await founders(held), ['handline.auditor:assignee']); // the holder answers; the agent it reports to is not told
  const theirs = (await owner.call('task.new', { owner: 'human', title: 'theirs', body: 'decide', request_id: 'r4' })).task as number;
  assert.deepEqual(await founders(theirs), [`${ARCH}:owner`]); // no agent holds it: the agent it reports to is
  const third = second(s.daemon, s.net, { client: 'handline.ui' });
  await third.start();
  const named = async (c: typeof owner, text: string, now = false) =>
    ((await c.call('task.comment.recipients', { task: held, text, now })).recipients as { agent: string; why: string }[]).map(
      (r) => `${r.agent}:${r.why}`,
    );
  // a comment that names agents reaches only them: not the holder, nor, sent now, who the task answers to
  assert.deepEqual(await named(third, '@handline.architect please look', true), [`${ARCH}:asked`]);
  assert.deepEqual(await named(owner, `@${ARCH} and @handline.term-build`), ['handline.term-build:asked']); // never the writer
  // the holder asks the founder; the founder's answer that names someone passes the turn back, so the holder hears it too
  await s.daemon.tasks.memo({ task: held, text: 'which one?', turn: 'human' }, 'add', 'handline.auditor', true);
  assert.deepEqual(await named(s.client, '@handline.term-build see this'), ['handline.auditor:assignee', 'handline.term-build:asked']);
  third.close();
  await s.client.call('task.comment.add', { task, text: `@${ARCH} look`, request_id: 'r2' });
  assert.equal((await s.client.call('task.get', { task })).memos.length, 1);
  owner.close();
  s.client.close();
});
