import { describe, expect, it } from 'vitest';
import { SITE } from '#src/town/city.ts';
import { findIn, NEW_TEAM, rankOf, rowsFor } from '#src/town/find.ts';

const team = (key: string, name: string, people: string[]) => ({ key, name, people: people.map((p) => ({ key: `${key}.${p}`, name: p })) });
const town = [
  team('hq', 'Your HQ', []),
  team('w1', 'shop', ['lead', 'sasha', 'builder']),
  team('w2', 'web-shell', ['lead', 'ash']),
  team('w3', 'Docs', ['marsha']),
];

describe('finding a team or a person', () => {
  it('ranks a name that starts with the word over one a word of which does, over one that only holds it', () => {
    expect(rankOf('shop', 'sh')).toBe(0);
    expect(rankOf('web-shell', 'sh')).toBe(1);
    expect(rankOf('Your HQ', 'hq')).toBe(1);
    expect(rankOf('marsha', 'sh')).toBe(2);
    expect(rankOf('Docs', 'sh')).toBeNull();
  });

  it('lists the better matches first, a team before a person who matches as well', () => {
    expect(findIn(town, 'sh').map((f) => f.name)).toEqual(['shop', 'web-shell', 'ash', 'marsha', 'sasha']);
    expect(findIn(town, 'do')).toEqual([{ house: 'w3', name: 'Docs' }]);
  });

  it('names the team of a person, and the building to go into', () => {
    expect(findIn(town, 'sasha')).toEqual([{ house: 'w1', person: 'w1.sasha', name: 'sasha', team: 'shop' }]);
    // Two people of one name are told apart by their teams, in the town's order.
    expect(findIn(town, 'lead').map((f) => [f.person, f.team])).toEqual([
      ['w1.lead', 'shop'],
      ['w2.lead', 'web-shell'],
    ]);
  });

  it('takes any case and the spaces round the word, and finds nothing for an empty one', () => {
    expect(findIn(town, '  DOCS ').map((f) => f.house)).toEqual(['w3']);
    expect(findIn(town, 'your')).toEqual([{ house: 'hq', name: 'Your HQ' }]);
    expect(findIn(town, '')).toEqual([]);
    expect(findIn(town, '   ')).toEqual([]);
    expect(findIn(town, 'zzz')).toEqual([]);
  });

  it('lists no more than the limit', () => {
    const many = Array.from({ length: 30 }, (_, i) => team(`t${i}`, `team ${i}`, ['dev']));
    expect(findIn(many, 'team')).toHaveLength(8);
    expect(findIn(many, 'dev', 3)).toHaveLength(3);
  });

  it('offers a new team where there is nothing to list: before a word is typed, and when nothing matches', () => {
    expect(rowsFor(town, '')).toEqual([NEW_TEAM]);
    expect(rowsFor(town, '  ')).toEqual([NEW_TEAM]);
    expect(rowsFor(town, 'zzz')).toEqual([NEW_TEAM]);
    expect(NEW_TEAM.house).toBe(SITE);
    // With matches it lists those alone.
    expect(rowsFor(town, 'do')).toEqual([{ house: 'w3', name: 'Docs' }]);
  });
});
