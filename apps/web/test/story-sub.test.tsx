import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';

let task: unknown = undefined;
vi.mock('@handline/app-data', () => ({
  useRead: (m: string) => ({ data: m === 'task.get' ? task : { commits: [] } }),
  useAttachmentUrl: () => null,
  useNameOf: () => (k: string) => k,
  useTasks: () => ({ data: { items: [] } }),
  useAgents: () => ({ data: undefined }),
  useWrite: () => ({ mutate: vi.fn(), isPending: false }),
}));
vi.mock('@handline/app-react', () => ({ useMe: () => 'human' }));
vi.mock('#src/tools/TaskTab.tsx', () => ({ TaskChat: () => null }));
vi.mock('#src/tools/TaskCode.tsx', () => ({ TaskCode: () => null, ChangeSize: () => null }));
vi.mock('#src/persist/tabState.ts', async () => {
  const { useState } = await import('react');
  return { useTabState: <T,>(_: string, v: T) => useState(v) };
});

const { SubTask } = await import('#src/tools/StorySub.tsx');

it('opens a sub-task that was loading without breaking the order of its hooks', () => {
  const story = { id: 1, subtasks: [{ id: 2, state: 'draft', plan_state: 'none', title: 'x' }] } as never;
  const sub = { id: 2 } as never;
  const el = document.createElement('div');
  const root = createRoot(el);
  act(() => root.render(<SubTask sub={sub} story={story} onPick={() => {}} at={[1, 1]} onStep={() => {}} />));
  task = { id: 2, title: 'Two', state: 'draft', to: 'a1', plan: null, plan_state: 'none', review: null, comments: [], timeline: [] };
  expect(() => act(() => root.render(<SubTask sub={sub} story={story} onPick={() => {}} at={[1, 1]} onStep={() => {}} />))).not.toThrow();
  expect(el.textContent).toContain('Two');
  act(() => root.unmount());
});
