// Finding a team or a person in the town by name (Find.tsx is the field): what a typed word matches, best first, and adding a new team
// where there is nothing to list.
import { SITE } from '#src/town/city.ts';
import type { House } from '#src/town/model.ts';

/** A match: a team's building, or a person, who is found in its team's building (`team` is that team's name). */
export type Found = { house: string; person?: string; name: string; team?: string };
type Listed = Pick<House, 'key' | 'name'> & { people: readonly { key: string; name: string }[] };

/** How well a name matches a typed word: 0 it starts with it, 1 one of its words does, 2 it holds it; null when it does not. */
export function rankOf(name: string, word: string): number | null {
  const n = name.toLowerCase();
  if (n.startsWith(word)) return 0;
  if (n.split(/[^\p{L}\p{N}]+/u).some((part) => part.startsWith(word))) return 1;
  return n.includes(word) ? 2 : null;
}

/** The teams and people `query` matches, whatever its case: the better matches first, a team before a person who matches as well, then
 *  by name; at most `limit`. Nothing for an empty query. */
export function findIn(houses: readonly Listed[], query: string, limit = 8): Found[] {
  const word = query.trim().toLowerCase();
  if (!word) return [];
  const all: Found[] = houses.flatMap((h) => [
    { house: h.key, name: h.name },
    ...h.people.map((p) => ({ house: h.key, person: p.key, name: p.name, team: h.name })),
  ]);
  return all
    .flatMap((found) => {
      const rank = rankOf(found.name, word);
      return rank === null ? [] : [{ found, rank }];
    })
    .sort((a, b) => a.rank - b.rank || Number(!!a.found.person) - Number(!!b.found.person) || a.found.name.localeCompare(b.found.name))
    .slice(0, limit)
    .map((m) => m.found);
}

/** The row that adds a team: it goes to the building site. */
export const NEW_TEAM: Found = { house: SITE, name: 'New team…' };

/** What the field lists for `query`: its matches, or the row that adds a team when nothing is typed yet or nothing matches. */
export function rowsFor(houses: readonly Listed[], query: string): Found[] {
  const found = findIn(houses, query);
  return found.length ? found : [NEW_TEAM];
}
