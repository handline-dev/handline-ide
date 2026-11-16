import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spentText } from '#src/index.ts';

test('the numbers beside a line give the share of the limit and the cost in one money form', () => {
  assert.equal(spentText({ weekPct: 12.4, cost: 3.4 }), '12% · $3.40');
  assert.equal(spentText({ weekPct: 0.55, cost: 120 }), '0.6% · $120.00');
  assert.equal(spentText({ weekPct: null, cost: 0.5 }), '$0.50');
});
