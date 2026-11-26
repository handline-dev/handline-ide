import { test } from 'node:test';
import assert from 'node:assert/strict';
import { FakeDaemon } from '#src/daemon.ts';
import { ALL, applySeeds, BY_NAME, SEEDS } from '#src/seeds.ts';
import { setup } from '#src/helpers.ts';

test('an unknown seed is refused with the known names', () => {
  assert.throws(() => applySeeds(new FakeDaemon(), 'nope'), /known: inbox-terminal/);
});

test('inbox-terminal and proposals come through the ordinary methods', async () => {
  const { client, daemon } = setup();
  applySeeds(daemon, 'inbox-terminal,proposals');
  await client.start();
  const inbox = await client.call('inbox.list', {});
  assert.ok(inbox.items.some((i) => i.fix === 'open_terminal' && i.agent));
  const { proposals } = await client.call('proposal.list', {});
  assert.deepEqual(
    proposals.map((p) => p.kind),
    ['add', 'settings', 'retire', 'run', 'sandbox'],
  );
  assert.equal(proposals[0]!.asked?.team, 'handline');
  assert.equal(proposals[1]!.asked?.sets?.model, 'opus');
  client.close();
});

test('chat-actions puts the OK and No buttons on a message', async () => {
  const { client, daemon } = setup();
  applySeeds(daemon, 'chat-actions');
  await client.start();
  const { messages } = await client.call('chat.messages', { agent: 'handline.architect', scope: 'all' });
  const keys = messages
    .flatMap((m) => m.actions ?? [])
    .map((a) => a.key)
    .sort();
  assert.deepEqual(keys, ['no', 'ok']);
  client.close();
});

test('tasks-200 seeds 200 tasks, and all switches on every seed but those asked for by name', () => {
  const d = new FakeDaemon();
  d.tasks.verbs = true;
  assert.deepEqual(applySeeds(d, 'all'), ALL);
  assert.ok(BY_NAME.every((n) => !ALL.includes(n) && n in SEEDS));
  assert.equal(d.tasks.find(6000).id, 6000);
  assert.equal(d.tasks.find(6199).id, 6199);
});

test('review: a task tagged review whose first comment names two pictures the daemon serves', () => {
  const daemon = new FakeDaemon();
  applySeeds(daemon, 'review');
  const { memos } = daemon.tasks.get({ task: 9301 });
  const images = memos[0]!.parts!.flatMap((p) => ('image' in p ? [p.image.id] : []));
  assert.equal(images.length, 2);
  assert.equal(daemon.uploads.get({ id: images[0] }).mime, 'image/png');
});

test('my-requests: the founder has five open requests, one waiting on them and one blocked', () => {
  const d = new FakeDaemon();
  applySeeds(d, 'my-requests');
  const items = d.requests.rows;
  assert.equal(items.filter((r) => r.kind === 'task').length, 5);
  assert.equal(items.filter((r) => r.status === 'waiting_on_you').length, 1);
  assert.equal(items.filter((r) => r.blocked === true).length, 1);
});

test('chat-actions: the open task the founder owns has a body, a time box, two comments and a running stretch', async () => {
  const { client, daemon } = setup();
  applySeeds(daemon, 'chat-actions');
  await client.start();
  const t = await client.call('task.get', { task: 6902 });
  assert.equal(t.state, 'doing');
  assert.equal(t.memos.length, 2);
  assert.ok(t.body.includes('three steps'));
  client.close();
});

test('story seeds a story with five drafts, their plans in every state a node shows', async () => {
  const { client, daemon } = setup();
  applySeeds(daemon, 'story');
  await client.start();
  const story = await client.call('task.get', { task: 9400 });
  assert.deepEqual(
    story.subtasks.map((s) => [s.id, s.state, s.plan_state]),
    [
      [9401, 'draft', 'ok'],
      [9402, 'draft', 'written'],
      [9403, 'draft', 'written'],
      [9404, 'draft', 'writing'],
      [9405, 'draft', 'written'],
    ],
  );
  const changed = await client.call('task.get', { task: 9402 });
  assert.equal(changed.plan?.versions.length, 2);
  const asked = await client.call('task.get', { task: 9405 });
  assert.equal(asked.turns?.at(-1)?.agent, 'human');
  assert.equal((await client.call('task.get', { task: 9403 })).blockers[0]?.task, 9402);
  await client.call('task.plan_ok', { task: 9402, request_id: 'ok-9402' });
  assert.equal((await client.call('task.get', { task: 9402 })).plan_state, 'ok');
  client.close();
});

test('story-review seeds a sub-task Ready for you with pictures and a video, and one working after a Redo; OK and Redo answer it', async () => {
  const { client, daemon } = setup();
  applySeeds(daemon, 'story-review');
  await client.start();
  const story = await client.call('task.get', { task: 9410 });
  assert.deepEqual(
    story.subtasks.map((s) => [s.id, s.review?.ready, s.review?.redo]),
    [
      [9411, true, 0],
      [9412, false, 1],
    ],
  );
  const ready = await client.call('task.get', { task: 9411 });
  assert.equal(ready.turns?.at(-1)?.agent, 'p1');
  const files = ready.memos.find((m) => m.id === ready.review?.memo)?.files ?? [];
  const video = files.find((f) => f.kind === 'video');
  assert.deepEqual(
    files.map((f) => f.kind),
    ['image', 'image', 'image', 'video'],
  );
  assert.equal(video?.poster, files[2]?.id);
  const code = await client.call('task.changes', { task: 9411 });
  assert.ok(code.diff.includes('unread.ts') && !code.diff.includes('Worktrees.tsx') && code.commits.length === 1, code.diff); // its commit, not what is checked out
  const got = await client.call('attachment.get', { id: video!.id });
  assert.equal(Buffer.from(got.data, 'base64').subarray(4, 8).toString(), 'ftyp');

  await assert.rejects(client.call('task.ok', { task: 9412, request_id: 'ok-9412' }), /not Ready/);
  await assert.rejects(client.call('task.redo', { task: 9411, note: ' ', request_id: 'redo-0' }), /note/);
  await client.call('task.comment.add', { task: 9411, text: 'Why the yellow?', request_id: 'ask-9411' });
  const asked = await client.call('task.get', { task: 9411 });
  assert.deepEqual([asked.review?.ready, asked.turns], [true, ready.turns]); // a question to the assignee: still Ready
  await assert.rejects(client.call('task.turn.pop', { task: 9411, outcome: 'pass', request_id: 'pop-9411' }), /OK, merge or Redo/);
  await client.call('task.redo', { task: 9411, note: 'Make the badge round.', request_id: 'redo-9411' });
  const back = await client.call('task.get', { task: 9411 });
  assert.deepEqual([back.review?.ready, back.review?.redo, back.turns?.length ?? 0], [false, 1, 0]);

  daemon.tasks.find(9412).review = { ...daemon.tasks.find(9412).review!, ready: true };
  await client.call('task.ok', { task: 9412, request_id: 'ok-9412b' });
  const merged = await client.call('task.get', { task: 9412 });
  assert.deepEqual([merged.to, merged.review?.ok, merged.review?.ready], ['handline.architect', true, false]); // local-ref-ok
  client.close();
});
