import { test } from 'node:test';
import assert from 'node:assert/strict';
import { foldSeats, ownerOf } from '#src/index.ts';

const dir = {
  agents: [
    { key: 'a1', name: 'ui', workspace: 'w1' },
    { key: 'a2', name: 'ui-2', workspace: 'w1', hired_from: 'a1' },
    { key: 'a3', name: 'ui-3', workspace: 'w1', hired_from: 'a1' },
    { key: 'a4', name: 'lead', workspace: 'w1' },
    { key: 'a5', name: 'tools-2', workspace: 'w1', hired_from: 'a6' },
  ],
};

test('an agent and its temps fold to one entry with how many run and how many work', () => {
  const working = new Set(['a1', 'a2', 'a3']);
  assert.deepEqual(
    foldSeats(dir, ['a2', 'a4', 'a1', 'a3'], (k) => working.has(k)),
    [
      { key: 'a1', sessions: 3, working: 3 },
      { key: 'a4', sessions: 1, working: 0 },
    ],
  );
});

test('a temp whose agent is not on the list stands under its agent alone', () => {
  assert.deepEqual(
    foldSeats(dir, ['a5'], () => true),
    [{ key: 'a6', sessions: 1, working: 1 }],
  );
  assert.equal(ownerOf(dir, 'a9'), 'a9');
});
