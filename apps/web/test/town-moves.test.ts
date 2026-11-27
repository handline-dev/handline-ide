import { describe, expect, it } from 'vitest';
import type { Activity } from '#src/layout/agentList.ts';
import { crowdOf, stepped, talking, type Mover, type Stage } from '#src/town/crowd.ts';
import { givenWork } from '#src/town/errands.ts';
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
/** The stage of a laid room; one talks at each desk from right beside the front of its chair. */
const stageOf = (l: ReturnType<typeof laid>): Stage => ({
  ...l,
  desks: new Map([...l.seats].map(([k, s]) => [k, [s.at[0] - 0.4, s.at[1]] as Spot])),
  people: l.room.people,
  layout: l.room.w,
});
/** A mover walked on in frames of `dt` until `done` says stop, at most a minute. */
const walk = (m: Mover, done: (m: Mover) => boolean, dt = 1 / 30) => {
  for (let i = 0; i < 1800 && !done(m); i++) m = stepped(m, dt);
  return m;
};
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

  it('is seen to change only when its place does', () => {
    // a stops working and b starts while their caches live: each stays in its own chair.
    const resting = laid([person('a', 'resting', true), person('b', 'working'), person('c', 'asleep')]);
    expect(changesOf(was.seats, resting.seats)).toEqual([]);
    // Once a's cache runs out it goes to a sofa.
    const sofa = laid([person('a', 'resting'), person('b', 'working'), person('c', 'asleep')]);
    expect(changesOf(resting.seats, sofa.seats).map((c) => c.key)).toEqual(['a']);
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

describe('one agent giving another work', () => {
  it('counts a task just opened by one agent for another, not a draft, an old one or one of its own', () => {
    const t = { from: 'a', to: 'b', state: 'queued', created: new Date(1_000_000).toISOString() };
    expect(givenWork(t, 1_010_000)).toBe(true);
    expect(givenWork({ ...t, state: 'draft' }, 1_010_000)).toBe(false);
    expect(givenWork({ ...t, to: 'a' }, 1_010_000)).toBe(false);
    expect(givenWork(t, 1_000_000 + 5 * 60_000)).toBe(false);
    expect(givenWork({ ...t, state: 'done' }, 1_010_000)).toBe(false);
    // Made before the screen opened: only changed now.
    expect(givenWork(t, 1_010_000, 1_008_000)).toBe(false);
    expect(givenWork(t, 1_010_000, 1_002_000)).toBe(true);
  });

  it('walks over to the other desk, talks there, and walks back to its seat', () => {
    const l = laid([person('a', 'working'), person('b', 'working'), person('c', 'working')]);
    const crowd = crowdOf();
    crowd.place(stageOf(l));
    crowd.errand('a', 'c');
    let m = crowd.get().get('a')!;
    expect(m.at).toEqual(l.seats.get('a')!.at);
    m = walk(m, talking);
    expect(m.at[0]).toBeCloseTo(l.seats.get('c')!.at[0] - 0.4, 1);
    const talked = walk(m, (x) => !talking(x));
    expect(talked.path.length).toBeGreaterThan(0);
    const home = walk(talked, (x) => !x.path.length);
    expect(home.at).toEqual(l.seats.get('a')!.at);
    crowd.place(null);
  });

  it('goes on with its errand when something else in the office changes', () => {
    const crowd = crowdOf();
    crowd.place(stageOf(laid([person('a', 'working'), person('b', 'working'), person('c', 'working')])));
    crowd.errand('a', 'c');
    crowd.place(stageOf(laid([person('a', 'working'), person('b', 'resting'), person('c', 'working')])));
    expect(crowd.get().get('a')?.errand).toBeDefined();
    crowd.place(null);
  });

  it('goes home when the other leaves the office mid-errand', () => {
    const crowd = crowdOf();
    const three = laid([person('a', 'working'), person('b', 'working'), person('c', 'working')]);
    crowd.place(stageOf(three));
    crowd.errand('a', 'c');
    crowd.place(stageOf(laid([person('a', 'working'), person('b', 'working')])));
    const m = crowd.get().get('a');
    expect(m && { errand: m.errand, end: m.path.at(-1) }).toEqual({ errand: undefined, end: three.seats.get('a')!.at });
    crowd.place(null);
  });
});
