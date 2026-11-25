import { describe, expect, it } from 'vitest';
import { onePerTask, type TaskEvent } from '#src/chat/TaskActivity.tsx';

const ev = (id: string, key: string): TaskEvent => ({ key, kind: 'edit', id, title: id, detail: '' });

describe('task cards in a chat', () => {
  it('show each task once, in the first card it came up in, with all its changes; a later card left empty goes', () => {
    const rows = [
      { id: 'a', events: [ev('T1', '1'), ev('T2', '2')] },
      { id: 'line' },
      { id: 'b', events: [ev('T2', '3')] },
      { id: 'c', events: [ev('T1', '4')] },
    ];
    const out = onePerTask(rows);
    expect(out.map((r) => r.id)).toEqual(['a', 'line']);
    expect((out[0] as { events: TaskEvent[] }).events.map((e) => e.key)).toEqual(['1', '2', '3', '4']);
  });
});
