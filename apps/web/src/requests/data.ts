import { laterHolds, laterKey, navOf, type LaterAt } from '@handline/app-core';
import { hooks, useFeed, usePrefs } from '@handline/app-data';
import type { PendingRequest, RequestRow } from '@handline/client';
import { useNow } from '#src/format/time.ts';

const FINISHED = new Set<RequestRow['status']>(['done']);

/** A request is a task; a row without one is a chat message from a daemon that still lists them, and is the chat's. */
export type TaskRequest = RequestRow & { task: number };

/** The founder's requests in the sections of the Requests view and the bell: their turn, notes to them, in progress, finished. `count` is
 * the one number the bell and the sidebar's Requests show: the Your turn rows, without notes or rows put off. */
export function useRequests() {
  const pending = hooks.useRequestsPending();
  // The daemon's warnings reach requests.pending only through the for_you topic's events.
  useFeed('for_you', true);
  const list = hooks.useRequestsList({ limit: 100 });
  const rows = (list.data?.items ?? []).filter((r): r is TaskRequest => r.task !== null);
  // A row put off for later leaves the founder's turn until its time comes, and the count with it.
  const later = (navOf(usePrefs().data).later ?? {}) as Record<string, LaterAt>;
  const now = useNow(); // a row put off comes back by itself when its time comes
  const open = (task: number) => rows.some((r) => r.task === task && !FINISHED.has(r.status));
  const all: PendingRequest[] = pending.data?.answer ?? [];
  const held = (p: PendingRequest) => {
    const k = laterKey(p);
    return !!k && laterHolds(later[k], now, open);
  };
  const turn = all.filter((p) => !held(p) && p.kind !== 'note');
  return {
    loaded: !!pending.data && !!list.data,
    failed: pending.isError || list.isError,
    turn,
    /** @human comments the founder has not read: news that asks nothing. */
    notes: all.filter((p) => !held(p) && p.kind === 'note'),
    /** Rows put off for later, each with when it comes back. */
    later: all.filter(held).map((p) => ({ p, at: later[laterKey(p)!]! })),
    doing: rows.filter((r) => !FINISHED.has(r.status)),
    done: rows.filter((r) => FINISHED.has(r.status)),
    count: turn.length,
    /** Finished requests the founder has not opened since they ended: the bell's dot. */
    unseen: pending.data?.counts.unseen_results ?? 0,
  };
}
