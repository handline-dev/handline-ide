import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, expect, it, vi } from 'vitest';

vi.mock('@handline/app-data', () => ({
  useTask: () => ({ data: { title: 'Count unread chats' } }),
  Hooks: { useTaskChanges: () => ({ data: undefined }) },
}));
import { WorkLine } from '#src/chat/agent/WorkLine.tsx';
import { PaneOpen, type How } from '#src/layout/openTab.tsx';

describe('a work-alongside line', () => {
  it("opens the task's own page, as any task does, not a record of its own", async () => {
    const opened: [string, How | undefined][] = [];
    const el = document.createElement('div');
    document.body.append(el);
    const root = createRoot(el);
    const work = { phase: 'started', seat: 'a77', task_id: 9571 } as Parameters<typeof WorkLine>[0]['work'];
    await act(async () =>
      root.render(
        <PaneOpen.Provider value={(o, how) => opened.push([o, how])}>
          <WorkLine work={work} at={new Date().toISOString()} over={false} />
        </PaneOpen.Provider>,
      ),
    );
    await act(async () => el.querySelector('button')!.click());
    expect(opened.map(([o]) => o)).toEqual(['task:9571']);
    act(() => root.unmount());
  });
});
