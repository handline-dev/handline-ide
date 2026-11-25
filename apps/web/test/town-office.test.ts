import { describe, expect, it } from 'vitest';
import { topsOf, touches } from '#src/town/office.ts';
import type { Picture } from '#src/town/shade.ts';

/** A picture `w` wide whose rows from `from[x]` down are covered in column x. */
const wall = (from: number[], h: number): Picture => {
  const w = from.length;
  const id = new Int32Array(w * h);
  for (let y = 0; y < h; y++) for (let x = 0; x < w; x++) if (y >= from[x]) id[y * w + x] = 1;
  return { w, h, rgba: new Uint8ClampedArray(w * h * 4), id, depth: new Float32Array(w * h) };
};

describe('the front wall layer', () => {
  it('finds the topmost covered row of each column, the height where none is', () => {
    expect([...topsOf(wall([6, 4, 10, 4], 10))]).toEqual([6, 4, 10, 4]);
  });

  it('is touched by a box whose bottom reaches below the wall top in a column it spans', () => {
    const top = topsOf(wall([6, 6, 10, 10, 10], 10));
    expect(touches(top, { x: 0, y: 0, w: 2, h: 6 })).toBe(false);
    expect(touches(top, { x: 0, y: 0, w: 2, h: 7 })).toBe(true);
    // A box over columns the wall leaves open never touches it, however low.
    expect(touches(top, { x: 2, y: 0, w: 3, h: 10 })).toBe(false);
    // Partly off the picture: only its columns on the picture count.
    expect(touches(top, { x: -3, y: 0, w: 4, h: 8 })).toBe(true);
    expect(touches(top, { x: 4.5, y: 0, w: 9, h: 8 })).toBe(false);
  });
});
