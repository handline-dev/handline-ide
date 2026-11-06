// A caller of a generated hook is a typecheck: a spec method removed or renamed breaks it here, and one added needs its hook.
import { describe, expect, it } from 'vitest';
import { hooks, type StateWord, type StatusReason, type StatusState } from '#src/index.ts';
import { reads } from '#src/reads.ts';
import { EVENT_REQUIRED } from '#src/generated/reads.ts';

describe('the generated hooks', () => {
  it('each read and cached write of the spec has a hook named from the method', () => {
    const callers = [hooks.useAgentGet, hooks.useAgentsList, hooks.useTaskNew, hooks.useTaskGet, hooks.useTaskCommentList];
    expect(callers.every((h) => typeof h === 'function')).toBe(true);
    for (const m of Object.keys(reads)) {
      const name = 'use' + m.split(/[._]/).map((w) => w[0]!.toUpperCase() + w.slice(1)).join('');
      expect(typeof (hooks as Record<string, unknown>)[name], name).toBe('function');
    }
  });
  it('the spec enums are literal unions', () => {
    const ok: [StateWord, StatusState, StatusReason] = ['running', 'waiting', 'stalled'];
    expect(ok).toHaveLength(3);
  });
  it('a pushed event names the fields it must carry', () => {
    expect(EVENT_REQUIRED['agent.changed']).toContain('name');
  });
});
