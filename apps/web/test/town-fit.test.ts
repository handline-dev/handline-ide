import { describe, expect, it } from 'vitest';
import type { Activity } from '#src/layout/agentList.ts';
import type { House, Person } from '#src/town/model.ts';
import { fittedRoom } from '#src/town/fit.ts';
import { viewOf } from '#src/town/view.ts';

const person = (key: string, activity: Activity): Person => ({ key, name: key, activity, doing: null, queue: 0, cached: false });
const team = (n: number, activity: Activity = 'working'): House => {
  const people = Array.from({ length: n }, (_, i) => person(`p${i}`, activity));
  return { key: 'w', name: 'w', lead: people[0] ?? null, people, working: 0, decisions: [] };
};
/** Where a desk is drawn on the picture, in pixels. */
const deskAt = (h: House, key: string) => {
  const room = fittedRoom(h);
  const [x, y, z] = room.blocks.find((b) => b.tag === `desk:${key}`)!.from;
  return viewOf(room).toPixel(x, y, z);
};

describe('fittedRoom', () => {
  it('draws every team size as a 4:3 picture', () => {
    for (const n of [0, 1, 3, 8, 15, 30, 33, 61])
      for (const activity of ['working', 'resting'] as const) {
        const v = viewOf(fittedRoom(team(n, activity)));
        expect(v.w / v.h).toBeCloseTo(4 / 3, 1);
      }
  });

  it('draws the desks in the same place whoever rests on the sofas', () => {
    for (const n of [5, 20, 30, 33, 61]) expect(deskAt(team(n, 'resting'), 'p1')).toEqual(deskAt(team(n), 'p1'));
  });
});
