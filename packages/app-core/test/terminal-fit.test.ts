import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fitOf } from '#src/index.ts';

test('the terminal never goes under 80 by 24 and shrinks the font to fit 80 columns on a phone', () => {
  const f = fitOf(374, 400);
  assert.equal(f.cols, 80);
  assert.ok(f.font < 11);
  assert.ok(f.rows >= 24);
  assert.ok(fitOf(1200, 700).cols > 80);
});

test('a short box still gives 24 rows', () => {
  assert.equal(fitOf(374, 100).rows, 24);
  assert.equal(fitOf(1200, 50).rows, 24);
});

test('no width gives under 80 columns, whatever the float floor says', () => {
  for (let w = 1; w <= 2000; w += 0.5) assert.ok(fitOf(w, 400).cols >= 80, `width ${w}`);
});
