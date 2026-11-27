import { describe, expect, it } from 'vitest';
import { type StateWord, type StatusReason, type StatusState } from '#src/index.ts';
import { EVENT_REQUIRED } from '#src/generated/reads.ts';

describe('the generated contract types', () => {
  it('the spec enums are literal unions', () => {
    const ok: [StateWord, StatusState, StatusReason] = ['running', 'waiting', 'stalled'];
    expect(ok).toHaveLength(3);
  });
  it('a pushed event names the fields it must carry', () => {
    expect(EVENT_REQUIRED['agent.changed']).toContain('name');
  });
});
