import { test } from 'node:test';
import assert from 'node:assert/strict';
import { addPin, DEFAULT_NAV, movePin, navOf, togglePin } from '#src/index.ts';

test('missing prefs read as the defaults', () => {
  assert.deepEqual(navOf(undefined), DEFAULT_NAV);
  assert.deepEqual(navOf({ nav: { ...DEFAULT_NAV, layout: 'flat' } }).layout, 'flat');
});

test('pins toggle and move', () => {
  assert.deepEqual(togglePin(['a'], 'b'), ['a', 'b']);
  assert.deepEqual(togglePin(['a', 'b'], 'a'), ['b']);
  assert.deepEqual(movePin(['a', 'b', 'c'], 'c', 0), ['c', 'a', 'b']);
  assert.deepEqual(movePin(['a', 'b'], 'x', 0), ['a', 'b']);
  assert.deepEqual(addPin(['a'], 'b'), ['a', 'b']);
  assert.deepEqual(addPin(['a', 'b'], 'a'), ['a', 'b']);
});
