import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it } from 'vitest';
import { createStore } from '#src/persist/store.ts';

it('a component that reads a store redraws when the store is set, under the React Compiler', () => {
  const store = createStore('before');
  function Shows() {
    const v = store.useValue();
    return <span>{v}</span>;
  }
  const el = document.createElement('div');
  const root = createRoot(el);
  act(() => root.render(<Shows />));
  expect(el.textContent).toBe('before');
  act(() => store.set('after'));
  expect(el.textContent).toBe('after');
});
