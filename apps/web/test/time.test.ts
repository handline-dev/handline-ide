import { describe, expect, it } from 'vitest';
import { ago } from '#src/format/time.ts';

const now = Date.parse('2026-11-04T12:00:00Z');
const before = (s: number) => new Date(now - s * 1000).toISOString();

describe('ago', () => {
  it('steps from now to minutes, hours, days, then a date', () => {
    expect(ago(before(30), now)).toBe('now');
    expect(ago(before(5 * 60), now)).toBe('5m');
    expect(ago(before(3 * 3600), now)).toBe('3h');
    expect(ago(before(2 * 86400), now)).toBe('2d');
    const d = new Date(now - 10 * 86400 * 1000);
    expect(ago(d.toISOString(), now)).toBe(`${d.getMonth() + 1}/${d.getDate()}`);
    expect(ago('2025-03-01T12:00:00Z', now)).toMatch(/^2025\/3\/\d$/);
  });
  it('reads a future time as now and a bad one as nothing', () => {
    expect(ago(before(-120), now)).toBe('now');
    expect(ago('nope', now)).toBe('');
  });
});
