import { test } from 'node:test';
import assert from 'node:assert/strict';
import { setup } from '#src/helpers.ts';

test("stats.outlook's budget carries each case of extra usage beside the plan money, and the days it was spent", async () => {
  const { client } = setup();
  await client.start();
  const out = await client.call('stats.outlook', { range: '7d' });
  const extra = Object.fromEntries((out.budget?.accounts ?? []).map((a) => [a.account, a.extra]));
  assert.deepEqual(extra.alpha, { on: true, cap: 50, spent: 14.5, currency: 'USD' });
  assert.equal(extra.beta?.on, true);
  assert.equal(extra.beta?.spent, 0);
  assert.equal(extra.kappa?.on, false);
  assert.deepEqual(out.budget?.extra_spent, { USD: 14.5 });
  const days = out.plan_cost_per_day ?? [];
  assert.equal(
    days.reduce((s, d) => s + (d.extra.USD ?? 0), 0),
    14.5,
    "the days' extra add up to alpha's month",
  );
  client.close();
});
