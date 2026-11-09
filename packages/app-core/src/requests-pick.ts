// Picking rows of Requests, and what can be done to all the picked rows at once. Pure.
import type { PendingRequest, RequestRow } from '@handline/client';
import { laterKey } from '#src/later.ts';

/** A row the person picked: one of their turn, or one of their requests. */
export type Picked = { key: string; p?: PendingRequest; r?: RequestRow };
export const turnKey = (p: PendingRequest) => `turn:${p.source ?? ''}:${p.id ?? p.task ?? p.items[0]}`;
export const requestKey = (r: RequestRow) => `request:${r.cursor}`;

/** Every row of the list, in the order it shows them. */
export const listedRows = (turn: PendingRequest[], requests: RequestRow[]): Picked[] => [
  ...turn.map((p) => ({ key: turnKey(p), p })),
  ...requests.map((r) => ({ key: requestKey(r), r })),
];

/** A finished request the founder has not opened since it ended. */
export const isUnseen = (r: RequestRow): boolean => r.ended?.seen === false;

/** Several rows can stand for one task: a task takes each write once, or the second would fail on what the first did. */
function perTask<T extends { task: number }>(on: T[]): T[] {
  const tasks = new Set<number>();
  return on.filter((x) => !tasks.has(x.task) && !!tasks.add(x.task));
}

/** The finished rows not seen yet, each task once. */
export const unseenOf = (rows: Picked[]) =>
  perTask(rows.flatMap((x) => (x.r && x.r.task !== null && isUnseen(x.r) ? [{ key: x.key, task: x.r.task }] : [])));

/** What several picked rows share: the turns to give back (`mine`: the tasks whose top turn the founder holds), the rows to put off,
 * the results to mark seen, the open requests to cancel. */
export function bulkOf(rows: Picked[], mine: ReadonlySet<number>) {
  return {
    giveBack: perTask(rows.flatMap((x) => (x.p?.task != null && mine.has(x.p.task) ? [{ key: x.key, task: x.p.task }] : []))),
    later: rows.flatMap((x) => {
      const later = x.p ? laterKey(x.p) : null;
      return later ? [{ key: x.key, later }] : [];
    }),
    seen: unseenOf(rows),
    cancel: perTask(
      rows.flatMap((x) => (x.r && x.r.task !== null && !x.r.ended ? [{ key: x.key, task: x.r.task, title: x.r.title }] : [])),
    ),
  };
}

/** How the select-all box stands over the rows shown. */
export function pickAllState(shown: Picked[], picked: ReadonlyMap<string, Picked>): 'none' | 'some' | 'all' {
  const n = shown.filter((x) => picked.has(x.key)).length;
  return n === 0 ? 'none' : n === shown.length ? 'all' : 'some';
}
