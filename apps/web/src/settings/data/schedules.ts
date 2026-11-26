// What the list of schedules says about time: a schedule's own times are the daemon machine's, and Next and Last the reader's.
import type { Methods } from '@handline/client';

type Zone = Methods['schedules.list']['result']['zone'];

/** A zone's distance from UTC as people write it: UTC+9, UTC-3:30. */
export function offsetWord(min: number): string {
  const [h, m] = [Math.floor(Math.abs(min) / 60), Math.abs(min) % 60];
  return `UTC${min < 0 ? '-' : '+'}${h}${m ? `:${String(m).padStart(2, '0')}` : ''}`;
}

/** Whose clock the schedules run by, and that Next and Last are in the reader's time when the reader's zone (`mine`, minutes east of
 * UTC) is another. */
export function zoneNote(zone: Zone, mine: number = -new Date().getTimezoneOffset()): string {
  const offset = offsetWord(zone.offset_min);
  // A zone with no name of its own comes as an offset (+09, GMT+9): the offset says it once.
  const named = /^(GMT|UTC)?([+-]?\d.*)?$/.test(zone.name) ? offset : `${zone.name}, ${offset}`;
  const theirs = `Each one runs by the clock of the machine Handline runs on (${named}).`;
  return mine === zone.offset_min ? theirs : `${theirs} Next and Last are in your own time.`;
}

/** A time to come as the list writes it, on the 24-hour clock the schedules' sentences use: Today 09:30, Tomorrow 09:30, then the day
 * (Nov 30 09:30). */
export function nextWord(iso: string, now: number = Date.now()): string {
  const at = new Date(iso);
  const hm = `${String(at.getHours()).padStart(2, '0')}:${String(at.getMinutes()).padStart(2, '0')}`;
  const days = Math.round((new Date(at).setHours(0, 0, 0, 0) - new Date(now).setHours(0, 0, 0, 0)) / 86_400_000);
  if (days === 0) return `Today ${hm}`;
  if (days === 1) return `Tomorrow ${hm}`;
  return `${at.toLocaleDateString([], { month: 'short', day: 'numeric' })} ${hm}`;
}
