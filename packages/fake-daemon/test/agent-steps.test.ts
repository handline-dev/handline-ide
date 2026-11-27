import { test } from 'node:test';
import assert from 'node:assert/strict';
import { setup } from '#src/helpers.ts';
import { applySeeds } from '#src/seeds.ts';

test('an agent’s view carries the steps of the task it holds, and a new count shows on the next list', async () => {
  const { daemon, client } = setup();
  await client.start();
  const steps = async () => (await client.call('agents.list', {})).agents.find((a) => a.key === 'st.agent')!.steps;
  const base = daemon.sessions.get('handline.architect')!;
  daemon.sessions.set('st.agent', {
    ...base,
    state: 'running',
    fx: { ...base.fx, name: 'st.agent', task: { id: 9201, title: 'held' }, view: {} },
  });
  daemon.tasks.seed({ id: 9201, from: 'human', to: 'st.agent', title: 'held', state: 'doing' });
  assert.equal(await steps(), null, 'no count given');
  const at = new Date().toISOString();
  daemon.tasks.find(9201).todo = { done: 3, total: 7, current: 'x', updated_at: at };
  assert.deepEqual(await steps(), { done: 3, total: 7 });
  daemon.tasks.find(9201).todo = { done: 4, total: 7, current: 'y', updated_at: at };
  assert.deepEqual(await steps(), { done: 4, total: 7 }, 'a new count');
  daemon.tasks.find(9201).state = 'done';
  assert.equal(await steps(), null, 'a done task');
  client.close();
});

test('the town seed’s working lead carries the steps of its task', async () => {
  const { daemon, client } = setup();
  applySeeds(daemon, 'town');
  await client.start();
  const lead = (await client.call('agents.list', {})).agents.find((a) => a.task?.id === 9810)!;
  assert.deepEqual(lead.steps, { done: 3, total: 7 });
  client.close();
});
