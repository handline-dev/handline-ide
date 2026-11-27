import { describe, expect, it } from 'vitest';
import { fadeOf, moreOf } from '#src/town/pan.ts';

describe('moreOf', () => {
  it('names the edges with more of the room beyond them', () => {
    expect(moreOf(0, 0, 100, 100, 100, 100)).toEqual([false, false, false, false]);
    expect(moreOf(0, 0, 100, 100, 300, 200)).toEqual([false, true, false, true]);
    expect(moreOf(200, 100, 100, 100, 300, 200)).toEqual([true, false, true, false]);
  });
});

describe('fadeOf', () => {
  it('masks nothing when the room fits, and fades only the cut edges', () => {
    expect(fadeOf([false, false, false, false])).toEqual({});
    const mask = String(fadeOf([false, true, false, true]).maskImage);
    expect(mask).toContain('to left, transparent, black 40px');
    expect(mask).toContain('to right, transparent, black 0px');
  });
});
