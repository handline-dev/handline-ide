import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, expect, it } from 'vitest';
import { CommandText } from '#src/ui/CommandText.tsx';

const draw = async (text: string, folder?: string) => {
  const el = document.createElement('div');
  await act(async () => createRoot(el).render(<CommandText text={text} folder={folder} />));
  return el;
};

describe('a command before a decision', () => {
  it('shows a long one-line command whole and wrapped, with its folder', async () => {
    const long = `brew install mas && mas install 497799835 && sudo xcodebuild -license accept && ${'x'.repeat(300)}`;
    const el = await draw(long, '/work/repo');
    expect(el.querySelector('pre')!.textContent).toBe(long);
    expect(el.querySelector('pre')!.className).toContain('whitespace-pre-wrap');
    expect(el.textContent).toContain('in /work/repo');
  });
  it('folds a command of many lines and opens it in place', async () => {
    const text = Array.from({ length: 20 }, (_, i) => `step ${i}`).join('\n');
    const el = await draw(text);
    expect(el.querySelector('pre')!.textContent!.split('\n')).toHaveLength(12);
    await act(async () => el.querySelector('button')!.click());
    expect(el.querySelector('pre')!.textContent).toBe(text);
    expect(el.querySelector('button')).toBeNull();
  });
});
