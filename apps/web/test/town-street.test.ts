import { describe, expect, it } from 'vitest';
import type { Activity } from '#src/layout/agentList.ts';
import { cityOf, HQ, ROAD, SITE, type City } from '#src/town/city.ts';
import type { House, Person } from '#src/town/model.ts';
import { STOPPED_GLOW } from '#src/town/palette.ts';
import { cityRoom, U } from '#src/town/street.ts';

const person = (key: string, activity: Activity): Person => ({ key, name: key, activity, doing: null, queue: 0, cached: false });
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
    expect(room.blocks.filter((b) => b.kind === 'wall' && b.tag?.startsWith('house:')).map((b) => b.tag)).toHaveLength(3);
  });

  it('lights a window for each agent at work and no other', () => {
    expect(room.blocks.filter((b) => b.kind === 'lit' && !b.colour)).toHaveLength(2); // lamps and car lights carry their own colour
  });

  it('glows a window red for an agent stopped on a fault, apart from the lit ones', () => {
    const faulty = cityRoom(cityOf([house('c', ['working', 'error', 'asleep'])], 1.6), 0);
    expect(faulty.blocks.filter((b) => b.kind === 'lit' && b.colour === STOPPED_GLOW)).toHaveLength(1);
    expect(faulty.blocks.filter((b) => b.kind === 'lit' && !b.colour)).toHaveLength(1);
    expect(room.blocks.filter((b) => b.colour === STOPPED_GLOW)).toHaveLength(0);
  });

  it('keeps the buildings and street things solid for the founder to walk round, and cars off the crossings', () => {
    expect(room.blocks.filter((b) => b.solid && b.tag?.startsWith('house:'))).toHaveLength(3);
    const crossings = city.avenues.map((a) => [(a + ROAD) * U, (a + ROAD) * U + 2.4]);
    const cars = room.blocks.filter((b) => b.solid && b.part?.startsWith('car:'));
    expect(cars.length).toBeGreaterThan(0);
    for (const c of cars) for (const [x0, x1] of crossings) expect(c.to[0] < x0 || c.from[0] > x1).toBe(true);
  });
});

describe('the building site for a new team', () => {
  const blockOf = (city: City, a: { x: number; y: number }) =>
    city.blocks.findIndex((b) => a.x >= b.x && a.x < b.x + b.w && a.y >= b.y && a.y < b.y + b.h);

  it('is the first free lot after the last building, in its block when that has room, and no park grows over it', () => {
    const city = cityOf([house('a', ['working', 'resting', 'working'])], 1.6);
    const last = city.lots[city.lots.length - 1];
    expect(last.key).toBe('a');
    expect(city.site.x).toBe(last.x + last.w);
    expect(blockOf(city, city.site)).toBe(blockOf(city, last));
    const beside = city.parks.filter((p) => blockOf(city, p) === blockOf(city, city.site));
    expect(beside).toHaveLength(1);
    expect(beside[0].x).toBe(city.site.x + city.site.w);
  });

  it('is a lot of its own, in the next block, when the last block is full', () => {
    // The HQ and two buildings leave two tiles of their block: less than the site takes.
    const city = cityOf([house('a', ['working', 'resting', 'working']), house('b', ['asleep', 'asleep'])], 1.6);
    const last = city.lots[city.lots.length - 1];
    const [mine, theirs] = [blockOf(city, city.site), blockOf(city, last)];
    expect(mine).toBeGreaterThanOrEqual(0);
    expect(mine).not.toBe(theirs);
    expect(city.site.x).toBe(city.blocks[mine].x + 1);
    expect(city.lots.some((l) => blockOf(city, l) === mine)).toBe(false);
  });

  it('is with one team or none, and is no workspace building', () => {
    for (const houses of [[], [house('a', ['working'])]]) {
      const city = cityOf(houses, 1.6);
      expect(blockOf(city, city.site)).toBeGreaterThanOrEqual(0);
      expect(city.lots.map((l) => l.key)).not.toContain(SITE);
    }
  });

  it('is built with a board to name it and a spot before it to stand at, and has no door', () => {
    const city = cityOf([house('a', ['working', 'resting', 'working'])], 1.6);
    const room = cityRoom(city, 0);
    expect(room.doors.map((d) => d.key)).not.toContain(SITE);
    expect(room.site?.key).toBe(SITE);
    expect(room.blocks.filter((b) => b.tag === `sign:${SITE}`)).toHaveLength(1);
    expect(room.blocks.filter((b) => b.tag === `house:${SITE}`)).toHaveLength(1);
    const [x, z] = room.site?.at ?? [0, 0];
    expect(x).toBeGreaterThan(city.site.x * U);
    expect(x).toBeLessThan((city.site.x + city.site.w) * U);
    expect(z).toBeGreaterThan(room.site?.front ?? Infinity);
    // Nothing solid stands where the founder is put.
    const solid = room.blocks.filter((b) => b.solid && b.from[0] < x && b.to[0] > x && b.from[2] < z && b.to[2] > z);
    expect(solid).toHaveLength(0);
  });
});
