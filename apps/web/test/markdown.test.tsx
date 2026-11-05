import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, expect, it } from 'vitest';

function render(node: React.ReactNode) {
  const container = document.createElement('div');
  act(() => createRoot(container).render(node));
  return { container };
}
import { Markdown } from '#src/ui/Markdown.tsx';

describe('Markdown', () => {
  it('keeps list numbers as written', () => {
    const { container } = render(<Markdown text={'1) one\n2) two\n\n3. three'} />);
    expect(container.textContent).toContain('1)one');
    expect(container.textContent).toContain('2)two');
    expect(container.textContent).toContain('3.three');
  });
  it('links tasks, addresses and paths', () => {
    const { container } = render(<Markdown text={'see T8520, https://example.com/a and apps/web/src/main.tsx:12'} />);
    const links = [...container.querySelectorAll('a')].map((a) => a.textContent);
    expect(links).toEqual(['T8520', 'https://example.com/a', 'apps/web/src/main.tsx:12']);
  });
  it('shows HTML as text', () => {
    const { container } = render(<Markdown text={'<b>x</b>'} />);
    expect(container.querySelector('b')).toBeNull();
  });
});
