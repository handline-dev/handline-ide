import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type Event, type RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';
import { DEVICE_CODE, GOOD_LOGIN_CODE } from '#src/accounts.ts';

type Login = { login_id: string; state: string; url: string };

test('a login answers its id and URL, the code ends it done and the account is logged in', async () => {
  const { client, daemon } = setup();
  await client.start();
  const seen: Event[] = [];
  client.onEvent((e) => e.topic === 'accounts' && seen.push(e));
  await client.subscribe(['accounts']);
  const r = (await client.call('account.login', { account: 'gamma', request_id: 'l1' })) as unknown as Login;
  assert.equal(r.state, 'waiting');
  assert.match(r.url, /^https:/);
  assert.equal(
    ((await client.call('accounts.list', {})).accounts as { name: string; login: { state: string } | null }[]).find(
      (a) => a.name === 'gamma',
    )?.login?.state,
    'waiting',
  );
  await client.call('account.login_code', { login_id: r.login_id, code: GOOD_LOGIN_CODE, request_id: 'l2' });
  const states = seen.map((e) => (e.data as { login: { state: string } }).login.state);
  assert.deepEqual(states, ['waiting', 'code_sent', 'done']);
  assert.equal(daemon.accounts.list().accounts.find((a) => a.name === 'gamma')?.login_state, 'ok');
  client.close();
});

test('a wrong code ends the login failed with a reason; a second login is refused while one waits; cancel is idempotent', async () => {
  const { client, daemon } = setup();
  await client.start();
  const first = (await client.call('account.login', { account: 'gamma', request_id: 'm1' })) as unknown as Login;
  const again = await client.call('account.login', { account: 'gamma', request_id: 'm2' }).catch((e: RpcError) => e);
  assert.equal((again as RpcError).data?.gate, 'state');
  await client.call('account.login_code', { login_id: first.login_id, code: 'nope', request_id: 'm3' });
  const row = () => daemon.accounts.list().accounts.find((a) => a.name === 'gamma')!;
  assert.deepEqual([row().login?.state, row().login_state], ['failed', 'needs_login']);
  assert.equal(typeof row().login?.reason, 'string');
  const late = await client
    .call('account.login_code', { login_id: first.login_id, code: GOOD_LOGIN_CODE, request_id: 'm4' })
    .catch((e: RpcError) => e);
  assert.equal((late as RpcError).data?.gate, 'state');
  const second = (await client.call('account.login', { account: 'gamma', request_id: 'm5' })) as unknown as Login;
  await client.call('account.login_cancel', { login_id: second.login_id, request_id: 'm6' });
  await client.call('account.login_cancel', { login_id: second.login_id, request_id: 'm7' });
  assert.equal(row().login?.state, 'cancelled');
  client.close();
});

test('account.add registers an account that needs a login, and refuses a used name, a bad name and an unknown vendor', async () => {
  const { client, daemon } = setup();
  await client.start();
  const r = await client.call('account.add', { account: 'delta', request_id: 'a1' });
  assert.equal(r.account, 'delta');
  assert.equal(daemon.accounts.list().accounts.find((a) => a.name === 'delta')?.login_state, 'needs_login');
  const used = await client.call('account.add', { account: 'delta', request_id: 'a2' }).catch((e: RpcError) => e);
  assert.equal((used as RpcError).data?.gate, 'state');
  for (const [params, field] of [
    [{ account: 'a/b' }, 'account'],
    [{ account: '.x' }, 'account'],
    [{ account: 'epsilon', vendor: 'nosuch' }, 'vendor'],
  ] as const) {
    const e = (await client.call('account.add', { ...params, request_id: `b-${field}` }).catch((x: RpcError) => x)) as RpcError;
    assert.deepEqual([e.code, e.data?.field], ['bad_request', field]);
  }
  assert.equal(
    daemon.accounts.list().accounts.some((a) => a.name === 'epsilon'),
    false,
  );
  client.close();
});

test('a login code is kept in no request record: the same request_id is judged again, not replayed', async () => {
  const { client } = setup();
  await client.start();
  const login = (await client.call('account.login', { account: 'gamma', request_id: 'k1' })) as unknown as Login;
  const first = await client.call('account.login_code', { login_id: login.login_id, code: GOOD_LOGIN_CODE, request_id: 'k2' });
  assert.equal(first.replayed, undefined);
  const again = (await client
    .call('account.login_code', { login_id: login.login_id, code: GOOD_LOGIN_CODE, request_id: 'k2' })
    .catch((e: RpcError) => e)) as RpcError;
  assert.equal(again.data?.gate, 'state');
  client.close();
});

test('a codex login answers a device code, takes no code back, and ends done on its own', async () => {
  const { client, daemon } = setup();
  await client.start();
  daemon.accounts.add({ name: 'cx', vendor: 'codex', login_state: 'needs_login' });
  daemon.accounts.deviceLoginMs = 10;
  const r = (await client.call('account.login', { account: 'cx', request_id: 'c1' })) as unknown as Login & { device_code: string };
  assert.deepEqual([r.state, r.device_code], ['waiting', DEVICE_CODE]);
  const code = await client.call('account.login_code', { login_id: r.login_id, code: 'x', request_id: 'c2' }).catch((e: RpcError) => e);
  assert.equal((code as RpcError).data?.gate, 'state');
  await new Promise((done) => setTimeout(done, 40));
  const row = daemon.accounts.list().accounts.find((a) => a.name === 'cx')!;
  assert.deepEqual([row.login?.state, row.login_state], ['done', 'ok']);
  const claude = (await client.call('account.login', { account: 'gamma', request_id: 'c3' })) as Record<string, unknown>;
  assert.equal(claude.device_code, undefined, 'a claude login has no device code');
  client.close();
});

test('daemon.status lists every vendor with whether it is installed and its fix', async () => {
  const { client } = setup();
  await client.start();
  const vendors = (await client.call('daemon.status', {})).vendors as { vendor: string; installed: boolean; fix: string }[];
  assert.deepEqual(
    vendors.map((v) => [v.vendor, v.installed]),
    [
      ['claude', true],
      ['codex', false],
    ],
  );
  client.close();
});

test('a codex device code is kept in no request record, and a login of a removed account never ends done', async () => {
  const { client, daemon } = setup();
  await client.start();
  daemon.accounts.add({ name: 'cx', vendor: 'codex', login_state: 'needs_login' });
  daemon.accounts.deviceLoginMs = 10;
  await client.call('account.login', { account: 'cx', request_id: 'd1' });
  const again = await client.call('account.login', { account: 'cx', request_id: 'd1' });
  assert.deepEqual([again.replayed, again.device_code], [true, undefined], 'a retry gets the reply without the code');
  const seen: Event[] = [];
  client.onEvent((e) => e.topic === 'accounts' && seen.push(e));
  await client.subscribe(['accounts']);
  await client.call('account.remove', { account: 'cx', confirm: 'cx', request_id: 'd2' });
  await new Promise((done) => setTimeout(done, 40));
  assert.equal(
    daemon.accounts.list().accounts.find((a) => a.name === 'cx'),
    undefined,
  );
  assert.deepEqual(seen, [], 'nothing is said of an account that is gone');
  client.close();
});

test('codex is a missing tool only while it is not installed and an account uses it', async () => {
  const { client, daemon } = setup();
  await client.start();
  const missing = async () => ((await client.call('daemon.status', {})).missing_tools ?? []) as { tool: string }[];
  assert.deepEqual(await missing(), []);
  daemon.accounts.add({ name: 'cx', vendor: 'codex', login_state: 'needs_login' });
  assert.deepEqual(
    (await missing()).map((m) => m.tool),
    ['codex'],
  );
  daemon.admin.vendors.find((v) => v.vendor === 'codex')!.installed = true;
  assert.deepEqual(await missing(), []);
  client.close();
});
