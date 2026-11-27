// What an office says about its people beyond where they sit (Marks.tsx draws it): the mark over one on hold, stopped or asleep, the line
// under a name saying what its person does, cut so it never reaches the neighbour's, the steps done of a task, and what the papers on a
// desk say on hover. Plain sums over what the town already holds, with no drawing in them.
import type { Task } from '@handline/client';
import type { Activity } from '#src/layout/agentList.ts';
import { WORDS, type Person } from '#src/town/model.ts';

/** The mark over a person: an hourglass on hold, a warning when stopped on a fault, Zzz over the desk of one asleep. */
export type Mark = 'held' | 'error' | 'asleep';
export const markOf = (a: Activity): Mark | null => (a === 'held' || a === 'error' || a === 'asleep' ? a : null);

/** Those whose work shows under their name: whoever sits at a desk. */
const SAYS: Activity[] = ['working', 'you', 'held', 'error'];
const STUCK: Activity[] = ['held', 'error'];

/** The line under a person's name: what it is doing; for one on hold or stopped with no work to name, why, else that word. Null for
 *  whoever has nothing to say. */
export function doingOf(p: Person): string | null {
  if (!SAYS.includes(p.activity)) return null;
  if (p.doing?.trim()) return p.doing.trim();
  return STUCK.includes(p.activity) ? p.why?.trim() || WORDS[p.activity] : null;
}

/** What hovering a person or its desk says: its name, its state when that needs a look, what it is doing and why it waits. */
export function tipOf(p: Person): string {
  const state = STUCK.includes(p.activity) || p.activity === 'you' ? WORDS[p.activity] : null;
  const parts = [p.name, state, p.doing?.trim() || (state ? null : WORDS[p.activity]), state ? p.why?.trim() : null];
  return parts.filter((t, i) => t && parts.indexOf(t) === i).join(' · ');
}

/** How many of a person's tasks wait behind the one it holds, in words; null when none does. */
export function waitingOf(p: Person): string | null {
  const n = Math.max(0, p.queue - (p.task ? 1 : 0));
  return n ? `${n} ${n === 1 ? 'task' : 'tasks'} waiting` : null;
}

/** A person's model and effort ("sonnet · high") and, when known, how full its context is ("Context 25%"); null with none of them. */
export function kindOf(p: Person): string | null {
  const parts = [p.model, p.effort, p.ctx == null ? null : `Context ${Math.round(p.ctx)}%`].filter(Boolean);
  return parts.length ? parts.join(' · ') : null;
}

/** The doing line's size in picture pixels, at `scale` screen pixels to a picture pixel: 9px on screen, shrinking with the names once
 *  the room is drawn small, and null (no line) when it would be under 8px on screen. */
export function doingSize(scale: number): number | null {
  const size = Math.min(9 / scale, 5.3);
  return size * scale < 8 ? null : size;
}

export type At = { x: number; y: number };

/** How wide a line centred on each of `at` may be, in picture pixels: it stops `gap` short of the line of its nearest neighbour on the
 *  same row (those at most `row` apart in y) and of the edges of a picture `w` wide, and is never over `most`. */
export function roomsOf(at: readonly At[], w: number, o: { gap: number; row: number; most: number }): number[] {
  return at.map((a, i) => {
    const near = at.filter((b, j) => j !== i && Math.abs(b.y - a.y) <= o.row).map((b) => Math.abs(b.x - a.x) - o.gap);
    return Math.max(0, Math.min(o.most, 2 * Math.min(a.x, w - a.x) - o.gap, ...near));
  });
}

/** `text` whole when it is at most `room` wide by `widthOf`, else its longest start that fits with an ellipsis after it; null when not
 *  even a letter fits. */
export function fitTo(text: string, room: number, widthOf: (s: string) => number): string | null {
  if (widthOf(text) <= room) return text;
  const letters = [...text];
  const cut = (n: number) => `${letters.slice(0, n).join('').trimEnd()}…`;
  let [fits, over] = [0, letters.length];
  while (over - fits > 1) {
    const mid = Math.floor((fits + over) / 2);
    if (widthOf(cut(mid)) <= room) fits = mid;
    else over = mid;
  }
  return fits > 0 ? cut(fits) : null;
}

/** `text` in at most `most` letters, the last an ellipsis when it was longer. */
export function cutTo(text: string, most: number): string {
  const letters = [...text.trim()];
  return letters.length <= most
    ? letters.join('')
    : `${letters
        .slice(0, most - 1)
        .join('')
        .trimEnd()}…`;
}

/** `text` on at most two lines, each at most `room` wide by `widthOf`: whole when it fits on one; else as many whole words as fit on
 *  the first (a first word too long for a line is cut where the line ends) and the rest on the second, ending in an ellipsis when that
 *  is too long too. Nothing when not even a letter fits. */
export function wrapTo(text: string, room: number, widthOf: (s: string) => number): string[] {
  if (widthOf(text) <= room) return [text];
  const words = text.split(/\s+/);
  let whole = 0;
  while (whole < words.length && widthOf(words.slice(0, whole + 1).join(' ')) <= room) whole++;
  let [first, rest] = [words.slice(0, whole).join(' '), words.slice(whole).join(' ')];
  if (!whole) {
    const letters = [...text];
    let fits = 0;
    while (fits < letters.length && widthOf(letters.slice(0, fits + 1).join('')) <= room) fits++;
    [first, rest] = [letters.slice(0, fits).join(''), letters.slice(fits).join('').trimStart()];
  }
  const second = first ? fitTo(rest, room, widthOf) : null;
  return second ? [first, second] : first ? [first] : [];
}

/** A doing line: its text on one line or two, centred on x under the name whose top is y. */
export type Line = { key: string; x: number; y: number; lines: string[] };

/** How far a line of text `size` tall reaches under its top, and from one line's top to the next's. */
const LINE = { tall: 1.3, step: 1.1 };
/** How far over a head's top its mark reaches, and to each side of its middle. */
const MARK = { over: 7, wide: 14 };

/** The doing lines of an office: under each of `names` whose person (the newest of `people`, by key) has something to say, on two
 *  lines when one is too short, cut to what its row leaves it. `others` are names that take room on a row and get no line: the
 *  founder's own desk's, the empty desk's. A second line that would reach the mark over a head of the row below (`heads`) is left out:
 *  there the first line ends in the ellipsis. `y` is the name's own top, `below` its height. */
export function linesOf(
  names: readonly (At & { key: string })[],
  others: readonly At[],
  people: readonly Person[],
  look: { w: number; size: number; below: number; heads: readonly At[]; widthOf: (s: string) => number },
): Line[] {
  const byKey = new Map(people.map((p) => [p.key, p]));
  // About a letter keeps two full lines apart; a row is whatever a name and the line under it reach, top to bottom. With nobody
  // beside it a line still ends within about a desk and a half, so it reads as its own person's.
  const rooms = roomsOf([...names, ...others], look.w, { gap: look.size * 0.9, row: look.size * 2.2, most: look.size * 13 });
  return names.flatMap((n, i) => {
    const p = byKey.get(n.key);
    const said = p && doingOf(p);
    if (!said) return [];
    const bottom = n.y + look.below + look.size * (LINE.step + LINE.tall);
    const two = !look.heads.some((h) => h.y > n.y && h.y - MARK.over < bottom && Math.abs(h.x - n.x) < rooms[i] / 2 + MARK.wide);
    const one = fitTo(said, rooms[i], look.widthOf);
    const lines = two ? wrapTo(said, rooms[i], look.widthOf) : one ? [one] : [];
    return lines.length ? [{ key: n.key, x: n.x, y: n.y, lines }] : [];
  });
}

/** Where each line of a doing line's text stands: its baseline, for a name `below` tall at `y`. */
export const lineAt = (y: number, below: number, size: number, i: number) => y + below + size * (1.05 + i * LINE.step);

/** The steps done of a task as the bar shows them: the count, and the part done from 0 to 1. Null without a count. */
export function stepsOf(todo: { done: number; total: number } | null | undefined): { text: string; part: number } | null {
  if (!todo || !(todo.total >= 1)) return null;
  const done = Math.min(Math.max(0, todo.done), todo.total);
  return { text: `${done}/${todo.total}`, part: done / todo.total };
}

/** The titles of the tasks `key` owns that are not done: the one being done first, then the oldest first. */
export function queueOf(tasks: readonly Pick<Task, 'id' | 'to' | 'state' | 'title'>[], key: string): string[] {
  const rank = (t: Pick<Task, 'state'>) => (t.state === 'doing' ? 0 : 1);
  return tasks
    .filter((t) => t.to === key && (t.state === 'doing' || t.state === 'queued'))
    .sort((a, b) => rank(a) - rank(b) || a.id - b.id)
    .map((t) => t.title);
}

/** What the papers on a desk say on hover: whose tasks they are, the first `most` by title, and how many more there are. */
export function papersTip(name: string, titles: readonly string[], most = 5): string {
  const more = titles.length - most;
  return [`${name}'s tasks`, ...titles.slice(0, most).map((t) => `· ${cutTo(t, 60)}`), ...(more > 0 ? [`+${more} more`] : [])].join('\n');
}
