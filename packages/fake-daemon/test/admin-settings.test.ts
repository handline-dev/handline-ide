import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';

const KEYS = [
  'park_timeout_s',
  'handoff_note',
  'ignore_cache_expiry',
  'dev_mode',
  'bg_notice_s',
  'bg_grace_s',
  'bg_check_s',
  'move_background_wait_s',
  'notice_digest_s',
  'token_prices',
  'input_hold_idle_s',
  'handoff_prompt',
  'push_prompt_asked',
  'push_task_done',
  'push_stall',
  'push_account_out',
  'push_only_away',
  'push_sound',
  'account_split',
  'block_screen_capture',
  'compact_at_task_end',
  'compact_now_stops_turn',
  'no_silence_reminder',
  'screencapture_via_handline',
  'agent_placement',
  'review_flow',
  'landed_deadline_s',
  'pace_target_pct',
  'pace_burst_pct',
  'host_active_cap',
  'load_per_core_pct',
  'ram_free_min_pct',
  'disk_free_min_gb',
  'worktree_remove_on_sleep',
  'agents_memory_max_gb',
  'node_battery_switch',
  'node_build_auto',
  'node_reclaim_s',
  'restart_reclaimed',
  'cluster_name',
  'port_range',
  'network_addresses',
  'keep_awake',
  'reap_orphans',
  'default_model_claude',
  'default_effort_claude',
  'default_model_codex',
  'default_effort_codex',
];

test('settings.get serves every SettingsValues key, a load limit reads back, and null restores the default', async () => {
  const { client } = setup();
  await client.start();
  const first = await client.call('settings.get', {});
  assert.deepEqual(Object.keys(first.values).sort(), [...KEYS].sort());
  assert.deepEqual(
    [first.values.host_active_cap, first.values.load_per_core_pct, first.values.ram_free_min_pct, first.values.disk_free_min_gb],
    [0, 0, 0, 0],
  );
  assert.equal(first.timings.bg_grace_s, 300);
  await client.call('settings.set', { changes: { load_per_core_pct: 200, push_stall: false }, request_id: 'l1-a' });
  const set = await client.call('settings.get', {});
  assert.deepEqual([set.values.load_per_core_pct, set.values.push_stall], [200, false]);
  await client.call('settings.set', { changes: { load_per_core_pct: null }, request_id: 'l1-b' });
  assert.equal((await client.call('settings.get', {})).values.load_per_core_pct, 0);
  // how long a gate waits for a pull request to land: a week, until it is set
  assert.equal(first.values.landed_deadline_s, 604800);
  await client.call('settings.set', { changes: { landed_deadline_s: 172800 }, request_id: 'l1-f' });
  assert.equal((await client.call('settings.get', {})).values.landed_deadline_s, 172800);
  await client.call('settings.set', { changes: { landed_deadline_s: null }, request_id: 'l1-g' });
  assert.equal((await client.call('settings.get', {})).values.landed_deadline_s, 604800);
  assert.deepEqual([first.values.keep_awake], ['off']);
  await assert.rejects(
    client.call('settings.set', { changes: { keep_awake: 'always' as never }, request_id: 'l1-d' }),
    (e: RpcError) => e.code === 'bad_request',
  );
  await client.call('settings.set', { changes: { keep_awake: 'working' }, request_id: 'l1-e' });
  const awake = (await client.call('settings.get', {})).values;
  assert.deepEqual([awake.keep_awake], ['working']);
  client.close();
});

test('settings.get shows the built-in handoff prompt and who sets their own; a short prompt is refused and line breaks are joined', async () => {
  const { client } = setup();
  await client.start();
  const got = await client.call('settings.get', {});
  assert.equal(got.handoff_prompt_default, got.values.handoff_prompt);
  assert.deepEqual(got.handoff_prompt_own, []);
  await assert.rejects(
    client.call('settings.set', { changes: { handoff_prompt: ' too short ' }, request_id: 'h1-a' }),
    (e: RpcError) => e.code === 'bad_request',
  );
  await client.call('settings.set', { changes: { handoff_prompt: 'Write {path}:\n what is next,\r\n and why.' }, request_id: 'h1-b' });
  assert.equal((await client.call('settings.get', {})).values.handoff_prompt, 'Write {path}: what is next, and why.');
  client.close();
});

test('daemon.status keep_awake follows the setting: held under on, since the start; not under off', async () => {
  const { client } = setup();
  await client.start();
  assert.deepEqual((await client.call('daemon.status', {})).keep_awake, { mode: 'off', held: false });
  await client.call('settings.set', { changes: { keep_awake: 'on' }, request_id: 'k1' });
  const st = await client.call('daemon.status', {});
  assert.deepEqual(st.keep_awake, { mode: 'on', held: true, since: st.started_at });
});

test('daemon.status keep_awake under working is held only while a session runs', async () => {
  const { client, daemon } = setup();
  await client.start();
  for (const s of daemon.sessions.values()) s.state = 'idle';
  await client.call('settings.set', { changes: { keep_awake: 'working' }, request_id: 'k2' });
  assert.deepEqual((await client.call('daemon.status', {})).keep_awake, { mode: 'working', held: false });
  daemon.sessions.values().next().value!.state = 'running';
  assert.equal((await client.call('daemon.status', {})).keep_awake?.held, true);
});

test('lease.get gives each shared run as a pool: its at_once, who holds a turn and who waits', async () => {
  const { client } = setup();
  await client.start();
  const kinds = Object.fromEntries((await client.call('lease.get', {})).kinds.map((k) => [k.kind, k]));
  const build = kinds['mac:ios-build'];
  assert.deepEqual([build?.max, build?.held.length, build?.queue.map((w) => w.agent)], [1, 1, ['handline.term-build']]);
  assert.equal(kinds['w1:check']?.max, 2);
  client.close();
});

test('restart_reclaimed is off until set, globally and for one agent over it', async () => {
  const { client } = setup();
  await client.start();
  assert.equal((await client.call('settings.get', {})).values.restart_reclaimed, false);
  await client.call('settings.set', { changes: { restart_reclaimed: true }, request_id: 'r1-a' });
  assert.equal((await client.call('settings.get', {})).values.restart_reclaimed, true);
  const agent = (await client.call('agents.list', {})).agents[0]!.key;
  await client.call('settings.set', { agent, changes: { restart_reclaimed: false }, request_id: 'r1-b' });
  assert.deepEqual((await client.call('settings.get', { agent })).overrides, { restart_reclaimed: false });
  client.close();
});
