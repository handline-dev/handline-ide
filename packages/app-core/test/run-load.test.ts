import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { RunLoad } from '@handline/client';
import { RUN_POLICIES, RUN_POLICY_LINE, RUN_POLICY_WORD, causeLines, hostLine, policyNow, runLoadChip, runTime } from '#src/index.ts';

const status = (r: Partial<RunLoad> = {}): RunLoad => ({
  policy: 'observe',
  slow: false,
  since: null,
  causes: [],
  cap: null,
  heavy: 0,
  waiting: 0,
  ...r,
});

test('the status bar says nothing on a calm Mac with no run waiting', () => {
  assert.equal(runLoadChip(null), null);
  assert.equal(runLoadChip(undefined), null);
  assert.equal(runLoadChip(status({ heavy: 6 })), null);
  assert.equal(runLoadChip(status({ policy: 'fixed', cap: 4, heavy: 4 })), null);
});

test('the status bar names a slow Mac with its runs, and runs that wait', () => {
  assert.deepEqual(runLoadChip(status({ slow: true, heavy: 5 })), { text: 'Mac is slow · 5 heavy runs', slow: true });
  assert.deepEqual(runLoadChip(status({ slow: true, heavy: 1, waiting: 2 })), {
    text: 'Mac is slow · 1 heavy run · 2 waiting',
    slow: true,
  });
  assert.deepEqual(runLoadChip(status({ policy: 'fixed', cap: 2, heavy: 2, waiting: 1 })), {
    text: '1 run waiting for a turn',
    slow: false,
  });
  assert.deepEqual(runLoadChip(status({ waiting: 3 })), { text: '3 runs waiting for a turn', slow: false });
});

test('each cause is a sentence with its figure, in the order given', () => {
  assert.deepEqual(causeLines({ causes: ['runs', 'timeouts', 'loop'], late: 3, timed_out: 2, loop_ms: 41_000 }), [
    '3 heavy runs are taking longer than usual',
    '2 heavy runs ran out of time in the last 5 minutes',
    "Handline's own loop took 41s",
  ]);
  assert.deepEqual(causeLines({ causes: ['runs'], late: 1, timed_out: 0, loop_ms: 0 }), ['1 heavy run is taking longer than usual']);
  assert.deepEqual(causeLines({ causes: ['loop'], late: 0, timed_out: 0, loop_ms: 293_000 }), ["Handline's own loop took 4m"]);
  assert.deepEqual(causeLines({ causes: [], late: 9, timed_out: 9, loop_ms: 9 }), []);
});

test('the host line leaves out normal memory pressure and light compression', () => {
  assert.equal(hostLine({ load_per_core: 0.32, memory_pressure: 'normal', compressed_pages_per_s: 12 }), 'Load 0.3 per core');
  assert.equal(hostLine({ load_per_core: 2, memory_pressure: null, compressed_pages_per_s: 999 }), 'Load 2.0 per core');
  assert.equal(
    hostLine({ load_per_core: 17.25, memory_pressure: 'warn', compressed_pages_per_s: 17_600 }),
    'Load 17.3 per core · memory pressure warn · compressing 18k pages/s',
  );
});

test('what is done now follows the policy, the cap and whether the Mac is slow', () => {
  const now = (policy: RunLoad['policy'], slow: boolean, cap: number | null) => policyNow({ policy, slow, cap });
  assert.equal(now('observe', true, null), 'Nothing waits: Handline only tells you.');
  assert.equal(now('priority', true, null), 'The heavy runs going now have a lower priority.');
  assert.equal(now('priority', false, null), 'Heavy runs get a lower priority when the Mac is slow.');
  assert.equal(now('turns', false, null), 'New heavy runs take turns when the Mac is slow.');
  assert.equal(now('turns', true, 4), 'New heavy runs take turns: at most 4 at once.');
  assert.equal(now('turns', false, 5), 'New heavy runs take turns: at most 5 at once, more as the Mac stays calm.');
  assert.equal(now('fixed', false, 6), 'Heavy runs take turns: at most 6 at once.');
});

test("a run's time reads against what its shape usually takes", () => {
  const now = Date.parse('2026-10-19T00:10:00Z');
  assert.equal(runTime('2026-10-19T00:01:00Z', 364, now), '9m · usually 6m');
  assert.equal(runTime('2026-10-19T00:09:20Z', null, now), '40s');
  assert.equal(runTime('2026-10-19T00:09:20Z', 0, now), '40s');
});

test('every policy has a word and a line', () => {
  for (const p of RUN_POLICIES) assert.ok(RUN_POLICY_WORD[p] && RUN_POLICY_LINE[p], p);
  assert.equal(RUN_POLICIES[0], 'observe');
});
