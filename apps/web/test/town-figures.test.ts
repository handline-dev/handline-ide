import { describe, expect, it } from 'vitest';
import { figureRows, PEOPLE } from '#src/town/sheet.ts';

describe('figureRows', () => {
  it('gives teammates different people and leaves an unknown key without one, so it falls back to its letter', () => {
    const rows = figureRows([
      { key: 'a', workspace: 'w' },
      { key: 'b', workspace: 'w' },
      { key: 'c', workspace: 'v' },
    ]);
    expect(rows.get('a')).not.toBe(rows.get('b'));
    expect([...rows.values()].every((r) => r >= 0 && r < PEOPLE)).toBe(true);
    expect(rows.get('human')).toBeUndefined();
  });
});
