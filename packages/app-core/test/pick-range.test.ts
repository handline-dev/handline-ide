import { test } from 'node:test';
import assert from 'node:assert/strict';
import { pickRange } from '#src/pick-range.ts';

const rows = ['a', 'b', 'c', 'd', 'e'];
const of =
  (...on: string[]) =>
  (k: string) =>
    on.includes(k);

test('a forward range from the last clicked row picks every row between', () => {
  assert.deepEqual(pickRange(rows, 'b', 'd', of('b')), { keys: ['b', 'c', 'd'], on: true });
});

test('a backward range runs in drawn order too', () => {
  assert.deepEqual(pickRange(rows, 'd', 'a', of('d')), { keys: ['a', 'b', 'c', 'd'], on: true });
});

test('a range from an unchecked row clears it', () => {
  assert.deepEqual(pickRange(rows, 'b', 'e', of('c', 'd', 'e')), { keys: ['b', 'c', 'd', 'e'], on: false });
});

test('after a filter only the rows still shown are taken', () => {
  assert.deepEqual(pickRange(['a', 'c', 'e'], 'a', 'e', of('a')), { keys: ['a', 'c', 'e'], on: true });
});

test('an anchor filtered out, or under a folded section, makes the click a plain toggle', () => {
  assert.deepEqual(pickRange(['c', 'd', 'e'], 'a', 'e', of('a')), { keys: ['e'], on: true });
  assert.deepEqual(pickRange(['c', 'd', 'e'], 'a', 'e', of('a', 'e')), { keys: ['e'], on: false });
});

test('a range across a section break takes the rows of both sections that are shown', () => {
  // Section one holds a, b; section two holds c, d; the list passes them as drawn.
  assert.deepEqual(pickRange(['a', 'b', 'c', 'd'], 'b', 'c', of('b')), { keys: ['b', 'c'], on: true });
});

test('no anchor yet is a plain toggle', () => {
  assert.deepEqual(pickRange(rows, null, 'c', of()), { keys: ['c'], on: true });
});
