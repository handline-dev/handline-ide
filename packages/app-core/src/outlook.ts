// What the Outlook page says in words and sums, the same on every app. Pure.
import type { Outlook } from '@handline/client';
import { span } from '#src/time.ts';

/** How long an account's week lasts at the pace so far. */
export const runwayWords = (r: Outlook['runway'][number]): string =>
  r.runs_out_in_seconds !== null
    ? `runs out in ${span(r.runs_out_in_seconds * 1000)}`
    : r.week_resets_at
      ? 'lasts until the week resets'
      : 'no burn, or a stale reading';

/** The agents with a cost in the window, each one's total in the same order, and the grand total. */
export function costByAgent(o: Outlook): { agents: string[]; sums: number[]; all: number } {
  const agents = [...new Set(o.cost_per_day.flatMap((d) => Object.keys(d.agents)))];
  const sums = agents.map((a) => o.cost_per_day.reduce((sum, d) => sum + (d.agents[a] ?? 0), 0));
  return { agents, sums, all: sums.reduce((a, b) => a + b, 0) };
}

/** The points of the guess line: it starts at the last recorded point, so the two lines join, then goes on with the guessed ones after it. */
export function guessLine(o: Pick<Outlook, 'past' | 'future'>): Outlook['future'] {
  const last = o.past.filter((p) => p.left_pct !== null).at(-1);
  return last ? [last, ...o.future.filter((p) => Date.parse(p.at) > Date.parse(last.at))] : o.future;
}

/** The weekly resets in the span, per account, oldest first; empty from a daemon that sends no `by_account`. */
export const weekResets = (o: Pick<Outlook, 'by_account'>): { account: string; at: string }[] =>
  (o.by_account ?? [])
    .flatMap((a) => a.resets.filter((r) => r.window === '7d').map((r) => ({ account: a.account, at: r.at })))
    .sort((a, b) => Date.parse(a.at) - Date.parse(b.at));

/** Each account's own guess for the week left unused at its next weekly reset: its guess line's last point before it, as the daemon gives it. An account whose guess does not pass the reset has none. */
export function lostByAccount(o: Pick<Outlook, 'now' | 'by_account'>): { account: string; at: string; pct: number }[] {
  return (o.by_account ?? []).flatMap((a) => {
    const future = a.future ?? []; // an older daemon sends an account with no guess line
    const reset = a.resets.find((r) => r.window === '7d' && Date.parse(r.at) > Date.parse(o.now));
    if (!reset || !future.some((p) => Date.parse(p.at) > Date.parse(reset.at))) return [];
    const before = future.filter((p) => p.week_left !== null && Date.parse(p.at) <= Date.parse(reset.at)).at(-1);
    return before ? [{ account: a.account, at: reset.at, pct: before.week_left! }] : [];
  });
}

/** The one the bar shows: the account whose weekly reset comes last. */
export const lostAtLastReset = (o: Pick<Outlook, 'now' | 'by_account'>) =>
  lostByAccount(o)
    .sort((a, b) => Date.parse(a.at) - Date.parse(b.at))
    .at(-1) ?? null;

type Forecast = NonNullable<NonNullable<Outlook['by_account']>[number]['forecast']>;

/** The least guessed loss an account row names: under it the row stays quiet. */
export const UNUSED_SHOWN_PCT = 10;

/** An account row's line under its Week bar, "71% · $3.28 unused" at its next reset (a click on the row puts the reset on the graph); null with no guess or one under UNUSED_SHOWN_PCT. */
export function unusedLine(f: Forecast | null | undefined): string | null {
  if (!f || f.unused_pct < UNUSED_SHOWN_PCT) return null;
  const usd = f.unused_usd === null ? '' : ` · $${f.unused_usd.toFixed(2)}`;
  return `${Math.round(f.unused_pct)}%${usd} unused`;
}

/** The account the graph is focused on after a click on `name`'s row: a second click on the same row goes back to all (null). */
export const focusAfter = (focused: string | null, name: string): string | null => (focused === name ? null : name);

/** Under this long to the stop, or from this much of the 5-hour window used, the stop time is red. */
const STOP_SOON_MS = 15 * 60_000;
const STOP_NEAR_PCT = 95;

/** When an account's agents stop: its 5-hour window running out before it resets, at its use over the last day. `left` is how long
 *  that is from now, in words. */
export type StopTime = { at: string; leftMs: number; left: string; tone: 'warn' | 'bad' };

/** How long until the stop, as the gauges word the time to a reset ("25m", "1h 52m"). The last minute is "under a minute": rounded to
 *  minutes it would read "0m", or "1m" with seconds to go. */
const stopLeft = (ms: number): string => (ms < 60_000 ? 'under a minute' : span(ms).replace(/ ([dhm])\b/g, '$1'));

/** The stop time of `account` as the daemon guesses it (by_account's five_runs_out_at), with how long that is from `now`. A stop time is
 *  always a warning, since the window resets only after it; it is red under 15 minutes or from 95% of the window used (`usedPct`, null
 *  with no reading). Null when the daemon gives none (the window lasts to its reset, is not in use, its reading is stale, or there is no
 *  5-hour window) and once the time has passed, when the gauge itself says what is left. */
export function stopTime(
  o: Pick<Outlook, 'by_account'> | undefined,
  account: string,
  usedPct: number | null,
  now: number,
): StopTime | null {
  const at = o?.by_account?.find((a) => a.account === account)?.five_runs_out_at;
  if (!at) return null;
  const leftMs = Date.parse(at) - now;
  // At the stop itself, and after it, there is nothing left to count down: no line, never "in 0m".
  if (!(leftMs > 0)) return null;
  return { at, leftMs, left: stopLeft(leftMs), tone: leftMs < STOP_SOON_MS || (usedPct ?? 0) >= STOP_NEAR_PCT ? 'bad' : 'warn' };
}

/** The labels under the time axis: every 6 hours (clock) over a day and a half or less, else each local midnight (weekday), thinned to at most 8. */
export function axisTicks(t0: number, t1: number): { at: number; label: string }[] {
  if (!(t1 > t0)) return [];
  const short = t1 - t0 <= 36 * 3_600_000;
  const d = new Date(t0);
  if (short) d.setMinutes(0, 0, 0);
  else d.setHours(0, 0, 0, 0);
  const ticks: { at: number; label: string }[] = [];
  for (; d.getTime() <= t1; short ? d.setHours(d.getHours() + 1) : d.setDate(d.getDate() + 1)) {
    if (d.getTime() < t0 || (short && d.getHours() % 6 !== 0)) continue;
    ticks.push({
      at: d.getTime(),
      label: short
        ? d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' })
        : d.toLocaleDateString([], { weekday: 'short' }),
    });
  }
  const every = Math.ceil(ticks.length / 8);
  return ticks.filter((_, i) => i % every === 0);
}
