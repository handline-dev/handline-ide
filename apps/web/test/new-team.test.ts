import { describe, expect, it } from 'vitest';
import { freeName, STARTERS } from '#src/newTeam.ts';

describe('freeName', () => {
  it('keeps a free name and numbers a taken one past every taken number', () => {
    expect(freeName('writing', ['coding'])).toBe('writing');
    expect(freeName('writing', ['writing', 'writing-2'])).toBe('writing-3');
  });

  it('stays within 32 letters, never ending in a dash before its number', () => {
    const base = `${'a'.repeat(29)}-bc`;
    const name = freeName(base, [base.slice(0, 32)]);
    expect(name.length).toBeLessThanOrEqual(32);
    expect(name).toMatch(/^a+-2$/);
  });

  it('gives every tile a name the daemon takes', () => {
    for (const s of STARTERS) expect(s.name).toMatch(/^[a-z][a-z0-9-]{0,31}$/);
  });
});
