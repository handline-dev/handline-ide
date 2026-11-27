import { test } from 'node:test';
import assert from 'node:assert/strict';
import { setup } from '#src/helpers.ts';
import { applySeeds } from '#src/seeds.ts';

test('the town office: the one on hold switches account, the stopped one tries again and the sleeper wakes, each out of its state', async () => {
  const { daemon, client } = setup();
  applySeeds(daemon, 'town');
  await client.start();
  const view = async (agent: string) => (await client.call('agents.list', {})).agents.find((x) => x.key === agent)!;
  assert.deepEqual(
    [(await view('handline.guide')).account_switch?.phase, (await view('handline.guide')).status?.reason],
    ['waiting', 'wall'],
  );
  assert.equal((await view('handline.checker')).status?.reason, 'stalled');
  assert.equal((await view('handline.sleeper')).status?.state, 'off');
  await client.call('agent.switch_now', { agent: 'handline.guide', account: 'alpha', request_id: 'tg-1' });
  await client.call('agent.unstall', { agent: 'handline.checker', request_id: 'tg-2' });
  await client.call('session.start', { agent: 'handline.sleeper', request_id: 'tg-3' });
  for (const a of ['handline.guide', 'handline.checker', 'handline.sleeper']) assert.equal((await view(a)).status, undefined, a);
  assert.equal((await client.call('agent.get', { agent: 'handline.sleeper' })).session_open, true);
  client.close();
});
