// Picking rows of Requests, and what can be done to all the picked rows at once. Pure.
import type { PendingRequest, RequestRow } from '@handline/client';
import { laterKey } from '#src/later.ts';

/** A row the person picked: one of their turn, or one of their requests. */
export type Picked = { key: string; p?: PendingRequest; r?: RequestRow };
export const turnKey = (p: PendingRequest) => `turn:${p.source ?? ''}:${p.id ?? p.task ?? p.items[0]}`;
export const requestKey = (r: RequestRow) => `request:${r.cursor}`;
export const turnRow = (p: PendingRequest): Picked => ({ key: turnKey(p), p });
export const requestRow = (r: RequestRow): Picked => ({ key: requestKey(r), r });

const NOUN: Record<PendingRequest['kind'], string> = {
  allow: 'permission',
  decide: 'decision',
  ask: 'question',
  warning: 'warning',
  comment: 'comment',
  note: 'note',
};
const counted = (n: number, noun: string) => `${n} ${noun}${n === 1 ? '' : 's'}`;
/** Turns counted by their kind, "3 questions"; turns of several kinds are "items". */
export function turnsCounted(ps: PendingRequest[]): string {
  const kinds = new Set(ps.map((p) => NOUN[p.kind]));
  const [only] = kinds;
  return counted(ps.length, kinds.size === 1 && only ? only : 'item');
}

/** A finished request the founder has not opened since it ended. */
export const isUnseen = (r: RequestRow): boolean => r.ended?.seen === false;

/** Several rows can stand for one task: a task takes each write once, or the second would fail on what the first did. The write
 * carries the keys of all its rows, as it acts on all of them. */
function perTask<T extends { keys: string[]; task: number }>(on: T[]): T[] {
  const byTask = new Map<number, T>();
  for (const x of on) {
    const first = byTask.get(x.task);
    if (first) first.keys.push(...x.keys);
    else byTask.set(x.task, x);
  }
  return [...byTask.values()];
}
const rowsIn = (on: { keys: string[] }[]) => on.reduce((n, x) => n + x.keys.length, 0);

/** The finished rows not seen yet, each task once. */
export const unseenOf = (rows: Picked[]) =>
  perTask(rows.flatMap((x) => (x.r && x.r.task !== null && isUnseen(x.r) ? [{ keys: [x.key], task: x.r.task }] : [])));

/** What several picked rows share: the turns to give back (`mine`: the tasks whose top turn the founder holds), the rows to put off,
 * the results to mark seen, and the founder's open tasks, to mark done or discard. */
export function bulkOf(rows: Picked[], mine: ReadonlySet<number>) {
  return {
    giveBack: perTask(rows.flatMap((x) => (x.p?.task != null && mine.has(x.p.task) ? [{ keys: [x.key], task: x.p.task }] : []))),
    later: rows.flatMap(({ key, p }) => {
      const later = p ? laterKey(p) : null;
      return p && later ? [{ keys: [key], later, p }] : [];
    }),
    seen: unseenOf(rows),
    close: perTask(
      rows.flatMap((x) => (x.r && x.r.task !== null && !x.r.ended ? [{ keys: [x.key], task: x.r.task, title: x.r.title }] : [])),
    ),
  };
}

/** What each action of several rows is called, naming the rows it acts on: "Discard 3 tasks". */
export function bulkNames(b: ReturnType<typeof bulkOf>) {
  return {
    seen: `Mark ${counted(rowsIn(b.seen), 'result')} seen`,
    later: `Put ${turnsCounted(b.later.map((x) => x.p))} off`,
    giveBack: `Give ${counted(rowsIn(b.giveBack), 'turn')} back`,
    done: `Mark ${counted(b.close.length, 'task')} done`,
    discard: `Discard ${counted(b.close.length, 'task')}`,
  };
}

/** How the select-all box stands over the rows shown. */
export function pickAllState(shown: Picked[], picked: ReadonlyMap<string, Picked>): 'none' | 'some' | 'all' {
  const n = shown.filter((x) => picked.has(x.key)).length;
  return n === 0 ? 'none' : n === shown.length ? 'all' : 'some';
}

/** A close refusal as one plain line plus the task to open; other messages pass through. */
export function refusalLine(message: string): { text: string; open: number | null } {
  const kids = /task T(\d+) still has open child tasks \(([^)]*)\)/.exec(message);
  if (kids) {
    const first = /T(\d+)/.exec(kids[2]!);
    return { text: `T${kids[1]} has sub-tasks still open (${kids[2]})`, open: first ? Number(first[1]) : null };
  }
  return { text: message, open: null };
}
