import { test } from 'node:test';
import assert from 'node:assert/strict';
import { budgetLeftWords, creditsLine, creditsMonthWords, planUsd, weekBudgets } from '#src/index.ts';

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

test('the top line names usage credits only when some were spent', () => {
  assert.equal(creditsMonthWords({ USD: 12.4 }), '+$12.40 spent on usage credits this month');
  assert.equal(creditsMonthWords({ USD: 0 }), null);
  assert.equal(creditsMonthWords({}), null);
  assert.equal(creditsMonthWords(undefined), null);
});

test("an account's usage-credit line: on with spend, on at $0, no limit, off", () => {
  const on = { on: true, cap: 50, spent: 12.4, currency: 'USD' };
  assert.deepEqual(creditsLine(on), { text: 'Usage credits · $12.40 spent this month · limit $50.00', spent: true });
  assert.deepEqual(creditsLine({ ...on, spent: 0 }), { text: 'Usage credits · $0.00 spent this month · limit $50.00', spent: false });
  assert.equal(creditsLine({ ...on, cap: null })?.text, 'Usage credits · $12.40 spent this month · no limit');
  assert.equal(creditsLine({ ...on, on: false }), null);
  assert.equal(creditsLine(null), null);
});
