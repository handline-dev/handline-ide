import assert from 'node:assert/strict';
import { test } from 'node:test';
import { cacheRewriteUsd, inputPrice } from '#src/prices.ts';

test('a model is priced by the first pattern it contains, else as sonnet', () => {
  assert.equal(inputPrice('claude-opus-5-5'), 4);
  assert.equal(inputPrice('opus'), 5);
  assert.equal(inputPrice('Haiku'), 1);
  assert.equal(inputPrice('gpt-x'), 3);
});

test('rewriting the context is 2x input per million tokens', () => {
  assert.equal(cacheRewriteUsd(200_000, 'opus'), 2);
});
