import { describe, expect, it } from 'vitest';
import { walkColumn } from '#src/town/people.ts';

describe('the walking sheet', () => {
  it('walks standing, left foot, standing, right foot, then again', () => {
    expect([0, 1, 2, 3, 4, 5].map((n) => walkColumn('down', n))).toEqual([0, 1, 0, 2, 0, 1]);
  });
  it('keeps each facing in its own three columns, left mirrored for right', () => {
    expect(walkColumn('up', 1)).toBe(4);
    expect(walkColumn('left', 3)).toBe(8);
    expect(walkColumn('right', 3)).toBe(walkColumn('left', 3));
  });
  it('counts a stride walked backwards or half done', () => {
    expect(walkColumn('down', 1.9)).toBe(1);
    expect(walkColumn('down', -1)).toBe(2);
  });
});
