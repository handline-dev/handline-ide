import { describe, expect, it } from 'vitest';
import type { Activity } from '#src/layout/agentList.ts';
import { crowdOf, stepped, type Mover, type Stage } from '#src/town/crowd.ts';
import type { House, Person } from '#src/town/model.ts';
import { changesOf, doorOf, routeOf, seatsOf, type Seat } from '#src/town/moves.ts';
import { roomOf } from '#src/town/room.ts';
import { floorOf, freeAt, type Spot } from '#src/town/walk.ts';

const person = (key: string, activity: Activity, cached = false): Person => ({
  key,
  name: key,
  activity,
  doing: null,
  queue: 0,
  cached,
});
const house = (people: Person[]): House => ({ key: 'w', name: 'w', lead: people[0], people, working: 0, decisions: [] });
/** The room for `people` laid out at three desks a bench, its floor as the founder's (only the solid blocks block it), and its seats. */
const laid = (people: Person[]) => {
  const room = roomOf(house(people), 3);
  const solid = room.blocks.filter((b) => b.solid).map((b) => ({ x0: b.from[0], z0: b.from[2], x1: b.to[0], z1: b.to[2] }));
  const floor = floorOf(room.w, room.d, solid);
  return { room, floor, seats: seatsOf(room, floor), door: doorOf(room) };
};
const stageOf = (l: ReturnType<typeof laid>): Stage => ({ ...l, people: l.room.people, layout: l.room.w });
/** Every step a walker takes along `turns`, a centimetre apart. */
const steps = (turns: Spot[]) =>
  turns.slice(1).flatMap((to, i) => {
    const from = turns[i];
    const n = Math.ceil(Math.hypot(to[0] - from[0], to[1] - from[1]) / 0.01);
    return Array.from({ length: n }, (_, k): Spot => [
      from[0] + ((to[0] - from[0]) * (k + 1)) / n,
      from[1] + ((to[1] - from[1]) * (k + 1)) / n,
    ]);
  });

describe('a person whose place changes', () => {
  const was = laid([person('a', 'working'), person('b', 'resting', true), person('c', 'asleep')]);
  const now = laid([person('a', 'resting'), person('b', 'working'), person('c', 'waking')]);

  it('is seen to change only when its place does: a chair turned round is the same place', () => {
    // a stops working and b starts while their caches live: each stays in its own chair.
    const turned = laid([person('a', 'resting', true), person('b', 'working'), person('c', 'asleep')]);
    expect(changesOf(was.seats, turned.seats)).toEqual([]);
    // Once a's cache runs out it goes to a sofa.
    const sofa = laid([person('a', 'resting'), person('b', 'working'), person('c', 'asleep')]);
    expect(changesOf(turned.seats, sofa.seats).map((c) => c.key)).toEqual(['a']);
  });

  it('walks from its old seat to its new one: up to the floor, along free floor, down into the seat', () => {
    const changes = changesOf(was.seats, now.seats);
    expect(changes.map((c) => c.key).sort()).toEqual(['a', 'c']);
    const a = changes.find((c) => c.key === 'a')!;
    const route = routeOf(now.floor, now.door, a.from, a.to);
    expect(route[0]).toEqual(a.from!.at);
    expect(route.at(-1)).toEqual(a.to!.at);
    // Between getting up and sitting down every step is on free floor.
    const [up, down] = [route.indexOf(a.from!.stand), route.lastIndexOf(a.to!.stand)];
    expect(up).toBe(1);
    expect(down).toBe(route.length - 2);
    expect(steps(route.slice(up, down + 1)).every((p) => freeAt(now.floor, p))).toBe(true);
  });

  it('comes in and goes away through the door', () => {
    const c = changesOf(was.seats, now.seats).find((c) => c.key === 'c')!;
    expect(c.from).toBeNull();
    const coming = routeOf(now.floor, now.door, null, c.to);
    expect(coming.slice(0, 2)).toEqual([now.door.out, now.door.inside]);
    const going = routeOf(now.floor, now.door, c.to, null);
    expect(going.slice(-2)).toEqual([now.door.inside, now.door.out]);
  });

  it('never moves further than its speed allows in a frame, and ends where its walk ends', () => {
    const seat = was.seats.get('a') as Seat;
    const route = routeOf(now.floor, now.door, seat, now.seats.get('a')!);
    let m: Mover = { at: route[0], path: route.slice(1), facing: 'down', walked: 0, row: 0, name: 'a', leaving: false };
    for (let i = 0; i < 2000 && m.path.length; i++) {
      const next = stepped(m, 1 / 60);
      expect(Math.hypot(next.at[0] - m.at[0], next.at[1] - m.at[1])).toBeLessThanOrEqual(1.1 / 60 + 1e-9);
      m = next;
    }
    expect(m.path).toEqual([]);
    expect(m.at).toEqual(route.at(-1));
  });

  it('walks when its desk is laid out elsewhere, as when a teammate before it leaves the team', () => {
    const four = laid(['a', 'b', 'c', 'd'].map((k) => person(k, 'working')));
    const three = laid(['a', 'c', 'd'].map((k) => person(k, 'working')));
    expect(changesOf(four.seats, three.seats).map((c) => c.key)).toEqual(['b', 'c', 'd']);
  });

  it('goes out of the door looking as it did, though gone from the team', () => {
    const crowd = crowdOf();
    const two = laid([{ ...person('a', 'working'), name: 'Ana' }, person('b', 'working')]);
    crowd.place(stageOf(two));
    crowd.place(stageOf(laid([person('b', 'working')])));
    const a = crowd.get().get('a');
    expect(a && { name: a.name, leaving: a.leaving, end: a.path.at(-1) }).toEqual({ name: 'Ana', leaving: true, end: two.door.out });
    crowd.place(null);
  });
});
