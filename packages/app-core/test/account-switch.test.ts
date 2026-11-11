import { test } from 'node:test';
import assert from 'node:assert/strict';
import { roomRows, switchWords } from '#src/index.ts';

const w = (used_pct: number, resets_at: string | null = '2026-11-10T21:30:00Z') => ({ used_pct, resets_at, source: 'measured' });

test('room is the tighter window; the most room first, the current account last', () => {
  const rows = roomRows(
    [
      { name: 'alpha', five_hour: w(100), weekly: w(45) },
      { name: 'theta', five_hour: w(92), weekly: w(40) },
      { name: 'gamma', five_hour: w(30), weekly: w(80) },
      { name: 'beta', five_hour: w(65), weekly: w(20) },
    ],
    'alpha',
  );
  assert.deepEqual(
    rows.map((r) => [r.name, r.room, r.tight]),
    [
      ['beta', 35, 'five'],
      ['gamma', 20, 'week'],
      ['theta', 8, 'five'],
      ['alpha', 0, 'five'],
    ],
  );
  assert.equal(rows[0].resets, null);
  assert.match(rows[2].resets!, /^resets /);
});

test('the offered account leads; an account with no reading comes after those read', () => {
  const rows = roomRows(
    [
      { name: 'new', five_hour: null, weekly: null },
      { name: 'gamma', five_hour: w(10), weekly: w(10) },
      { name: 'beta', five_hour: w(88), weekly: w(10) },
    ],
    'alpha',
    'beta',
  );
  assert.deepEqual(
    rows.map((r) => [r.name, r.room]),
    [
      ['beta', 12],
      ['gamma', 90],
      ['new', null],
    ],
  );
});

test('the box is locked while saving, switching and starting, not while waiting', () => {
  assert.equal(switchWords({ phase: 'switching', to: 'beta' }), 'Switching to beta…');
  assert.equal(switchWords({ phase: 'waiting', to: null }), null);
});
