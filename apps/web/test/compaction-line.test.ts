import { describe, expect, it } from 'vitest';
import { changesOf, compactionLabel } from '#src/chat/CompactionLine.tsx';

describe('compactionLabel', () => {
  it('says a restart into the same conversation, with what changed', () => {
    expect(compactionLabel({ cause: 'settings_applied', resumed: true, changed: 'model changed (opus-5-5 -> sonnet-5-5)' })).toBe(
      'Restarted: model changed',
    );
    const team = 'team instructions changed (team.yaml, the agent list or a Handline update)';
    expect(compactionLabel({ cause: 'restart', resumed: true, changed: `${team}, team rules changed, ${team}` })).toBe(
      'Restarted: settings changed',
    );
    expect(compactionLabel({ cause: 'settings_applied', resumed: true })).toBe('Restarted: settings changed');
    expect(compactionLabel({ cause: 'move', resumed: true })).toBe('Restarted: moved to another account');
    expect(compactionLabel({ cause: 'restart', resumed: true })).toBe('Restarted');
  });
  it('splits what changed at the commas outside brackets, without the bracket, each once', () => {
    expect(changesOf('a (x, y), b, a (z), b (w.md)')).toEqual(['a', 'b']);
    expect(changesOf('')).toEqual([]);
  });
  it('still says Compacted for a new conversation, whatever its settings changed', () => {
    expect(compactionLabel({ cause: 'context_full', changed: 'team changed' })).toBe('Compacted: context was full');
    expect(compactionLabel({})).toBe('Compacted');
  });
});
