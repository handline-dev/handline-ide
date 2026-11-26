import { test } from 'node:test';
import assert from 'node:assert/strict';
import { needsYouOut } from '#src/needs-you.ts';

const view = { top: 100, bottom: 400 };

test('a mark in sight, even in part, counts for neither edge', () => {
  assert.deepEqual(
    needsYouOut(
      [
        { keys: ['a'], top: 120, bottom: 150 },
        { keys: ['b'], top: 380, bottom: 420 },
      ],
      view,
    ),
    { above: [], below: [] },
  );
});

test('marks above the view go to the top edge and below it to the bottom, in list order', () => {
  const out = needsYouOut(
    [
      { keys: ['a'], top: 0, bottom: 40 },
      { keys: ['b'], top: 450, bottom: 480 },
      { keys: ['c', 'd'], top: 500, bottom: 520 }, // a folded workspace's header: its agents
    ],
    view,
  );
  assert.deepEqual(out, { above: ['a'], below: ['b', 'c', 'd'] });
});

test('an agent marked twice is named once, and not at all when one of its rows is in sight', () => {
  assert.deepEqual(
    needsYouOut(
      [
        { keys: ['a'], top: 120, bottom: 150 },
        { keys: ['a'], top: 600, bottom: 630 },
      ],
      view,
    ).below,
    [],
  );
  assert.deepEqual(
    needsYouOut(
      [
        { keys: ['a'], top: 450, bottom: 480 },
        { keys: ['a'], top: 600, bottom: 630 },
      ],
      view,
    ).below,
    ['a'],
  );
});
