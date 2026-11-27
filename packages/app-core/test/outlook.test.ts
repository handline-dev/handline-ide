import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import type { Outlook } from '@handline/client';
import {
  axisTicks,
  costByAgent,
  focusAfter,
  guessLine,
  lostAtLastReset,
  lostByAccount,
  stopTime,
  unusedLine,
  weekResets,
} from '#src/outlook.ts';

describe('unusedLine', () => {
  const at = '2026-11-30T09:00:00';
  it('says the loss in percent and plan money at the reset from 10% up', () => {
    assert.equal(unusedLine({ at, unused_pct: 71.4, unused_usd: 3.281 }), '71% · $3.28 unused');
    assert.equal(unusedLine({ at, unused_pct: 10, unused_usd: null }), '10% unused');
  });
  it('is quiet under 10% and with no guess', () => {
    assert.equal(unusedLine({ at, unused_pct: 9.9, unused_usd: 1 }), null);
    assert.equal(unusedLine(null), null);
  });
});

describe('focusAfter', () => {
  it('focuses a clicked row, and a second click on it goes back to all', () => {
    assert.equal(focusAfter(null, 'alpha'), 'alpha');
    assert.equal(focusAfter('beta', 'alpha'), 'alpha');
    assert.equal(focusAfter('alpha', 'alpha'), null);
  });
});

const p = (at: string, left_pct: number | null) => ({ at, left_pct });

describe('guessLine', () => {
  it('starts at the last recorded point so the guess joins the recorded line', () => {
    const past = [p('2026-10-26T10:00:00Z', 80), p('2026-10-26T11:00:00Z', 70)];
    const future = [p('2026-10-26T11:30:00Z', 66), p('2026-10-26T12:30:00Z', 60)];
    assert.deepEqual(guessLine({ past, future }), [past[1], ...future]);
  });
  it('skips a last point with no reading and drops a guess that is not after the join', () => {
    const past = [p('2026-10-26T10:00:00Z', 80), p('2026-10-26T11:00:00Z', null)];
    const future = [p('2026-10-26T09:00:00Z', 90), p('2026-10-26T11:30:00Z', 66)];
    assert.deepEqual(guessLine({ past, future }), [past[0], future[1]]);
  });
  it('is the guess alone when nothing was recorded', () => {
    const future = [p('2026-10-26T11:30:00Z', 66)];
    assert.deepEqual(guessLine({ past: [], future }), future);
  });
});

const w = (at: string, week_left: number | null) => ({ at, five_left: null, week_left, line_left: null });
const fresh = { five_hour: true, forecast: null, five_runs_out_at: null, five_trace: [], week_trace: [] };
const by = {
  ...fresh,
  account: 'alpha',
  past: [w('2026-10-26T10:00:00Z', 60), w('2026-10-26T11:00:00Z', 55)],
  future: [w('2026-10-26T12:00:00Z', 50), w('2026-10-26T14:00:00Z', 40), w('2026-10-26T16:00:00Z', 30)],
  resets: [
    { at: '2026-10-25T00:00:00Z', window: '7d' as const, unused_pct: 18 },
    { at: '2026-10-25T05:00:00Z', window: '5h' as const },
    { at: '2026-10-26T15:00:00Z', window: '7d' as const },
  ],
};
const o = { now: '2026-10-26T11:00:00Z', by_account: [by] };

describe('weekResets', () => {
  it('lists the weekly resets of every account, oldest first, and none for a daemon without by_account', () => {
    assert.deepEqual(weekResets(o), [
      { account: 'alpha', at: '2026-10-25T00:00:00Z' },
      { account: 'alpha', at: '2026-10-26T15:00:00Z' },
    ]);
    assert.deepEqual(weekResets({}), []);
  });
});
describe('lostByAccount', () => {
  it("is each account's own guess just before its next weekly reset, and the last reset's account is the bar's", () => {
    const beta = {
      ...fresh,
      account: 'beta',
      past: [],
      future: [w('2026-10-26T13:00:00Z', 70), w('2026-10-26T19:00:00Z', 65)],
      resets: [{ at: '2026-10-26T18:00:00Z', window: '7d' as const }],
    };
    const two = { now: o.now, by_account: [by, beta] };
    assert.deepEqual(lostByAccount(two), [
      { account: 'alpha', at: '2026-10-26T15:00:00Z', pct: 40 },
      { account: 'beta', at: '2026-10-26T18:00:00Z', pct: 70 },
    ]);
    assert.deepEqual(lostAtLastReset(two), { account: 'beta', at: '2026-10-26T18:00:00Z', pct: 70 });
  });
  it('is nothing for an account whose guess stops before its reset, or a daemon without by_account', () => {
    const short = { ...by, future: by.future.slice(0, 2) };
    assert.deepEqual(lostByAccount({ now: o.now, by_account: [short] }), []);
    assert.equal(lostAtLastReset({ now: o.now }), null);
  });
  it('is nothing, and no throw, for an account the daemon sends without its guess line', () => {
    const { future: _gone, ...old } = by;
    assert.deepEqual(lostByAccount({ now: o.now, by_account: [old as typeof by] }), []);
  });
});
describe('stopTime', () => {
  const now = Date.parse('2026-10-26T11:00:00Z');
  const at = (min: number) => new Date(now + min * 60_000).toISOString();
  const of = (runsOut: string | null) => ({ by_account: [{ ...by, five_runs_out_at: runsOut }] });
  it('is the time the daemon guesses, amber while it is 15 minutes or more away and under 95% used', () => {
    assert.deepEqual(stopTime(of(at(112)), 'alpha', 62, now), { at: at(112), leftMs: 112 * 60_000, left: '1h 52m', tone: 'warn' });
    assert.equal(stopTime(of(at(15)), 'alpha', 94, now)?.tone, 'warn');
    assert.equal(stopTime(of(at(15)), 'alpha', null, now)?.tone, 'warn');
  });
  it('is red under 15 minutes to go, or from 95% of the window used', () => {
    assert.equal(stopTime(of(at(14)), 'alpha', 50, now)?.tone, 'bad');
    assert.equal(stopTime(of(at(112)), 'alpha', 95, now)?.tone, 'bad');
  });
  it('is nothing when the daemon gives no time, the time has passed, the account is not listed, or the read is not in', () => {
    assert.equal(stopTime(of(null), 'alpha', 99, now), null);
    assert.equal(stopTime(of(at(0)), 'alpha', 100, now), null);
    assert.equal(stopTime(of(at(-5)), 'alpha', 100, now), null);
    assert.equal(stopTime(of(at(30)), 'beta', 50, now), null);
    assert.equal(stopTime({}, 'alpha', 50, now), null);
    assert.equal(stopTime(undefined, 'alpha', 50, now), null);
  });
  it('words the last minute as under a minute, never 0m, and says nothing at the stop or a second after it', () => {
    const secs = (s: number) => stopTime(of(new Date(now + s * 1000).toISOString()), 'alpha', 50, now);
    assert.equal(secs(25 * 60)?.left, '25m');
    assert.equal(secs(60)?.left, '1m');
    assert.equal(secs(59)?.left, 'under a minute');
    assert.equal(secs(30)?.left, 'under a minute');
    assert.equal(secs(1)?.left, 'under a minute');
    assert.equal(secs(0), null);
    assert.equal(secs(-1), null);
  });
});
describe('axisTicks', () => {
  it('is a weekday per local midnight over days, and a clock every 6 hours over a day', () => {
    const t0 = new Date(2026, 9, 26, 10).getTime();
    const days = axisTicks(t0, t0 + 6 * 86_400_000);
    assert.equal(days.length, 6);
    assert.ok(days.every((t) => new Date(t.at).getHours() === 0));
    const hours = axisTicks(t0, t0 + 24 * 3_600_000);
    assert.deepEqual(
      hours.map((t) => new Date(t.at).getHours()),
      [12, 18, 0, 6],
    );
  });
  it('is empty for an empty span', () => assert.deepEqual(axisTicks(5, 5), []));
});

describe('costByAgent', () => {
  it('sums each agent over the days, agents in first-seen order', () => {
    const o = {
      cost_per_day: [
        { day: 'd1', agents: { a: 1, b: 2 } },
        { day: 'd2', agents: { a: 3 } },
      ],
    } as unknown as Outlook;
    assert.deepEqual(costByAgent(o), { agents: ['a', 'b'], sums: [4, 2], all: 6 });
    assert.equal(costByAgent({ cost_per_day: [] } as unknown as Outlook).all, 0);
  });
});
