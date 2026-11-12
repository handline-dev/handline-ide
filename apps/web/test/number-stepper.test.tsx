import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { NumberStepper } from '#src/ui/NumberStepper.tsx';

let root: Root;
const el = document.createElement('div');
beforeEach(() => {
  document.body.append(el);
  root = createRoot(el);
});
afterEach(async () => {
  await act(async () => root.unmount());
  document.body.innerHTML = '';
});
const show = (ui: React.ReactElement) => act(async () => root.render(ui));
const box = () => el.querySelector('input') as HTMLInputElement;
const type = async (text: string) => {
  await act(async () => {
    box().focus();
    const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!;
    set.call(box(), text);
    box().dispatchEvent(new Event('input', { bubbles: true }));
  });
  await act(async () => box().blur());
};
const press = (label: string) => act(async () => (el.querySelector(`[aria-label="${label}"]`) as HTMLButtonElement).click());

describe('NumberStepper', () => {
  it('steps from the shown number and clamps a typed one', async () => {
    const commit = vi.fn();
    await show(<NumberStepper value={3} min={1} max={8} label="At once" onCommit={commit} />);
    await press('More');
    expect(commit).toHaveBeenLastCalledWith(4);
    await type('12');
    expect(commit).toHaveBeenLastCalledWith(8);
  });

  it('reads 0 as the word and commits a typed 0', async () => {
    const commit = vi.fn();
    await show(<NumberStepper value={2} zeroWord="No limit" label="At once" onCommit={commit} />);
    await type('0');
    expect(commit).toHaveBeenLastCalledWith(0);
    await show(<NumberStepper value={0} zeroWord="No limit" label="At once" onCommit={commit} />);
    expect(box().value).toBe('No limit');
  });

  it('keeps the sent number until the value moves, and drops it when the write fails', async () => {
    let fail: (e: unknown) => void = () => {};
    const commit = vi.fn(() => new Promise<void>((_, rej) => (fail = rej)));
    await show(<NumberStepper value={2} label="At once" onCommit={commit} />);
    await press('More');
    expect(box().value).toBe('3');
    await act(async () => fail(new Error('no')));
    expect(box().value).toBe('2');
  });
});
