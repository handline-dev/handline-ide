import { describe, expect, it } from 'vitest';
import type { Activity } from '#src/layout/agentList.ts';
import type { House, Person } from '#src/town/model.ts';
import { figuresOf } from '#src/town/people.ts';
import { inFront, roomOf, WALL_T, type Room } from '#src/town/room.ts';

const person = (key: string, activity: Activity): Person => ({ key, name: key, activity, doing: null, queue: 0, cached: false });
const house = (people: Person[]): House => ({ key: 'w', name: 'w', lead: people[0], people, working: 0, decisions: [] });
/** A desk's top is the one block of a desk with a tag. */
const desks = (r: Room) => r.blocks.filter((b) => b.tag && b.part?.startsWith('desk:')).map((b) => b.tag);

describe('roomOf', () => {
  it("stacks a paper per queued task on each desk and per decision on the founder's, up to a limit", () => {
    const h = house([{ ...person('a', 'working'), queue: 3 }, { ...person('b', 'resting'), queue: 52 }, person('c', 'asleep')]);
    const r = roomOf({ ...h, decisions: [{ id: '1', task: 1, title: 't', agent: 'a', house: 'w' }] });
    const count = (tag: string) => r.blocks.filter((b) => b.kind === 'paper' && b.tag === tag).length;
    expect([count('papers:a'), count('papers:b'), count('papers:c'), count('papers:you')]).toEqual([3, 14, 0, 1]);
  });

  it('gives every agent a desk and puts each where its state says', () => {
    const r = roomOf(
      house([
        person('a', 'working'),
        person('b', 'you'),
        person('c', 'asleep'),
        person('d', 'resting'),
        person('e', 'waking'),
        person('f', 'held'),
        { ...person('g', 'resting'), cached: true },
      ]),
    );
    expect(desks(r)).toEqual([...['a', 'b', 'c', 'd', 'e', 'f', 'g'].map((k) => `desk:${k}`), 'desk:hire', 'desk:you']);
    // At their desks: working, waiting for the founder and on hold; the idle one whose prompt cache lives at its desk turned to face the
    // room, the one whose cache ran out on a sofa; the starting one at the door, the asleep nowhere.
    const where = Object.fromEntries(r.places.map((p) => [p.person.key, p.how]));
    expect(where).toEqual({ a: 'desk', b: 'desk', f: 'desk', g: 'turned', d: 'sofa', e: 'door' });
    // The turned one's chair has its back against the desk, the others' toward the camera.
    const back = (k: string) => {
      const parts = r.blocks.filter((b) => b.part === r.blocks.find((c) => c.tag === `chair:${k}`)?.part);
      const seat = parts.find((b) => b.solid)!;
      return parts.find((b) => b.to[1] === 0.6)!.from[2] < seat.from[2] + 0.05 ? 'desk' : 'camera';
    };
    expect([back('a'), back('g')]).toEqual(['camera', 'desk']);
    // Only the working and the waiting agents' screens are lit.
    expect(r.blocks.filter((b) => b.kind === 'lit')).toHaveLength(2);
  });

  it('keeps one empty desk after the last agent for hiring, and moves nobody for it', () => {
    const team = (n: number) => house(Array.from({ length: n }, (_, i) => person(`p${i}`, 'working')));
    const top = (r: Room, tag: string) => r.blocks.find((b) => b.tag === tag && b.part?.startsWith('desk:'))!;
    const at = (r: Room, tag: string) => [top(r, tag).from[0], top(r, tag).to[2]].map((v) => +v.toFixed(3));
    // Three desks a bench: with five agents the empty desk is the sixth, right of the fifth on the second bench.
    const five = roomOf(team(5), 3);
    expect(at(five, 'desk:hire')).toEqual([+(at(five, 'desk:p4')[0] + 0.734).toFixed(3), at(five, 'desk:p4')[1]]);
    // A dark screen and an empty chair at it, and no papers.
    expect(five.places.map((p) => p.person.key)).toEqual(['p0', 'p1', 'p2', 'p3', 'p4']);
    expect(five.blocks.filter((b) => b.kind === 'lit')).toHaveLength(5);
    expect(five.blocks.some((b) => b.tag === 'papers:hire')).toBe(false);
    expect(five.blocks.some((b) => b.tag === 'desk:hire' && b.part?.startsWith('chair:'))).toBe(true);
    // With the last bench full it starts a bench of its own: the room is a row deeper, as wide as before, every agent where it was.
    const six = roomOf(team(6), 3);
    expect(at(six, 'desk:hire')).toEqual([at(six, 'desk:p0')[0], +(at(six, 'desk:p3')[1] + 1.75).toFixed(3)]);
    expect([six.w, six.d > five.d]).toEqual([five.w, true]);
    for (const k of ['p0', 'p1', 'p2', 'p3', 'p4']) expect(at(six, `desk:${k}`)).toEqual(at(five, `desk:${k}`));
    // The bench is as wide as the team alone makes it, the empty desk not counted: four agents sit two by two, with or without it.
    expect(at(roomOf(team(4)), 'desk:p3')).toEqual(at(roomOf(team(4), 2), 'desk:p3'));
    expect(at(roomOf(team(4)), 'desk:hire')[0]).toBe(at(roomOf(team(4)), 'desk:p0')[0]);
    // A team of nobody still has it, first on the first bench.
    expect(desks(roomOf(house([])))).toEqual(['desk:hire', 'desk:you']);
  });

  it('adds a row of sofas for every four idle agents and deepens the room for them', () => {
    const idle = (n: number) => roomOf(house(Array.from({ length: n }, (_, i) => person(`p${i}`, 'resting'))));
    const sofas = (n: number) =>
      new Set(
        idle(n)
          .blocks.map((b) => b.part)
          .filter((p) => p?.startsWith('sofa:')),
      ).size;
    expect([sofas(1), sofas(4), sofas(5), sofas(9)]).toEqual([2, 2, 4, 6]);
    expect(idle(9).d).toBeGreaterThan(idle(4).d);
  });

  it("puts the door in the front wall, the founder's desk beside it, and the sofas behind a wall open at the front", () => {
    const r = roomOf(house([person('a', 'working'), person('b', 'resting'), person('c', 'working')]));
    const door = r.blocks.find((b) => b.kind === 'door')!;
    // The founder comes up from below: the door stands in the front wall, the walk in starts just inside it.
    expect(door.to[2]).toBeGreaterThan(r.d - WALL_T);
    expect(r.entry[1]).toBeGreaterThan(r.d - 0.6);
    const front = r.blocks.filter(inFront);
    expect(front.filter((b) => b.solid).every((b) => b.from[2] >= r.d - WALL_T && b.to[2] === r.d)).toBe(true);
    // The front wall stands across the whole front but for the doorway, which the founder walks through.
    const spans = front.filter((b) => b.solid).map((b) => [b.from[0], b.to[0]]);
    expect(spans).toEqual([
      [-WALL_T, door.from[0]],
      [door.to[0], r.w + WALL_T],
    ]);
    // The founder's desk ends just left of the door, in front of the agents' desks and clear of the front wall's cover.
    const mine = r.blocks.find((b) => b.tag === 'desk:you' && b.part?.startsWith('desk:'))!;
    expect(door.from[0] - mine.to[0]).toBeGreaterThan(0);
    expect(door.from[0] - mine.to[0]).toBeLessThan(0.3);
    const others = r.blocks.filter((b) => b.tag?.startsWith('desk:') && b.tag !== 'desk:you' && b.part?.startsWith('desk:'));
    expect(Math.max(...others.map((b) => b.to[0]))).toBeLessThan(mine.from[0]);
    expect(mine.to[2]).toBeLessThan(r.d - 1.6);
    // The rest room's wall stands between the door and every sofa, from the back wall to short of the front, and the founder walks round it.
    const wall = r.blocks.find((b) => b.part === 'wall:rest')!;
    expect(wall.solid).toBe(true);
    expect([wall.from[2], wall.to[2] < r.d - 1.5]).toEqual([0, true]);
    expect(wall.from[0]).toBeGreaterThan(door.to[0]);
    for (const s of r.blocks.filter((b) => b.part?.startsWith('sofa:'))) expect(s.from[0]).toBeGreaterThan(wall.to[0]);
    expect(r.places.find((p) => p.person.key === 'b')?.x).toBeGreaterThan(wall.to[0]);
  });

  it('gives teammates different figures while there are enough', () => {
    expect(new Set(figuresOf(['a', 'b', 'c', 'd', 'e', 'f'].map((k) => person(k, 'working'))).values()).size).toBe(6);
  });

  it("stands the HQ's pinned agents together, whatever they are doing, with no desks but the founder's", () => {
    const h = house([person('a', 'working'), person('b', 'resting'), person('c', 'waking'), person('d', 'asleep')]);
    const r = roomOf({ ...h, hq: true });
    expect(r.places.map((p) => [p.person.key, p.how])).toEqual(['a', 'b', 'c', 'd'].map((k) => [k, 'door']));
    expect(desks(r)).toEqual(['desk:you']);
  });
});
