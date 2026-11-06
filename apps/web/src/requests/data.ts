import { hooks } from '@handline/app-data';
import type { PendingRequest, RequestRow } from '@handline/client';

const FINISHED = new Set<RequestRow['status']>(['done', 'no_work']);

/** The founder's requests in the three sections of the Requests view and the bell: their turn, in progress, finished. `count` is the
 * one number the bell and the sidebar's Requests show (requests.pending's answer). */
export function useRequests() {
  const pending = hooks.useRequestsPending();
  const list = hooks.useRequestsList({ limit: 100 });
  const rows = list.data?.items ?? [];
  const turn: PendingRequest[] = pending.data?.answer ?? [];
  return {
    loaded: !!pending.data && !!list.data,
    failed: pending.isError || list.isError,
    turn,
    doing: rows.filter((r) => !FINISHED.has(r.status)),
    done: rows.filter((r) => FINISHED.has(r.status)),
    count: pending.data?.counts.answer ?? 0,
  };
}
