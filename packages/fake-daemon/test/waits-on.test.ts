import { test } from 'node:test';
import assert from 'node:assert/strict';
import { setup } from '#src/helpers.ts';
import { FOUNDER } from '#src/founder.ts';

test('waits_on names the first task an idle agent’s held tasks all wait on, as the daemon reads it', async () => {
  const { daemon, client } = setup();
  await client.start();
  const waits = async () => (await client.call('agents.list', {})).agents.find((a) => a.key === 'wt.agent')!.waits_on;
  const base = daemon.sessions.get('handline.architect')!;
  daemon.sessions.set('wt.agent', {
    ...base,
    state: 'idle',
    fx: { ...base.fx, name: 'wt.agent', task: { id: 9101, title: 'held' }, view: {} },
  });
  daemon.tasks.seed({ id: 9101, from: 'human', to: 'wt.agent', title: 'held', state: 'doing' });
  daemon.tasks.seed({ id: 9102, from: 'human', to: 'other', title: 'b', state: 'queued' });
  daemon.tasks.seed({ id: 9103, from: 'human', to: 'other', title: 'c', state: 'queued' });
  assert.equal(await waits(), null, 'a held task that waits on nothing');
  daemon.tasks.find(9101).blockers = [
    { task: 9102, reason: 'x' },
    { task: 9103, reason: 'x' },
  ];
  assert.deepEqual(await waits(), { task: 9102, holder: 'other', more: 1 });
  daemon.tasks.find(9102).state = 'draft';
  daemon.tasks.find(9102).acting = 'other';
  assert.deepEqual(await waits(), { task: 9102, holder: FOUNDER, more: 1 }, 'a draft with no turn waits for the founder to run it');
  daemon.tasks.find(9102).state = 'done';
  daemon.tasks.find(9103).state = 'done';
  daemon.tasks.find(9101).acting = FOUNDER;
  assert.deepEqual(await waits(), { task: 9101, holder: FOUNDER, more: 0 }, "the founder's turn");
  daemon.sessions.get('wt.agent')!.state = 'running';
  assert.equal(await waits(), null, 'working');
});
