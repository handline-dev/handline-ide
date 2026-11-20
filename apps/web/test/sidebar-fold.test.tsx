import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
import { Section } from '#src/Sidebar.tsx';

const draw = (folded: boolean, onFold: () => void) => {
  const el = document.createElement('div');
  act(() =>
    createRoot(el).render(
      <Section title="handline" folded={folded} onFold={onFold}>
        <p>architect</p>
      </Section>,
    ),
  );
  return el;
};

it('a folded workspace shows only its header, and its chevron unfolds it', () => {
  const onFold = vi.fn();
  const folded = draw(true, onFold);
  expect(folded.textContent).not.toContain('architect');
  act(() => folded.querySelector<HTMLButtonElement>('[aria-label="Unfold handline"]')!.click());
  expect(onFold).toHaveBeenCalledOnce();
  expect(draw(false, onFold).textContent).toContain('architect');
});
