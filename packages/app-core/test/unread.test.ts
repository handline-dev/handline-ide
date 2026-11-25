import { test } from 'node:test';
import assert from 'node:assert/strict';
import { foldMarks, unreadAgents, unreadCount } from '#src/unread.ts';

const a = (key: string, n: number, at = '') => ({ key, founder_unread: n, last_to_founder: at ? { at } : null });

test('the unread view lists agents with unread messages, newest message first', () => {
  const list = unreadAgents([a('x', 1, '2026-11-25T10:00'), a('y', 0, '2026-11-25T12:00'), a('z', 2, '2026-11-25T11:00')], new Set());
  assert.deepEqual(
    list.map((x) => x.key),
    ['z', 'x'],
  );
});

test('an agent read while the view is on stays in it', () => {
  assert.deepEqual(
    unreadAgents([a('x', 0, '2026-11-25T10:00')], new Set(['x'])).map((x) => x.key),
    ['x'],
  );
});

test('the count is of agents, not of messages', () => {
  assert.equal(unreadCount([a('x', 3), a('y', 0), a('z', 1)]), 2);
});

test('a folded workspace shows needs-you, error and working counts above zero, in that order', () => {
  assert.deepEqual(foldMarks(['working', 'resting', 'you', 'working', 'asleep']), [
    { act: 'you', n: 1 },
    { act: 'working', n: 2 },
  ]);
  assert.deepEqual(foldMarks(['resting', 'asleep']), []);
});
