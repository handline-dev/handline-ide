// Under an account's 5-hour gauge: when its agents stop, if the window runs out before it resets. The time is the daemon's guess
// (stats.outlook by_account, the read the account's 5-hour history draws from); nothing is shown when it gives none.
import { stopTime, windowRead } from '@handline/app-core';
import { useOutlook } from '@handline/app-data';
import type { Account } from '@handline/client';
import { useNow } from '#src/format/time.ts';

/** "17:40", on the 24-hour clock whatever the locale, as the reset times of the Outlook graph. */
const clockOf = (iso: string) => new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' });

/** The clock is read this often here, so the last minute says so and the line goes soon after the stop, not up to a minute late. */
const TICK_MS = 10_000;

export function StopTime({ account: a }: { account: Account }) {
  const now = useNow(TICK_MS);
  const outlook = useOutlook('24h').data;
  const read = windowRead(a.five_hour, a.age_s, now);
  const stop = read.kind === 'pct' ? stopTime(outlook, a.name, read.pct, now) : null;
  if (!stop) return null;
  return (
    <span
      className={`mt-1 block text-xs ${stop.tone === 'bad' ? 'text-danger' : 'text-warning'}`}
      title="A guess from its use over the last day: the 5-hour window fills before it resets, and the agents on this account wait for the reset."
    >
      {`agents stop at ${clockOf(stop.at)} at this rate (in ${stop.left})`}
    </span>
  );
}
