import { test } from 'node:test';
import assert from 'node:assert/strict';
import { leftPath, resetMarks, usedPath } from '#src/usage-lines.ts';

const t = (h: number) => new Date(Date.UTC(2026, 10, 25, h)).toISOString();
const from = Date.parse(t(0));
const to = Date.parse(t(10));

test('used percent rises upward: 100% left is the bottom, 0% left the top', () => {
  assert.equal(
    usedPath(
      [
        { at: t(0), left: 100 },
        { at: t(10), left: 0 },
      ],
      from,
      to,
      100,
      50,
    ),
    'M0.0,50.0L100.0,0.0',
  );
});

test('a missing sample breaks the line instead of drawing to zero', () => {
  assert.equal(
    usedPath(
      [
        { at: t(0), left: 80 },
        { at: t(5), left: null },
        { at: t(10), left: 60 },
      ],
      from,
      to,
      100,
      50,
    ),
    'M0.0,40.0M100.0,30.0',
  );
});

test('a reset sent as two points at one time is a vertical line, what is left drawn up', () => {
  assert.equal(
    leftPath(
      [
        { at: t(0), left: 40 },
        { at: t(5), left: 20 },
        { at: t(5), left: 100 },
        { at: t(10), left: 90 },
      ],
      from,
      to,
      100,
      50,
    ),
    'M0.0,30.0L50.0,40.0L50.0,0.0L100.0,5.0',
  );
});

test("the marks are the one window's resets inside the span", () => {
  const resets = [
    { at: t(1), window: '5h' },
    { at: t(2), window: '7d' },
    { at: t(6), window: '5h' },
    { at: t(11), window: '5h' },
  ];
  assert.deepEqual(resetMarks(resets, '5h', from, to), [t(1), t(6)]);
});
