import { describe, expect, it } from 'vitest';
import { nearOf, type Near } from '#src/town/near.ts';

const a: Near = { key: 'a', at: [1, 1], opens: 'chat:a', title: 'a' };
const b: Near = { key: 'b', at: [2, 1], opens: 'chat:b', title: 'b' };
const still = (x: number, z = 1) => ({ at: [x, z] as [number, number], moving: false });
const walking = (x: number, z = 1) => ({ at: [x, z] as [number, number], moving: true });

describe('what the founder is beside', () => {
  it('is the nearest thing it stopped beside, none while walking up to it', () => {
    expect(nearOf(still(1.2), [a, b], null)?.key).toBe('a');
    expect(nearOf(still(1.8), [a, b], null)?.key).toBe('b');
    expect(nearOf(walking(1.1), [a, b], null)).toBeNull();
    expect(nearOf(still(1.5), [a, b], null)).toBeNull();
    expect(nearOf(null, [a, b], a)).toBeNull();
  });

  it('keeps it while the founder is still close, and lets go once it walks off or stops beside another', () => {
    expect(nearOf(walking(1.5), [a, b], a)?.key).toBe('a');
    expect(nearOf(walking(1.7), [a, b], a)).toBeNull();
    expect(nearOf(still(1.95), [a, b], a)?.key).toBe('b');
    expect(nearOf(still(1.2), [b], a)).toBeNull();
  });
});

describe('two things on one spot', () => {
  const papers: Near = { key: 'papers:z', at: [1, 1], opens: 'tasks|z', title: 'Tasks · z' };
  it('gives the one clicked, else the one listed first', () => {
    expect(nearOf(still(1), [a, papers], null)?.key).toBe('a');
    expect(nearOf(still(1), [a, papers], null, 'papers:z')?.key).toBe('papers:z');
    expect(nearOf(still(1.5), [a, papers], null, 'papers:z')).toBeNull();
  });
});
