import { act, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, expect, it } from 'vitest';
import { useBackCloses } from '#src/ui/backCloses.ts';

// The test DOM keeps no entries of its own: a small history, as a browser's, with back firing popstate.
const entries: unknown[] = [null];
history.pushState = (state: unknown) => void entries.push(state);
Object.defineProperty(history, 'state', { get: () => entries[entries.length - 1] ?? null });
Object.defineProperty(history, 'length', { get: () => entries.length });
history.back = () => {
  entries.pop();
  setTimeout(() => dispatchEvent(new PopStateEvent('popstate', { state: history.state })), 0);
};
const popped = () => new Promise<void>((r) => addEventListener('popstate', () => setTimeout(r, 0), { once: true }));

const api = {} as { open: boolean; step: number; setOpen: (o: boolean) => void };
function Probe({ steps }: { steps: boolean }) {
  const [open, setOpen] = useState(false);
  const [step, setStep] = useState(2);
  useBackCloses(open, () => (steps && step > 0 ? setStep(step - 1) : setOpen(false)));
  Object.assign(api, { open, step, setOpen });
  return null;
}
function Swap() {
  const [which, setWhich] = useState<'a' | 'b' | null>('a');
  useBackCloses(which === 'a', () => setWhich(null));
  useBackCloses(which === 'b', () => setWhich(null));
  Object.assign(swapper, { set: setWhich });
  return null;
}
const swapper = {} as { set: (w: 'a' | 'b' | null) => void };
const settled = () => new Promise((r) => setTimeout(r, 10));
const mount = (steps: boolean) => act(() => createRoot(document.createElement('div')).render(<Probe steps={steps} />));

describe('Back and modals', () => {
  it('Back closes an open modal, and closing it by a button takes its history entry off', async () => {
    mount(false);
    const before = history.length;
    act(() => api.setOpen(true));
    await settled();
    expect(history.length).toBe(before + 1);
    const p = popped();
    history.back();
    await p;
    expect(api.open).toBe(false);

    act(() => api.setOpen(true));
    await settled();
    expect((history.state as { modal?: boolean }).modal).toBe(true);
    const q = popped();
    act(() => api.setOpen(false));
    await q;
    expect((history.state as { modal?: string } | null)?.modal).toBeUndefined();
  });
  it('a modal with steps goes back one step and stays open, then closes on the first', async () => {
    mount(true);
    act(() => api.setOpen(true));
    await settled();
    for (const want of [1, 0]) {
      const p = popped();
      history.back();
      await act(() => p);
      expect(api.open).toBe(true);
      expect(api.step).toBe(want);
      await settled();
      expect((history.state as { modal?: boolean }).modal).toBe(true);
    }
    const p = popped();
    history.back();
    await act(() => p);
    expect(api.open).toBe(false);
  });
  it('a modal closing as another opens keeps the one entry', async () => {
    act(() => createRoot(document.createElement('div')).render(<Swap />));
    await settled();
    const n = history.length;
    act(() => swapper.set('b'));
    await settled();
    expect(history.length).toBe(n);
    expect((history.state as { modal?: boolean }).modal).toBe(true);
    const p = popped();
    act(() => swapper.set(null));
    await p;
    await settled();
    expect(history.length).toBe(n - 1);
  });
});
