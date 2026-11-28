import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';

const AUD = 'handline.auditor';

test('agent.get reads session_mode as following its workspace, agent.set takes chat and terminal, and refuses any other mode', async () => {
  const { client } = setup();
  await client.start();
  const mode = async () => (await client.call('agent.get', { agent: AUD })).settings?.session_mode;
  assert.equal(await mode(), '');
  assert.equal((await client.call('agent.get', { agent: AUD })).settings?.session_mode_in_force, 'terminal');
  await client.call('agent.set', { agent: AUD, values: { session_mode: 'chat' }, request_id: 'm1' });
  assert.equal(await mode(), 'chat');
  await client.call('agent.set', { agent: AUD, values: { session_mode: 'terminal' }, request_id: 'm2' });
  assert.equal(await mode(), 'terminal');
  await assert.rejects(
    client.call('agent.set', { agent: AUD, values: { session_mode: 'headless' as 'chat' }, request_id: 'm3' }),
    (e: RpcError) => e.code === 'bad_request',
  );
  client.close();
});

test('workspace.set takes session_mode alone and lists it, refuses any other mode, and an agent whose own mode is "" follows it', async () => {
  const { client } = setup();
  await client.start();
  const ws = async () => (await client.call('workspaces.list', {})).workspaces.find((w) => w.name === 'handline');
  const inForce = async () => (await client.call('agent.get', { agent: AUD })).settings?.session_mode_in_force;
  assert.equal((await ws())?.session_mode, 'terminal');
  await client.call('workspace.set', { workspace: 'handline', session_mode: 'chat', request_id: 'w1' });
  assert.equal((await ws())?.session_mode, 'chat');
  assert.equal(await inForce(), 'chat');
  await client.call('agent.set', { agent: AUD, values: { session_mode: 'terminal' }, request_id: 'w2' });
  assert.equal(await inForce(), 'terminal');
  await assert.rejects(
    client.call('workspace.set', { workspace: 'handline', session_mode: 'headless' as 'chat', request_id: 'w3' }),
    (e: RpcError) => e.code === 'bad_request' && e.data?.field === 'session_mode',
  );
  assert.equal((await ws())?.session_mode, 'chat');
  client.close();
});

test('agent.open_terminal moves a Chat agent between turns into its terminal, starts one with no session there, and knows no stranger', async () => {
  const { client } = setup();
  await client.start();
  const get = () => client.call('agent.get', { agent: AUD });
  assert.equal((await get()).structured, true);
  const moved = await client.call('agent.open_terminal', { agent: AUD, request_id: 'o1' });
  assert.match(moved.said ?? '', /opened in its terminal/);
  const d = await get();
  assert.deepEqual([d.structured ?? false, d.terminal !== null, d.session_open], [false, true, true]);
  assert.match((await client.call('agent.open_terminal', { agent: AUD, request_id: 'o2' })).said ?? '', /already/);
  await client.call('session.stop', { agent: AUD, mode: 'close', request_id: 'o3' });
  await client.call('session.wake', { agent: AUD, request_id: 'o3w' });
  assert.equal((await get()).structured, true, 'the next session follows session_mode again');
  await client.call('session.stop', { agent: AUD, mode: 'close', request_id: 'o3s' });
  assert.match((await client.call('agent.open_terminal', { agent: AUD, request_id: 'o4' })).said ?? '', /started in its terminal/);
  assert.deepEqual([(await get()).session_open, (await get()).terminal !== null], [true, true]);
  await assert.rejects(
    client.call('agent.open_terminal', { agent: 'handline.nobody', request_id: 'o5' }),
    (e: RpcError) => e.code === 'not_found',
  );
  client.close();
});

test('agent.open_terminal mid-turn moves the Chat agent into its terminal when the turn ends', async () => {
  const { client, daemon } = setup();
  await client.start();
  const s = daemon.sessions.get(AUD)!;
  daemon.script.setState(s, 'running');
  assert.match((await client.call('agent.open_terminal', { agent: AUD, request_id: 't1' })).said ?? '', /when its turn ends/);
  assert.equal((await client.call('agent.get', { agent: AUD })).structured, true);
  daemon.script.setState(s, 'idle');
  const d = await client.call('agent.get', { agent: AUD });
  assert.deepEqual([d.structured ?? false, d.terminal !== null], [false, true]);
  client.close();
});

test('agents.list and agent.get say how the open session runs now: chat, terminal after Open terminal, null once closed', async () => {
  const { client } = setup();
  await client.start();
  const now = async () => {
    const row = (await client.call('agents.list', {})).agents.find((a) => a.key === AUD);
    return [row?.session_mode_now, (await client.call('agent.get', { agent: AUD })).session_mode_now];
  };
  assert.deepEqual(await now(), ['chat', 'chat']);
  await client.call('agent.open_terminal', { agent: AUD, request_id: 'n1' });
  assert.deepEqual(await now(), ['terminal', 'terminal']);
  await client.call('session.stop', { agent: AUD, mode: 'close', request_id: 'n2' });
  assert.deepEqual(await now(), [null, null]);
  client.close();
});
