// What an account says about itself: whether a login runs, and what a usage window reads at a time. Pure: the page passes the time.
import type { Account, Window } from '@handline/client';

/** A login the daemon has not ended: the URL (when this app started it) and the code box belong to it. */
export const loginRunning = (a: Account): boolean => a.login?.state === 'waiting' || a.login?.state === 'code_sent';

/** Past this age a reading is shown with how old it is. */
export const STALE_S = 5 * 60;

/** What one window's number may honestly say. `not-open`: read, but no such window is open. */
export type WindowRead = { kind: 'pct'; pct: number; ago: string | null } | { kind: 'ended' } | { kind: 'not-open' } | { kind: 'none' };

export function windowRead(w: Window | null, ageS: number | null | undefined, now: number): WindowRead {
  if (!w) return ageS == null ? { kind: 'none' } : { kind: 'not-open' };
  if (w.resets_at && Date.parse(w.resets_at) <= now) return { kind: 'ended' };
  const m = ageS != null && ageS > STALE_S ? Math.floor(ageS / 60) : null;
  const ago = m === null ? null : m < 60 ? `${m}m ago` : `${Math.floor(m / 60)}h ${m % 60}m ago`;
  return { kind: 'pct', pct: Math.round(w.used_pct), ago };
}
