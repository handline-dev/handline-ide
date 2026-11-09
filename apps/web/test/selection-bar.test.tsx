import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { selectionRoom, WithSelectionBar } from '#src/ui/SelectionBar.tsx';

let root: Root;
let el: HTMLDivElement;
beforeEach(() => {
  el = document.createElement('div');
  document.body.append(el);
  root = createRoot(el);
});
afterEach(async () => {
  await act(async () => root.unmount());
  document.body.innerHTML = '';
});

const render = (count: number, onClear = vi.fn()) =>
  act(async () =>
    root.render(
      <WithSelectionBar count={count} onClear={onClear} actions={<button type="button">Do it</button>}>
        <div className={selectionRoom(count)}>
          <button type="button">row</button>
        </div>
      </WithSelectionBar>,
    ),
  );
const bar = () => document.querySelector('[role="region"]');
const press = (target: Element, key: string) =>
  act(async () => {
    target.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }));
  });

describe('WithSelectionBar', () => {
  it('shows nothing while nothing is selected', async () => {
    await render(0);
    expect(bar()).toBeNull();
  });

  it('floats the count and the actions', async () => {
    await render(3);
    expect(bar()?.textContent).toContain('3 selected');
    expect(bar()?.textContent).toContain('Do it');
  });

  it('clears on its × and on Escape inside the list, not on Escape a menu already took', async () => {
    const onClear = vi.fn();
    await render(2, onClear);
    await act(async () => (el.querySelector('[aria-label^="Clear the selection"]') as HTMLButtonElement).click());
    expect(onClear).toHaveBeenCalledTimes(1);
    const row = [...el.querySelectorAll('button')].find((b) => b.textContent === 'row')!;
    await press(row, 'Escape');
    expect(onClear).toHaveBeenCalledTimes(2);
    await act(async () => {
      const e = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true });
      e.preventDefault();
      row.dispatchEvent(e);
    });
    expect(onClear).toHaveBeenCalledTimes(2);
  });
});
