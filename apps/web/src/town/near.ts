// What the nearby window shows (NearWindow.tsx): the thing the founder has stopped beside in an office, a person (its chat), the papers
// on a desk (its tasks) or the founder's own desk (the decisions). Never chosen while walking; kept while the founder is still close to
// it, so the window does not blink while it turns or takes a step; gone once it walks off.
import type { Person } from '#src/town/model.ts';
import type { Spot } from '#src/town/walk.ts';

/** A thing the founder can stand beside: where it stands for it (`at`), and what that opens. */
export type Near = { key: string; at: Spot; opens: string; title: string; person?: Person };

/** How close to a thing's spot the founder stops to be beside it, and how far it walks off before it has left it, in metres. */
export const BESIDE = 0.35;
export const LEFT = 0.6;

const apart = (a: Spot, b: Spot) => Math.hypot(a[0] - b[0], a[1] - b[1]);

/** What the founder at `me` is beside now, among `things`, having been beside `was`. Two things can share a spot (a desk's side is the
 *  next desk's papers): the one clicked to walk there (`sought`) wins, else the one listed first. */
export function nearOf(me: { at: Spot; moving: boolean } | null, things: Near[], was: Near | null, sought?: string | null): Near | null {
  if (!me) return null;
  const kept = was && things.find((t) => t.key === was.key);
  const stays = kept && apart(me.at, kept.at) <= LEFT ? kept : null;
  if (me.moving) return stays;
  const asked = sought && things.find((t) => t.key === sought);
  if (asked && apart(me.at, asked.at) <= BESIDE) return asked;
  let best: Near | null = null;
  let least = BESIDE;
  for (const t of things) {
    const d = apart(me.at, t.at);
    if (d < least || (d === least && !best)) [best, least] = [t, d];
  }
  return best ?? stays;
}

type Placed = { person: Person; at: Spot };

/** The things of an office drawn as `cast` to stand beside: each person at its desk and where it sits elsewhere (its chat), the papers on
 *  each desk (its tasks) and the founder's desk (`desk`); `people` the newest of them. */
export function thingsOf(
  cast: { desks: Placed[]; spots: Placed[]; papers: Placed[]; mine?: { seat: Spot } },
  people: Person[],
  desk: { opens: string; title: string },
): Near[] {
  const chat =
    (where: string) =>
    (s: Placed): Near => {
      const p = people.find((q) => q.key === s.person.key) ?? s.person;
      return { key: `${where}:${p.key}`, at: s.at, opens: `chat:${p.key}`, title: p.name, person: p };
    };
  return [
    ...cast.desks.map(chat('desk')),
    ...cast.spots.map(chat('spot')),
    ...cast.papers.map((s) => ({
      key: `papers:${s.person.key}`,
      at: s.at,
      opens: `tasks|${s.person.key}`,
      title: `Tasks · ${s.person.name}`,
    })),
    ...(cast.mine ? [{ key: 'mine', at: cast.mine.seat, ...desk }] : []),
  ];
}
