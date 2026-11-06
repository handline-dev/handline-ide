import { test } from 'node:test';
import assert from 'node:assert/strict';
import { second, setup } from '#src/helpers.ts';

const ARCH = 'handline.architect'; // local-ref-ok: a fixture agent name

test('a read marker counts the comments above it, never moves back, and drops a read task out of requests.pending', async () => {
  const s = setup({ client: 'boss' });
  s.daemon.tasks.verbs = true;
  await s.client.start();
  const agent = second(s.daemon, s.net, { role: 'agent', client: ARCH });
  await agent.start();
  const task = (await s.client.call('task.new', { owner: 'human', title: 'decide this', body: 'b', request_id: 'r1' })).task as number;
  await agent.call('task.comment.add', { task, text: 'first', request_id: 'r2' });
  await agent.call('task.comment.add', { task, text: 'second', request_id: 'r3' });
  const t = await s.client.call('task.get', { task });
  assert.deepEqual([t.unread, t.first_unread], [2, 1]);
  const before = await s.client.call('requests.pending', {});
  assert.deepEqual(
    [before.comments.map((r) => [r.task, r.kind, r.unread, r.first_unread]), before.counts.comments, before.counts.needs_you],
    [[[task, 'comment', 2, 1]], 1, 1],
  );
  await s.client.call('task.seen', { task, upto: 1, request_id: 'r4' });
  assert.deepEqual([(await s.client.call('task.get', { task })).unread, (await s.client.call('task.get', { task })).first_unread], [1, 2]);
  await s.client.call('task.seen', { task, upto: 0, request_id: 'r5' });
  assert.equal((await s.client.call('task.get', { task })).unread, 1, 'a lower number changes nothing');
  await s.client.call('task.seen', { task, request_id: 'r6' });
  assert.deepEqual(
    [(await s.client.call('task.get', { task })).unread, (await s.client.call('requests.pending', {})).counts.comments],
    [0, 0],
  );
  assert.equal((await agent.call('task.get', { task })).unread, 0, 'an agent sees none');
  agent.close();
  s.client.close();
});

test('requests.pending holds an answer-needed item until it is answered, and counts drafts', async () => {
  const s = setup({ client: 'boss' });
  s.daemon.tasks.verbs = true;
  await s.client.start();
  const pending = await s.client.call('requests.pending', {});
  assert.ok(pending.answer.length > 0, 'the fixtures hold a permission ask and a decision');
  assert.ok(pending.answer.every((r) => ['allow', 'decide', 'ask'].includes(r.kind)));
  assert.equal(pending.counts.answer, pending.answer.length);
  const draft = (await s.client.call('task.new', { owner: ARCH, title: 'a draft', body: 'b', request_id: 'd1' })).task as number;
  await s.client.call('task.draft', { tasks: [draft], request_id: 'd2' });
  const counts = (await s.client.call('requests.pending', {})).counts;
  assert.equal((await s.client.call('task.get', { task: draft })).state, 'draft');
  assert.equal(counts.drafts, (await s.client.call('tasks.list', { state: 'draft' })).items.length);
  s.client.close();
});

test('tasks.list serves open and draft whole, and closed by cursor with done and discarded together', async () => {
  const s = setup({ client: 'boss' });
  s.daemon.tasks.verbs = true;
  await s.client.start();
  const open = await s.client.call('tasks.list', { state: 'open', limit: 1 });
  assert.ok(open.items.length > 1 && !open.more, 'every open row, whatever the limit');
  assert.ok(open.items.every((t) => t.state === 'queued' || t.state === 'doing'));
  const a = (await s.client.call('task.new', { owner: ARCH, title: 'a', body: 'b', request_id: 'c1' })).task as number;
  const b = (await s.client.call('task.new', { owner: ARCH, title: 'b', body: 'b', request_id: 'c2' })).task as number;
  await s.client.call('task.done', { task: a, evidence: 'x', request_id: 'c3' });
  await s.client.call('task.discard', { task: b, reason: 'no', request_id: 'c4' });
  const page = await s.client.call('tasks.list', { state: 'closed', order: 'closed', limit: 1 });
  assert.ok(page.more && page.next);
  const rest = await s.client.call('tasks.list', { state: 'closed', order: 'closed', cursor: page.next, limit: 200 });
  const states = new Set([...page.items, ...rest.items].map((t) => t.state));
  assert.ok(states.has('done') && states.has('discarded'));
  await assert.rejects(s.client.call('tasks.list', { state: 'closed' }), /order/);
  s.client.close();
});
