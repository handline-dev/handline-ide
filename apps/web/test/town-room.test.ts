import { describe, expect, it } from 'vitest';
import type { Activity } from '#src/layout/agentList.ts';
import type { House, Person } from '#src/town/model.ts';
import { figuresOf } from '#src/town/people.ts';
import { roomOf } from '#src/town/room.ts';

const person = (key: string, activity: Activity): Person => ({ key, name: key, activity, doing: null });
const house = (people: Person[]): House => ({ key: 'w', name: 'w', lead: people[0], people, working: 0, decisions: [] });

describe('roomOf', () => {
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
    expect(r.things.filter((t) => t.tag?.startsWith('desk:')).map((t) => t.tag)).toEqual(
      ['a', 'b', 'c', 'd', 'e', 'f'].map((k) => `desk:${k}`),
    );
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
