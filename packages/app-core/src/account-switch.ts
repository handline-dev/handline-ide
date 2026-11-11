// What the chat says while an agent changes accounts at its usage limit, and how the accounts it may go to are ranked: by the room
// left in the tighter of their two windows. Pure: the page passes the accounts and the time.
import type { Window } from '@handline/client';
import { clockTime, dateTime } from '#src/time.ts';

export type SwitchPhase = 'saving' | 'switching' | 'starting' | 'waiting';

/** The words the message box shows while it is locked for a switch; a wait does not lock it. */
export function switchWords(s: { phase: SwitchPhase; to: string | null }): string | null {
  if (s.phase === 'saving') return 'Saving progress…';
  if (s.phase === 'switching') return `Switching to ${s.to ?? 'another account'}…`;
  if (s.phase === 'starting') return `Starting on ${s.to ?? 'the new account'}…`;
  return null;
}

/** The wait's line: "Waiting until 21:30 (alpha resets)". */
export const waitLine = (from: string, until: string | null): string =>
  until ? `Waiting until ${clockTime(until)} (${from} resets)` : `Waiting for ${from} to reset`;

type Acct = { name: string; five_hour: Window | null; weekly: Window | null };

/** One account in the switch list: room left in each window, the tighter of the two, and when the tighter one resets. */
export type RoomRow = {
  name: string;
  five: number | null;
  week: number | null;
  /** The room the account has: the smaller of the two, null when neither is read. */
  room: number | null;
  /** Which window is the tighter, null when neither is read. */
  tight: 'five' | 'week' | null;
  /** When the tighter window resets, in words, when it is out of room or nearly; else null. */
  resets: string | null;
  current: boolean;
};

const left = (w: Window | null) => (w ? Math.max(0, Math.round(100 - w.used_pct)) : null);
/** Below this much room the row says when its tighter window resets. */
const LOW_ROOM = 25;

export function roomRow(a: Acct, current: string): RoomRow {
  const five = left(a.five_hour);
  const week = left(a.weekly);
  const tight = five === null && week === null ? null : week === null || (five !== null && five <= week) ? 'five' : 'week';
  const room = tight === 'five' ? five : tight === 'week' ? week : null;
  const w = tight === 'five' ? a.five_hour : tight === 'week' ? a.weekly : null;
  const resets =
    room !== null && room < LOW_ROOM && w?.resets_at ? `resets ${tight === 'five' ? clockTime(w.resets_at) : dateTime(w.resets_at)}` : null;
  return { name: a.name, five, week, room, tight, resets, current: a.name === current };
}

/** The accounts an agent on `current` may switch to: `first` (the daemon's pick) at the top, then most room first, then those not
 * read yet, with the current one last. */
export const roomRows = (accounts: Acct[], current: string, first?: string): RoomRow[] => {
  const rank = (r: RoomRow) => (r.current ? 3 : r.name === first ? 0 : r.room === null ? 2 : 1);
  return accounts
    .map((a) => roomRow(a, current))
    .sort((x, y) => rank(x) - rank(y) || (y.room ?? 0) - (x.room ?? 0) || x.name.localeCompare(y.name));
};
