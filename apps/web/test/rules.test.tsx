import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, expect, it, vi } from 'vitest';

const mutate = vi.fn();
const items = [
  { id: 1, agent: 'p1', text: 'Flat rows, never cards.', area: 'screen', count: 3, created_at: '', created_by: 'p1' },
  { id: 2, agent: 'p1', text: 'Fix the cause, not a patch.', area: 'work', count: 1, created_at: '', created_by: 'p1' },
  { id: 3, agent: 'p1', text: 'No shadows.', area: 'screen', count: 5, created_at: '', created_by: 'p1' },
];
vi.mock('@handline/app-data', () => ({
  useLessons: () => ({ data: { items } }),
  useWrite: () => ({ mutate, isPending: false }),
  why: String,
}));
import { Rules } from '#src/settings/Rules.tsx';

describe("the founder's rules", () => {
  it('lists Screens then Work, the most said first, each with how many times', async () => {
    const el = document.createElement('div');
    await act(async () => createRoot(el).render(<Rules person="p1" />));
    expect([...el.querySelectorAll('h3')].map((h) => h.textContent)).toEqual(['Screens', 'Work']);
    expect([...el.querySelectorAll('li')].map((li) => li.textContent)).toEqual([
      'No shadows.said 5 times',
      'Flat rows, never cards.said 3 times',
      'Fix the cause, not a patch.said once',
    ]);
    await act(async () => el.querySelector<HTMLButtonElement>('[aria-label="Remove this preference"]')!.click());
    expect(mutate).toHaveBeenCalledWith({ id: 3 }, expect.anything());
  });
});
