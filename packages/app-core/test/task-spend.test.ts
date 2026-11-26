import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spendShares, taskCostText, usdText } from '#src/index.ts';

test("a task's cost says it is at API prices, so it does not read as a bill", () => {
  assert.equal(taskCostText({ usd: 6.84 }), '$6.84 at API prices');
  assert.equal(taskCostText({ usd: 120 }), '$120.00 at API prices');
});

test('an amount under half a cent is not shown as zero', () => {
  assert.equal(usdText(0.004), 'under $0.01');
  assert.equal(usdText(0.005), '$0.01');
  assert.equal(usdText(0), '$0.00');
});

test('the breakdown is largest first, by name among equals, each with its share of the whole', () => {
  assert.deepEqual(spendShares({ 'grp.two': 1, 'grp.one': 3, 'grp.b': 1 }), [
    { key: 'grp.one', usd: 3, share: 0.6 },
    { key: 'grp.b', usd: 1, share: 0.2 },
    { key: 'grp.two', usd: 1, share: 0.2 },
  ]);
  assert.deepEqual(spendShares({}), []);
  assert.deepEqual(spendShares({ a: 0 }), [{ key: 'a', usd: 0, share: 0 }]);
});
