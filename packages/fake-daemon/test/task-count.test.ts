import { test } from 'node:test';
import assert from 'node:assert/strict';
import { setup } from '#src/helpers.ts';

test("task_count leaves out a task on someone else's turn or behind an open blocker, as the daemon counts it", async () => {
  const { daemon, client } = setup();
  await client.start();
  const count = async () => (await client.call('agents.list', {})).agents.find((a) => a.key === 'cnt.agent')!.task_count;
  const base = daemon.sessions.get('handline.architect')!;
  daemon.sessions.set('cnt.agent', { ...base, fx: { ...base.fx, name: 'cnt.agent', task: { id: 9001, title: 'held' }, view: {} } });
  daemon.tasks.seed({ id: 9001, from: 'human', to: 'cnt.agent', title: 'held', state: 'doing', acting: 'human' });
  daemon.tasks.seed({ id: 9002, from: 'human', to: 'cnt.agent', title: 'next', state: 'queued' });
  daemon.tasks.seed({ id: 9003, from: 'human', to: 'other', title: 'blocker', state: 'queued' });
  assert.equal(await count(), 2, "the held task counts on the founder's turn");
  daemon.tasks.find(9002).acting = 'human';
  assert.equal(await count(), 1, "the founder's turn");
  daemon.tasks.find(9002).acting = 'cnt.agent';
  assert.equal(await count(), 2, 'the turn back');
  daemon.tasks.find(9002).blockers = [{ task: 9003, reason: 'waits' }];
  assert.equal(await count(), 1, 'blocked');
  daemon.tasks.find(9003).state = 'done';
  assert.equal(await count(), 2, 'the blocker done');
});
