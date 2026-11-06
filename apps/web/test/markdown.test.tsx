import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, expect, it, vi } from 'vitest';

// The thumbnail reads the attachment from the daemon; here it only says which one it would show.
vi.mock('#src/chat/MessageImages.tsx', () => ({
  MessageImages: ({ images }: { images: { id: string; name: string }[] }) =>
    images.length ? <span data-ids={images.map((i) => i.id).join(' ')} /> : null,
}));

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
  it('draws a pasted picture (attachment: link) as its image, not as the raw reference', () => {
    const { container } = render(<Markdown text={'look\n![capture](attachment:aaa)\n![capture](attachment:bbb)'} />);
    // One row for the paragraph's pictures, after its words and outside the <p>.
    expect(container.querySelector('[data-ids]')?.getAttribute('data-ids')).toBe('aaa bbb');
    expect(container.querySelector('p [data-ids]')).toBeNull();
    expect(container.textContent).toBe('look');
  });
  it('puts pictures pasted on lines or paragraphs of their own into one row', () => {
    const one = render(<Markdown text={'![a](attachment:a)'} />).container;
    expect([...one.querySelectorAll('[data-ids]')].map((e) => e.getAttribute('data-ids'))).toEqual(['a']);
    const five = render(
      <Markdown text={'see\n\n![a](attachment:a)\n\n![b](attachment:b)\n![c](attachment:c)\n\n![d](attachment:d)\n\n![e](attachment:e)'} />,
    ).container;
    expect([...five.querySelectorAll('[data-ids]')].map((e) => e.getAttribute('data-ids'))).toEqual(['a b c d e']);
    const said = render(<Markdown text={'look\n![a](attachment:a)\n\n![b](attachment:b)'} />).container;
    expect([...said.querySelectorAll('[data-ids]')].map((e) => e.getAttribute('data-ids'))).toEqual(['a b']);
  });
  it('shows HTML as text', () => {
    const { container } = render(<Markdown text={'<b>x</b>'} />);
    expect(container.querySelector('b')).toBeNull();
  });
});
