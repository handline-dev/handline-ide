import { describe, expect, it } from 'vitest';
import type { VendorStatus } from '@handline/client';
import { aiRows, resumeAt, stepsDone, type Facts } from '#src/onboarding/data/steps.ts';

const none: Facts = { accounts: [], missingTools: [], notify: 'default', notifySkipped: false, repos: 0, agents: 0 };
const all = { ai: true, tools: true, notify: true, project: true, agent: true };

describe('stepsDone', () => {
  it('reads each step from the machine', () => {
    expect(stepsDone(none)).toEqual({ ai: false, tools: true, notify: false, project: false, agent: false });
    const f: Facts = { ...none, accounts: [{ login_state: 'ok' }], notify: 'granted', repos: 1, agents: 1 };
    expect(stepsDone(f)).toEqual(all);
  });
  it('counts only git and tmux for tools: a missing AI command belongs to the AI step', () => {
    expect(stepsDone({ ...none, missingTools: [{ tool: 'codex' }] }).tools).toBe(true);
    expect(stepsDone({ ...none, missingTools: [{ tool: 'tmux' }] }).tools).toBe(false);
  });
  it('ticks notifications on a skip or where the browser has none', () => {
    expect(stepsDone({ ...none, notifySkipped: true }).notify).toBe(true);
    expect(stepsDone({ ...none, notify: 'unsupported' }).notify).toBe(true);
    expect(stepsDone({ ...none, notify: 'denied' }).notify).toBe(false);
  });
});

describe('resumeAt', () => {
  it('opens where the person left when that step is still open', () => {
    expect(resumeAt({ ...all, notify: false, project: false }, 3)).toBe(3);
  });
  it('moves on to the next open step, wrapping round', () => {
    expect(resumeAt({ ...all, notify: false, agent: false }, 2 + 1)).toBe(4);
    expect(resumeAt({ ...all, ai: false }, 3)).toBe(0);
  });
  it('clamps a kept step out of range', () => {
    expect(resumeAt({ ...all, agent: false }, 9)).toBe(4);
  });
});

describe('aiRows', () => {
  const vendors: VendorStatus[] = [
    { vendor: 'claude', binary: 'claude', installed: true, fix: 'curl … | sh' },
    { vendor: 'codex', binary: 'codex', installed: false, fix: 'npm install -g @openai/codex' },
    { vendor: 'opencode', binary: 'opencode', installed: true, fix: '' },
  ];
  const acct = (name: string, vendor: string, login_state: 'ok' | 'needs_login', login?: { state: 'waiting' }) => ({
    name,
    vendor,
    login_state,
    email: `${name}@example.com`,
    login: login && { login_id: 'l1', ...login },
  });

  it('offers claude and codex only, with what was found on this Mac', () => {
    const found = [
      { vendor: 'claude', source: '~/.claude', logged_in: true, adoptable: false },
      { vendor: 'codex', source: '~/.codex', logged_in: true, adoptable: true },
    ];
    const rows = aiRows(vendors, found, []);
    expect(rows.map((r) => [r.vendor, r.state, r.source, r.adoptable, r.installed])).toEqual([
      ['claude', 'found', '~/.claude', false, true],
      ['codex', 'found', '~/.codex', true, false],
    ]);
  });
  it('prefers a ready account, then one logging in, over a found login', () => {
    const found = [{ vendor: 'codex', source: '~/.codex', logged_in: true, adoptable: true }];
    const rows = aiRows(vendors, found, [acct('claude', 'claude', 'ok'), acct('codex', 'codex', 'needs_login', { state: 'waiting' })]);
    expect(rows.map((r) => [r.state, r.account])).toEqual([
      ['ready', 'claude'],
      ['logging_in', 'codex'],
    ]);
  });
  it('shows nothing found for a folder whose login is out, and keeps an account made earlier for the next login', () => {
    const found = [{ vendor: 'codex', source: '~/.codex', logged_in: false, adoptable: true }];
    const [, codex] = aiRows(vendors, found, [acct('codex', 'codex', 'needs_login')]);
    expect([codex!.state, codex!.account, codex!.email, codex!.source]).toEqual(['none', 'codex', null, null]);
  });
  it('leaves out a vendor the daemon cannot run', () => {
    expect(aiRows(vendors.slice(0, 1), [], []).map((r) => r.vendor)).toEqual(['claude']);
  });
});
