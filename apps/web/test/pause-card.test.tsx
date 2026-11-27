import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';

const ok = (data: unknown) => ({ isSuccess: true, data });
const resume = vi.fn(() => Promise.resolve({}));
const status = { state: 'waiting', reason: 'paused', text: 'paused', tasks: [7], until: null, since: null };
vi.mock('@handline/app-data', () => ({
  useAgents: () => ok({ agents: [{ key: 'a', status }] }),
  useTask: () => ok({ title: 'Do it' }),
  useRead: () => ok({}),
  useWrite: () => ({ mutateAsync: resume }),
}));
vi.mock('#src/ui/toast.tsx', () => ({ toast: vi.fn() }));
import { PauseCard } from '#src/chat/PauseCard.tsx';

it('leaves at the accepted click, while agents.list still says paused', async () => {
  const el = document.createElement('div');
  await act(async () => createRoot(el).render(<PauseCard agent="a" />));
  expect(el.textContent).toContain('T7 Do it');
  const button = [...el.querySelectorAll('button')].find((b) => b.textContent === 'Resume all')!;
  await act(async () => button.click());
  expect(resume).toHaveBeenCalled();
  expect(el.textContent).toBe('');
});
