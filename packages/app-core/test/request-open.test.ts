import { test } from 'node:test';
import assert from 'node:assert/strict';
import { isPainted, requestClick } from '#src/request-open.ts';

test('a row of another task shows that task and is the one painted', () => {
  assert.deepEqual(requestClick(12, { key: 'turn:a', task: 40 }), { show: 40, painted: 'turn:a', scroll: null });
});

test('a row of the task already shown scrolls to its comment instead of showing it again', () => {
  assert.deepEqual(requestClick(40, { key: 'note:7', task: 40, memo: 7 }), { show: 40, painted: 'note:7', scroll: { memo: 7 } });
});

test('a turn row of the task already shown scrolls to the turn', () => {
  assert.deepEqual(requestClick(40, { key: 'turn:a', task: 40 }).scroll, 'turn');
});

test('only the clicked row is painted, not another row of the same task', () => {
  assert.equal(isPainted('note:7', 'note:7'), true);
  assert.equal(isPainted('note:7', 'decided:40'), false);
  assert.equal(isPainted(null, 'note:7'), false);
});
