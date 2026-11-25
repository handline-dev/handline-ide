import { test } from 'node:test';
import assert from 'node:assert/strict';
import { setup } from '#src/helpers.ts';

test("stats.outlook's by_account gives each account's 5h and week history, and its resets oldest first", async () => {
  const { client } = setup();
  await client.start();
  const out = await client.call('stats.outlook', { range: '3d' });
  const now = Date.parse(out.now);
  const accounts = out.by_account ?? [];
  assert.deepEqual(
    accounts.map((a) => a.account),
    ['alpha', 'beta'],
  );
  for (const a of accounts) {
    // At the daemon's step: 96 points over twice the 3 days, one every 1.5 hours.
    assert.equal(a.past.length, 3 * 16 + 1, `${a.account}: a point every 1.5 hours`);
    for (const p of a.past) {
      assert.ok(p.five_left !== null && p.five_left >= 0 && p.five_left <= 100, `${a.account}: a 5h reading at ${p.at}`);
      assert.ok(p.week_left !== null && p.week_left >= 0 && p.week_left <= 100, `${a.account}: a week reading at ${p.at}`);
    }
    const times = a.resets.map((r) => Date.parse(r.at));
    assert.deepEqual(
      times,
      [...times].sort((x, y) => x - y),
      `${a.account}: resets oldest first`,
    );
    const fives = a.resets.filter((r) => r.window === '5h');
    assert.ok(fives.length >= 14 && fives.every((r) => Date.parse(r.at) <= now), `${a.account}: the past 3 days' 5h resets`);
  }
  const lowest = (name: string) => Math.min(...accounts.find((a) => a.account === name)!.past.map((p) => p.five_left!));
  assert.ok(lowest('alpha') < lowest('beta'), 'alpha spends more of its 5h window than beta');
  client.close();
});
