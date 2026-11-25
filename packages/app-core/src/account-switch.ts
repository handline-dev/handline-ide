// What the chat says while an agent changes accounts at its usage limit, and how the accounts it may go to are ranked: by the room
// left in the tighter of their two windows. Pure: the page passes the accounts and the time.
import type { Window } from '@handline/client';
import { clockTime, dateTime, shortSpan } from '#src/time.ts';

export type SwitchPhase = 'saving' | 'switching' | 'note' | 'starting' | 'waiting';

/** The words the message box shows while it is locked for a switch; a wait does not lock it. */
export function switchWords(s: { phase: SwitchPhase; to: string | null; waits?: string | null }): string | null {
  if (s.phase === 'saving') return s.waits ? `Preparing to change accounts… · ${waitsWords(s.waits)}` : 'Preparing to change accounts…';
  if (s.phase === 'switching') return `Switching to ${s.to ?? 'another account'}…`;
  if (s.phase === 'note') return `Switching to ${s.to ?? 'another account'}… · writing its handoff note`;
  if (s.phase === 'starting') return `Starting on ${s.to ?? 'the new account'}…`;
  return null;
}

/** One step of a move: done, under way (the current one) or still to come. */
export type MoveStep = { name: string; state: 'done' | 'now' | 'next' };

const ORDER = ['saving', 'switching', 'note', 'starting'] as const;

/** A move's four steps, in the order the daemon goes through them; null for a wait, which is not a move. The current step is the one
 * its phase names, so a long step shows where it is and never looks stopped. */
export function moveSteps(s: { phase: SwitchPhase; from: string; to: string | null; waits?: string | null }): {
  title: string;
  steps: MoveStep[];
  at: number;
} | null {
  const at = ORDER.indexOf(s.phase as (typeof ORDER)[number]);
  if (at < 0) return null;
  const names = [
    s.waits ? `Finish turn ${waitsWords(s.waits)}` : 'Finish turn',
    `Close on ${s.from}`,
    'Write handoff note',
    `Start on ${s.to ?? 'the new account'}`,
  ];
  return {
    title: s.to ? `Moving to ${s.to}` : 'Moving to another account',
    steps: names.map((name, i) => ({ name, state: i < at ? 'done' : i === at ? 'now' : 'next' })),
    at,
  };
}

/** What a saving switch waits for, from the daemon's words: the turn to end, its handoff note, or background: <jobs>. */
const waitsWords = (w: string): string =>
  w === 'the turn to end' ? 'after this turn' : w.startsWith('background: ') ? `after ${w.slice(12)} ends` : `after ${w}`;

/** The wait's line: "Waiting for alpha to reset · 5m left"; the clock time it resets at is the line's hover. */
export const waitLine = (from: string, until: string | null, now = Date.now()): string =>
  until && !Number.isNaN(Date.parse(until))
    ? `Waiting for ${from} to reset · ${shortSpan(Math.max(60, (Date.parse(until) - now) / 1000))} left`
    : `Waiting for ${from} to reset`;

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
