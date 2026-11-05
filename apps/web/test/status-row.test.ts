import { describe, expect, it } from 'vitest';
import { weekUsagePct } from '#src/sidebar/status.ts';

describe('weekUsagePct', () => {
  it('is the highest weekly use, rounded', () => {
    expect(weekUsagePct([{ weekly: { used_pct: 12.4 } }, { weekly: { used_pct: 61.6 } }, { weekly: null }])).toBe(62);
  });
  it('is null when no account reports one', () => {
    expect(weekUsagePct([{ weekly: null }])).toBeNull();
    expect(weekUsagePct([])).toBeNull();
  });
});
