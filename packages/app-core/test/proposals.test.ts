import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Proposal } from '@handline/client';
import { isWaiting, waitingCount } from '#src/proposals.ts';

const p = (state: string): Proposal => ({ id: 1, title: 'settings a1', subject: 's1', state, created: '2026-10-01T00:00:00Z' }) as Proposal;

test('open and apply_failed wait for the founder; an applying, settled or unknown state does not', () => {
  assert.deepEqual(
    ['open', 'apply_failed', 'applying', 'approved', 'rejected', 'withdrawn', 'later'].map((s) => isWaiting(p(s))),
    [true, true, false, false, false, false, false],
  );
  assert.equal(waitingCount([p('open'), p('applying'), p('apply_failed'), p('later')]), 2);
});
