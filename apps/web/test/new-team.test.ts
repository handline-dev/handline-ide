import { describe, expect, it } from 'vitest';
import { createParams, freeName, STARTERS, takenNames } from '#src/newTeam.ts';

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

  it('gives every tile but Empty a role for its lead', () => {
    for (const s of STARTERS) expect(Boolean(s.role)).toBe(s.word !== 'Empty');
  });
});

describe('createParams', () => {
  it('sends a folder and a role only when there is one', () => {
    expect(createParams('team', {})).toEqual({ name: 'team' });
    expect(createParams('writing', { role: 'r' })).toEqual({ name: 'writing', role: 'r' });
    expect(createParams('site', { folder: '/x/site' })).toEqual({ name: 'site', folder: '/x/site' });
  });
});

describe('takenNames', () => {
  it('holds every name the daemon refuses a new workspace: a workspace, any agent, a handle', () => {
    const taken = takenNames([{ name: 'shop' }], [{ name: 'research' }], [{ handle: 'kim' }, { handle: null }]);
    expect(taken).toEqual(['shop', 'research', 'kim']);
    expect(freeName('research', taken)).toBe('research-2');
  });
});
