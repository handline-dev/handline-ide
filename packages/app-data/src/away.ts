// What a notice-raising event did while the connection was down: counted once on reconnect, shown as one line, never replayed as toasts.
import { type QueryClient } from '@tanstack/react-query';
import type { Notices } from '@handline/app-core';
import type { DaemonClient } from '@handline/client';

export interface Away {
  count: number;
  /** The daemon kept no more than this many (or dropped older ones): the count is a floor. */
  more: boolean;
}

const LIMIT = 100;
const awayKey = (d: string) => ['away', d] as const;

/** Reads the notices after `from` once and keeps their count under its own key (a resync of the daemon's reads leaves it alone). A failed read leaves no line. With `notices`, the events also go into its list. */
export async function readAway(
  client: DaemonClient,
  qc: QueryClient,
  d: string,
  from: number | undefined,
  notices?: Notices,
): Promise<void> {
  if (from === undefined) return;
  const feed = await client.call('events.feed', { since: from, topics: ['notifications', 'proposals'], limit: LIMIT }).catch(() => null);
  for (const e of feed?.events ?? []) notices?.takeEvent(e); // the missed ones join the list; the live ones taken before are skipped
  if (feed && feed.events.length)
    qc.setQueryData<Away>(awayKey(d), { count: feed.events.length, more: feed.events.length >= LIMIT || feed.gap });
}
