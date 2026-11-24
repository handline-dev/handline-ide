import { describe, expect, it } from 'vitest';
import { floorOf, freeAt, pathTo, step } from '#src/town/walk.ts';

// A 4 x 3 m room with a table across its middle, open only along the left wall.
const floor = floorOf(4, 3, [{ x0: 0.6, z0: 1.4, x1: 4, z1: 1.6 }]);
const on = (path: [number, number][], from: [number, number]) =>
  path.every((to, i) => {
    const a = i ? path[i - 1] : from;
    return Array.from(
      { length: 20 },
      (_, k) => [a[0] + ((to[0] - a[0]) * k) / 20, a[1] + ((to[1] - a[1]) * k) / 20] as [number, number],
    ).every((p) => freeAt(floor, p));
  });

describe('walking in an office', () => {
  it('keeps clear of furniture and the walls', () => {
    expect(freeAt(floor, [2, 1.5])).toBe(false);
    expect(freeAt(floor, [2, 1.3])).toBe(false); // within a body's width of the table
    expect(freeAt(floor, [0.05, 1])).toBe(false);
    expect(freeAt(floor, [2, 1])).toBe(true);
  });

  it('walks round the table to a spot behind it, along free floor', () => {
    const from: [number, number] = [3, 0.8];
    const path = pathTo(floor, from, [3, 2.4]);
    expect(path).not.toBeNull();
    expect(path?.at(-1)).toEqual([3, 2.4]);
    expect(Math.min(...(path ?? []).map((p) => p[0]))).toBeLessThan(0.6);
    expect(on(path ?? [], from)).toBe(true);
  });

  it('walks to the nearest free spot when the click lands on furniture', () => {
    const path = pathTo(floor, [2, 0.8], [2, 1.5]);
    expect(path && freeAt(floor, path.at(-1) ?? [0, 0])).toBe(true);
  });

  it('slides along what a held key walks into', () => {
    const at = step(floor, [2, 1.27], [1, 1], 0.05);
    expect(at[1]).toBe(1.27);
    expect(at[0]).toBeGreaterThan(2);
  });
});
