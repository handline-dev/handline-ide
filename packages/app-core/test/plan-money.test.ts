import { test } from 'node:test';
import assert from 'node:assert/strict';
import { budgetLeftWords, planUsd, weekBudgets } from '#src/index.ts';

test('the top line says the money left of the weekly budgets', () => {
  assert.equal(budgetLeftWords({ week_usd: 46.153, left_usd: 31.2 }), '$31.20 of $46.15 left this week');
  assert.equal(budgetLeftWords({ week_usd: 0, left_usd: 0 }), null);
  assert.equal(budgetLeftWords({ week_usd: 10, left_usd: null }), null);
  assert.equal(budgetLeftWords(undefined), null);
});

test("an agent's plan money is its week share of its account's weekly budget", () => {
  const budgets = weekBudgets({ accounts: [{ account: 'alpha', week_usd: 46.15 }] as never });
  assert.equal(planUsd(10, budgets.get('alpha')), 4.615);
  assert.equal(planUsd(null, budgets.get('alpha')), 0);
  assert.equal(planUsd(10, budgets.get('beta')), 0);
});
