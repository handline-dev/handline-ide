import { test } from 'node:test';
import assert from 'node:assert/strict';
import { turnCount } from '#src/turn-count.ts';

test('turnCount adds every cluster and the stopped service', () => {
  assert.equal(turnCount([21, 3], true), 25);
  assert.equal(turnCount([21, 3], false), 24);
  assert.equal(turnCount([], true), 1);
  assert.equal(turnCount([], false), 0);
});
