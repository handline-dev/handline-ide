// What a notice-raising event did while the connection was down: read once on reconnect into the notices list, never replayed as toasts.
import type { Notices } from '@handline/app-core';
import type { DaemonClient } from '@handline/client';

/** Reads the notices after `from` once into `notices`' list; a failed read adds nothing. */
export async function readAway(client: DaemonClient, from: number | undefined, notices?: Notices): Promise<void> {
  if (from === undefined) return;
  const feed = await client.call('events.feed', { since: from, topics: ['notifications', 'proposals'], limit: 100 }).catch(() => null);
  for (const e of feed?.events ?? []) notices?.takeEvent(e); // the missed ones join the list; the live ones taken before are skipped
}
