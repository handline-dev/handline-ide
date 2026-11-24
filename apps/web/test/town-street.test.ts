import { describe, expect, it } from 'vitest';
import type { Activity } from '#src/layout/agentList.ts';
import { cityOf, HQ, ROAD } from '#src/town/city.ts';
import type { House, Person } from '#src/town/model.ts';
import { cityRoom, U } from '#src/town/street.ts';

const person = (key: string, activity: Activity): Person => ({ key, name: key, activity, doing: null, queue: 0 });
const house = (key: string, acts: Activity[]): House => {
  const people = acts.map((a, i) => person(`${key}${i}`, a));
  return { key, name: key, lead: people[0], people, working: acts.filter((a) => a === 'working').length, decisions: [] };
};

describe('cityRoom', () => {
  const city = cityOf([house('a', ['working', 'resting', 'working']), house('b', ['asleep', 'asleep'])], 1.6);
  const room = cityRoom(city, 3);

  it('gives the HQ and every house a building with a door, the founder starting at the HQ', () => {
    expect(room.doors.map((d) => d.key).sort()).toEqual(['a', 'b', HQ].sort());
    expect(room.entry).toEqual(room.doors.find((d) => d.key === HQ)?.at);
    expect(room.blocks.filter((b) => b.tag?.startsWith('house:')).map((b) => b.tag)).toHaveLength(3);
  });

  it('lights a window for each agent at work and no other', () => {
    expect(room.blocks.filter((b) => b.kind === 'lit' && !b.colour)).toHaveLength(2); // lamps and car lights carry their own colour
  });

  it('keeps the buildings and street things solid for the founder to walk round, and cars off the crossings', () => {
    expect(room.blocks.filter((b) => b.solid && b.tag?.startsWith('house:'))).toHaveLength(3);
    const crossings = city.avenues.map((a) => [(a + ROAD) * U, (a + ROAD) * U + 2.4]);
    const cars = room.blocks.filter((b) => b.solid && b.kind === 'side' && b.to[0] - b.from[0] > 1.4);
    for (const c of cars) for (const [x0, x1] of crossings) expect(c.to[0] < x0 || c.from[0] > x1).toBe(true);
  });
});
