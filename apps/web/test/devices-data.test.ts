import { describe, expect, it } from 'vitest';
import { isStale } from '#src/settings/data/devices.ts';

const now = Date.parse('2026-11-06T00:00:00Z');
const day = 86_400_000;
const seen = (days: number) => new Date(now - days * day).toISOString();

describe('isStale', () => {
  it('is a device not connected and not seen for over 30 days', () => {
    expect(isStale({ connected: false, last_seen: seen(31) }, now)).toBe(true);
    expect(isStale({ connected: false, last_seen: seen(30) }, now)).toBe(false);
  });
  it('is never a connected device or one never seen', () => {
    expect(isStale({ connected: true, last_seen: seen(90) }, now)).toBe(false);
    expect(isStale({ connected: false, last_seen: null }, now)).toBe(false);
  });
});
