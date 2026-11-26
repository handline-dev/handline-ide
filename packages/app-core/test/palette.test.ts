import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { SearchHit } from '@handline/client';
import { chatOfHit, goMatches, hitGroups, isPaletteChord, snippetParts, stepRow, taskNumberOf, withRecent } from '#src/palette.ts';

const key = (over: Partial<Parameters<typeof isPaletteChord>[0]>) => ({
  code: 'KeyK',
  key: 'k',
  metaKey: true,
  ctrlKey: false,
  altKey: false,
  shiftKey: false,
  ...over,
});

test('the palette opens on Cmd-K on a Mac, by the key pressed, and on nothing else', () => {
  assert.equal(isPaletteChord(key({}), true), true);
  assert.equal(isPaletteChord(key({ key: 'ㅏ' }), true), true); // a Korean input types another letter on the same key
  assert.equal(isPaletteChord(key({ code: 'KeyJ', key: 'j' }), true), false);
  for (const mod of ['ctrlKey', 'altKey', 'shiftKey'] as const) assert.equal(isPaletteChord(key({ [mod]: true }), true), false, mod);
  // Off a Mac Ctrl-K is the terminal's kill-line, and Cmd is not the command key.
  assert.equal(isPaletteChord(key({ metaKey: false, ctrlKey: true }), false), false);
  assert.equal(isPaletteChord(key({}), false), false);
});

test('a task number is a bare number, or one after T or #', () => {
  assert.deepEqual(['123', ' T123 ', 't7', '#9001'].map(taskNumberOf), [123, 123, 7, 9001]); // local-ref-ok: what a person types
  assert.deepEqual(['', '0', 'T0', 'T12a', '12 3', 'TT1', '#', '-4', '1234567890'].map(taskNumberOf), Array(9).fill(null));
});

test('places match by their start, then a word, then anywhere, and by their other words last', () => {
  const items = [
    { label: 'web-settings', words: 'shop' },
    { label: 'Settings' },
    { label: 'reset' },
    { label: 'lead', words: 'settlers' },
    { label: 'Tasks' },
  ];
  assert.deepEqual(
    goMatches(items, ' SET ').map((i) => i.label),
    ['Settings', 'web-settings', 'reset', 'lead'],
  );
  assert.deepEqual(goMatches(items, 'se', 2).length, 2);
  assert.deepEqual(goMatches(items, '  '), []);
  assert.deepEqual(goMatches(items, 'zzz'), []);
});

test('a recent thing goes first, once, and the list is cut to its size', () => {
  assert.deepEqual(withRecent(['a', 'b', 'c'], 'b'), ['b', 'a', 'c']);
  assert.deepEqual(withRecent(['a', 'b', 'c'], 'd', 3), ['d', 'a', 'b']);
});

test('hits are grouped by kind in the order the kinds came', () => {
  const hit = (kind: SearchHit['kind'], snippet: string): SearchHit => ({ kind, at: '2026-01-01T00:00:00Z', snippet });
  const groups = hitGroups([hit('task', 'a'), hit('task', 'b'), hit('message', 'c')]);
  assert.deepEqual(
    groups.map((g) => [g.title, g.hits.map((h) => h.snippet)]),
    [
      ['Tasks', ['a', 'b']],
      ['Messages', ['c']],
    ],
  );
  assert.deepEqual(hitGroups([]), []);
});

test('a snippet is cut at its first match, whatever the case of its ASCII letters', () => {
  assert.deepEqual(snippetParts('Fix the Checkout flow', ' checkOUT '), [
    { text: 'Fix the ', hit: false },
    { text: 'Checkout', hit: true },
    { text: ' flow', hit: false },
  ]);
  assert.deepEqual(snippetParts('가나다 plan', '가나'), [
    { text: '가나', hit: true },
    { text: '다 plan', hit: false },
  ]);
  assert.deepEqual(snippetParts('no match here', 'zzz'), [{ text: 'no match here', hit: false }]);
  assert.deepEqual(snippetParts('anything', ''), [{ text: 'anything', hit: false }]);
});

test('a message hit is in the chat that draws it: the sender for talk between agents, else the one the daemon names', () => {
  assert.equal(chatOfHit({ agent: 'a2', from: 'a1' }), 'a1');
  assert.equal(chatOfHit({ agent: 'a2', from: 'p1' }), 'a2');
  assert.equal(chatOfHit({ agent: 'a1', from: 'a1' }), 'a1');
  assert.equal(chatOfHit({ agent: 'a2', from: 'human' }), 'a2');
  assert.equal(chatOfHit({ agent: 'a2', from: 'schedule' }), 'a2'); // typed by the daemon: it has no chat of its own
  assert.equal(chatOfHit({}), undefined);
});

test('arrows walk the rows round the ends, Home and End go to them', () => {
  assert.deepEqual(
    ['ArrowDown', 'ArrowUp', 'Home', 'End', 'x'].map((k) => stepRow(0, 3, k)),
    [1, 2, 0, 2, 0],
  );
  assert.equal(stepRow(2, 3, 'ArrowDown'), 0);
  assert.equal(stepRow(5, 0, 'ArrowDown'), 0);
});
