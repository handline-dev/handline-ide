import { describe, expect, it } from 'vitest';
import { outlineDots } from '#src/town/outline.ts';

/** An image of `w` by `h` with a solid rectangle from (x0, y0) to (x1, y1), exclusive. */
const rect = (w: number, h: number, [x0, y0, x1, y1]: number[]) =>
  Array.from({ length: w * h }, (_, i) => (i % w >= x0 && i % w < x1 && Math.floor(i / w) >= y0 && Math.floor(i / w) < y1 ? 255 : 0));

describe('outlineDots', () => {
  it('dots every other cell of the ring just outside the figure, corners included', () => {
    // A 2 by 2 cell figure in a 6 by 6 cell grid: its ring is the 12 cells round it, every other one dotted.
    const dots = outlineDots(rect(12, 12, [4, 4, 8, 8]), 12, 12, 2);
    expect(dots).toHaveLength(6);
    for (const [x, y] of dots) {
      expect((x + y) % 2).toBe(0);
      expect(x >= 1 && x <= 4 && y >= 1 && y <= 4 && !(x >= 2 && x <= 3 && y >= 2 && y <= 3)).toBe(true);
    }
  });

  it('counts a cell in the figure when half its pixels are, and an empty image has no outline', () => {
    // Half of cell (1, 1) is in: it is figure, not ring.
    const half = outlineDots(rect(6, 6, [2, 2, 3, 4]), 6, 6, 2);
    expect(half.some(([x, y]) => x === 1 && y === 1)).toBe(false);
    expect(half).toContainEqual([0, 0]);
    expect(outlineDots(rect(6, 6, [0, 0, 0, 0]), 6, 6, 2)).toEqual([]);
  });
});
