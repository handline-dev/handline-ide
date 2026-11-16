import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Window } from '@handline/client';
import { loginRunning, windowRead } from '#src/index.ts';

test('a window reads as its percent, aged when old, ended once its reset passed, and not open when the account was read without it', () => {
  const now = Date.parse('2026-11-11T10:00:00Z');
  const w = { used_pct: 42.4, resets_at: '2026-11-11T12:00:00Z' } as Window;
  assert.deepEqual(windowRead(w, 60, now), { kind: 'pct', pct: 42, ago: null });
  assert.deepEqual(windowRead(w, 12 * 60, now), { kind: 'pct', pct: 42, ago: '12m ago' });
  assert.deepEqual(windowRead({ ...w, resets_at: '2026-11-11T09:00:00Z' }, 60, now), { kind: 'ended' });
  assert.deepEqual(windowRead(null, 60, now), { kind: 'not-open' });
  assert.deepEqual(windowRead(null, null, now), { kind: 'none' });
});

test('a login runs while it waits or has sent its code', () => {
  assert.equal(loginRunning({ login: { state: 'waiting' } } as never), true);
  assert.equal(loginRunning({ login: { state: 'code_sent' } } as never), true);
  assert.equal(loginRunning({} as never), false);
});
