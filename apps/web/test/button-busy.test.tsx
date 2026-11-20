import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it } from 'vitest';
import { Button, useAction } from '#src/ui/Button.tsx';

it('a busy button keeps its words, takes no press and says it is busy', () => {
  const el = document.createElement('div');
  const root = createRoot(el);
  act(() => root.render(<Button busy>Save</Button>));
  const b = el.querySelector('button')!;
  expect(b.disabled).toBe(true);
  expect(b.getAttribute('aria-busy')).toBe('true');
  expect(b.textContent).toContain('Save');
  act(() => root.render(<Button>Save</Button>));
  expect(b.disabled).toBe(false);
  expect(b.hasAttribute('aria-busy')).toBe(false);
  act(() => root.unmount());
});

it('useAction is busy while the call runs, then keeps the refusal', async () => {
  let a!: ReturnType<typeof useAction>;
  const Probe = () => {
    a = useAction();
    return null;
  };
  const root = createRoot(document.createElement('div'));
  act(() => root.render(<Probe />));
  let fail!: (e: Error) => void;
  let done!: Promise<boolean>;
  act(() => {
    done = a.go(() => new Promise((_, j) => (fail = j)));
  });
  expect(a.busy).toBe(true);
  await act(async () => {
    fail(new Error('no'));
    expect(await done).toBe(false);
  });
  expect(a.busy).toBe(false);
  expect((a.error as Error).message).toBe('no');
  act(() => root.unmount());
});
