import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Account } from '@handline/client';
import { paceLine } from '#src/index.ts';

const pace = (state: string) => ({ pace: { state, line_pct: 50, used_pct: 55, rate_pct_h: 0.3, back_at: null } }) as Pick<Account, 'pace'>;

test('paceLine says where each pace state stands, as a plain fact', () => {
  assert.equal(paceLine(pace('behind')), 'on track');
  assert.equal(paceLine(pace('ahead')), 'ahead of pace');
  assert.equal(paceLine(pace('over')), 'over pace');
});

test('paceLine is null with pacing off, no pace, or a state it does not know', () => {
  assert.equal(paceLine(pace('off')), null);
  assert.equal(paceLine({ pace: null }), null);
  assert.equal(paceLine({}), null);
  assert.equal(paceLine(pace('later')), null);
});

test('the short pace line fits a row: one word', () => {
  assert.equal(paceLine(pace('behind'), true), 'on track');
  assert.equal(paceLine(pace('ahead'), true), 'ahead');
  assert.equal(paceLine(pace('over'), true), 'over');
});
