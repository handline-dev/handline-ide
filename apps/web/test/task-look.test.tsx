import { describe, expect, it } from 'vitest';
import { failedLine, look } from '#src/ui/taskLook.tsx';

const done = (evidence: string | null) => ({ state: 'done', block_reason: null, hold_reason: null, evidence, title: 'Audit' });

describe('a done task whose evidence says FAIL', () => {
  it("reads as failed, with the checker's first line", () => {
    expect(failedLine('FAIL @abc1234 two tests red\nmore')).toBe('Failed: two tests red');
    expect(look(done('FAIL @abc1234 two tests red')).word).toBe('Failed');
  });
  it('is done when the evidence passes or says nothing', () => {
    expect(failedLine('PASS @abc1234 ok')).toBeNull();
    expect(look(done(null)).word).toBe('Done');
    expect(look(done('merged abc')).word).toBe('Done');
  });
});
