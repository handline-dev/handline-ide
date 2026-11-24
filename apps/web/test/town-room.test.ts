import { describe, expect, it } from 'vitest';
import type { Activity } from '#src/layout/agentList.ts';
import type { House, Person } from '#src/town/model.ts';
import { figuresOf } from '#src/town/people.ts';
import { roomOf } from '#src/town/room.ts';

const person = (key: string, activity: Activity): Person => ({ key, name: key, activity, doing: null, queue: 0 });
const house = (people: Person[]): House => ({ key: 'w', name: 'w', lead: people[0], people, working: 0, decisions: [] });

describe('roomOf', () => {
  it("stacks a paper per queued task on each desk and per decision on the founder's, up to a limit", () => {
    const h = house([{ ...person('a', 'working'), queue: 3 }, { ...person('b', 'resting'), queue: 52 }, person('c', 'asleep')]);
    const r = roomOf({ ...h, decisions: [{ id: '1', task: 1, title: 't', agent: 'a', house: 'w' }] });
    const count = (tag: string) => r.blocks.filter((b) => b.kind === 'paper' && b.tag === tag).length;
    expect([count('desk:a'), count('desk:b'), count('desk:c'), count('desk:you')]).toEqual([3, 14, 0, 1]);
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
      ]),
    );
    expect(r.things.filter((t) => t.model === 'desk').map((t) => t.tag)).toEqual([
      ...['a', 'b', 'c', 'd', 'e', 'f'].map((k) => `desk:${k}`),
      'desk:you',
    ]);
    // At their desks: working, waiting for the founder and on hold; the idle one on a sofa, the starting one at the door, the asleep nowhere.
    const where = Object.fromEntries(r.places.map((p) => [p.person.key, p.how]));
    expect(where).toEqual({ a: 'desk', b: 'desk', f: 'desk', d: 'sofa', e: 'door' });
    // Only the working and the waiting agents' screens are lit.
    expect(r.things.filter((t) => t.model === 'computerScreen' && t.lit)).toHaveLength(2);
  });

  it('adds a row of sofas for every four idle agents and deepens the room for them', () => {
    const idle = (n: number) => roomOf(house(Array.from({ length: n }, (_, i) => person(`p${i}`, 'resting'))));
    const sofas = (n: number) => idle(n).things.filter((t) => t.model === 'loungeSofa').length;
    expect([sofas(1), sofas(4), sofas(5), sofas(9)]).toEqual([2, 2, 4, 6]);
    expect(idle(9).d).toBeGreaterThan(idle(4).d);
  });

  it('gives teammates different figures while there are enough', () => {
    expect(new Set(figuresOf(['a', 'b', 'c', 'd', 'e', 'f'].map((k) => person(k, 'working'))).values()).size).toBe(6);
  });
});
