import { test } from 'node:test';
import assert from 'node:assert/strict';
import { moveSteps, roomRows, switchWords, waitLine } from '#src/index.ts';

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

test('the box is locked while saving, switching, writing the note and starting, not while waiting', () => {
  assert.equal(switchWords({ phase: 'saving', to: null }), 'Preparing to change accounts…');
  assert.equal(switchWords({ phase: 'saving', to: null, waits: 'the turn to end' }), 'Preparing to change accounts… · after this turn');
  assert.equal(
    switchWords({ phase: 'saving', to: null, waits: 'its handoff note' }),
    'Preparing to change accounts… · after its handoff note',
  );
  assert.equal(
    switchWords({ phase: 'saving', to: null, waits: 'background: shell: vite dev' }),
    'Preparing to change accounts… · after shell: vite dev ends',
  );
  assert.equal(switchWords({ phase: 'switching', to: 'beta' }), 'Switching to beta…');
  assert.equal(switchWords({ phase: 'waiting', to: null }), null);
});

test("a move's three steps: the phase's step is now, those before done, those after next", () => {
  const m = moveSteps({ phase: 'switching', from: 'alpha', to: 'sigma' })!;
  assert.equal(m.title, 'Moving to sigma');
  assert.equal(m.at, 1);
  assert.deepEqual(
    m.steps.map((s) => `${s.state}:${s.name}`),
    ['done:Finish turn', 'now:Close on alpha', 'next:Start on sigma'],
  );
  assert.equal(
    moveSteps({ phase: 'saving', from: 'alpha', to: null, waits: 'the turn to end' })!.steps[0].name,
    'Finish turn after this turn',
  );
  assert.equal(moveSteps({ phase: 'waiting', from: 'alpha', to: null }), null);
});

test('the wait line counts down to the reset, a minute at least', () => {
  const now = Date.parse('2026-11-25T13:54:00Z');
  assert.equal(waitLine('alpha', '2026-11-25T13:59:00Z', now), 'Waiting for alpha to reset · 5m left');
  assert.equal(waitLine('alpha', '2026-11-25T13:54:10Z', now), 'Waiting for alpha to reset · 1m left');
  assert.equal(waitLine('alpha', null, now), 'Waiting for alpha to reset');
});
