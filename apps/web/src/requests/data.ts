import { laterHolds, navOf, type LaterAt } from '@handline/app-core';
import { hooks, useFeed, usePrefs } from '@handline/app-data';
import type { PendingRequest, RequestRow } from '@handline/client';
import { useNow } from '#src/format/time.ts';

const FINISHED = new Set<RequestRow['status']>(['done', 'no_work']);

/** The For you key a row of the founder's turn is put off under (NavPrefs.later), or null for one that cannot be. */
export const laterKey = (p: PendingRequest): string | null =>
  p.source && p.id && p.source !== 'prompt' && p.source !== 'proposal' ? `${p.source}:${p.id}` : null;

/** The founder's requests in the three sections of the Requests view and the bell: their turn, in progress, finished. `count` is the
 * one number the bell and the sidebar's Requests show (requests.pending's answer). */
export function useRequests() {
  const pending = hooks.useRequestsPending();
  // The daemon's warnings reach requests.pending only through the for_you topic's events.
  useFeed('for_you', true);
  const list = hooks.useRequestsList({ limit: 100 });
  const rows = list.data?.items ?? [];
  // A row put off for later leaves the founder's turn until its time comes; the count still counts it, as the daemon does.
  const later = (navOf(usePrefs().data).later ?? {}) as Record<string, LaterAt>;
  const now = useNow(); // a row put off comes back by itself when its time comes
  const open = (task: number) => rows.some((r) => r.task === task && !FINISHED.has(r.status));
  const all: PendingRequest[] = pending.data?.answer ?? [];
  const held = (p: PendingRequest) => {
    const k = laterKey(p);
    return !!k && laterHolds(later[k], now, open);
  };
  return {
    loaded: !!pending.data && !!list.data,
    failed: pending.isError || list.isError,
    turn: all.filter((p) => !held(p)),
    /** Rows put off for later, each with when it comes back. */
    later: all.filter(held).map((p) => ({ p, at: later[laterKey(p)!]! })),
    doing: rows.filter((r) => !FINISHED.has(r.status)),
    done: rows.filter((r) => FINISHED.has(r.status)),
    count: pending.data?.counts.answer ?? 0,
    /** Finished requests the founder has not opened since they ended: the bell's dot. */
    unseen: pending.data?.counts.unseen_results ?? 0,
  };
}
