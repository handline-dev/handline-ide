import { describe, expect, it } from 'vitest';
import { cut, lineUp } from '#src/town/names.ts';

// One unit per letter, so a text's width is its length.
const width = (t: string) => t.length;
const bench = (texts: string[], pitch: number, y = 0) => texts.map((text, i) => ({ item: i, x: i * pitch, y, text }));

/** Whether any two labels on the same line cover each other. */
const overlap = (out: ReturnType<typeof lineUp<number>>) =>
  out.some((a) => out.some((b) => a !== b && a.y === b.y && Math.abs(a.x - b.x) < (width(a.text) + width(b.text)) / 2));

describe('lineUp', () => {
  it('keeps short names on one line, as written', () => {
    const out = lineUp(bench(['web', 'api', 'qa'], 10), 2, width);
    expect(out.map((l) => [l.text, l.y])).toEqual([
      ['web', 0],
      ['api', 0],
      ['qa', 0],
    ]);
  });

  it('drops a name that runs into the one before it to a second line, whole', () => {
    const out = lineUp(bench(['daemon-tasks', 'audit-daemon', 'ui'], 10), 2, width);
    expect(out.map((l) => l.text)).toEqual(['daemon-tasks', 'audit-daemon', 'ui']);
    expect(out[1].y).toBeGreaterThan(out[0].y);
    expect(out[2].y).toBe(0);
    expect(overlap(out)).toBe(false);
  });

  it('lays out a bench of long names with none covering another', () => {
    const out = lineUp(bench(['daemon-tasks-2', 'daemon-core-3', 'daemon-usage-2', 'research-3'], 6), 2, width);
    expect(out.some((l) => l.text.endsWith('…'))).toBe(true);
    expect(overlap(out)).toBe(false);
  });

  it('cuts a name wider than two desks so the names after it still fit', () => {
    const long = 'x'.repeat(30);
    const out = lineUp(bench([long, long, long, 'ab'], 6), 2, width);
    expect(out.every((l) => l.text.length < 30)).toBe(true);
    expect(overlap(out)).toBe(false);
  });

  it('lays out each bench on its own', () => {
    const out = lineUp([...bench(['long-name-a', 'b'], 10, 0), ...bench(['long-name-c', 'd'], 10, 50)], 2, width);
    expect(out.map((l) => l.y)).toEqual([0, 0, 50, 50]);
  });
});

describe('cut', () => {
  it('keeps what fits and ends with an ellipsis', () => {
    expect(cut('research', 20, width)).toBe('research');
    expect(cut('research', 5, width)).toBe('rese…');
    expect(cut('research', 0, width)).toBe('r…');
  });
});
