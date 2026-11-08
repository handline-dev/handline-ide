import { describe, expect, it } from 'vitest';
import { compactionLabel } from '#src/chat/CompactionLine.tsx';

describe('compactionLabel', () => {
  it('says a restart into the same conversation, with what changed', () => {
    expect(compactionLabel({ cause: 'settings_applied', resumed: true, changed: 'model changed (opus-5-5 -> sonnet-5-5)' })).toBe(
      'Restarted: model changed (opus-5-5 -> sonnet-5-5)',
    );
    expect(compactionLabel({ cause: 'settings_applied', resumed: true })).toBe('Restarted: settings changed');
    expect(compactionLabel({ cause: 'move', resumed: true })).toBe('Restarted: moved to another account');
    expect(compactionLabel({ cause: 'restart', resumed: true })).toBe('Restarted');
  });
  it('still says Compacted for a new conversation, whatever its settings changed', () => {
    expect(compactionLabel({ cause: 'context_full', changed: 'team changed' })).toBe('Compacted: context was full');
    expect(compactionLabel({})).toBe('Compacted');
  });
});
