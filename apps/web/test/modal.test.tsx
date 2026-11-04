import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type * as Modal from '#src/ui/modal.tsx';

// The modal store is module state: a fresh module per test keeps tests apart.
let m: typeof Modal;
let root: Root;

beforeEach(async () => {
  vi.resetModules();
  m = await import('#src/ui/modal.tsx');
  const el = document.createElement('div');
  document.body.append(el);
  root = createRoot(el);
  await act(async () => root.render(<m.ModalHost />));
});

afterEach(async () => {
  await act(async () => root.unmount());
  document.body.innerHTML = '';
});

const flush = () => act(async () => new Promise((r) => setTimeout(r, 20)));
const dialogs = () => [...document.querySelectorAll('[role="dialog"], [role="alertdialog"]')];
const button = (text: string) => [...document.querySelectorAll('button')].reverse().find((b) => b.textContent === text)!;
const escape = () =>
  act(async () => {
    (document.activeElement ?? document.body).dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  });

describe('modal', () => {
  it('confirm resolves true on OK and false on Cancel', async () => {
    let p!: Promise<boolean>;
    await act(async () => void (p = m.confirm({ title: 'Sure' })));
    await flush();
    await act(async () => button('OK').click());
    expect(await p).toBe(true);

    await act(async () => void (p = m.confirm({ title: 'Sure' })));
    await flush();
    await act(async () => button('Cancel').click());
    expect(await p).toBe(false);
  });

  it('Escape resolves undefined', async () => {
    let p!: Promise<string | undefined>;
    await act(async () => void (p = m.openModal<string>(() => <m.ModalFrame title="T" />)));
    await flush();
    await escape();
    expect(await p).toBeUndefined();
  });

  it('canClose false keeps the modal open', async () => {
    let settled = false;
    await act(async () => void m.openModal(() => <m.ModalFrame title="T" />, { canClose: () => false }).then(() => (settled = true)));
    await flush();
    await escape();
    await flush();
    expect(settled).toBe(false);
    expect(dialogs()).toHaveLength(1);
  });

  it('a second close while the guard is asking shows the guard once', async () => {
    const guard = vi.fn(() => new Promise<boolean>((r) => setTimeout(() => r(true), 30)));
    let close!: Modal.Close<string>;
    let p!: Promise<string | undefined>;
    await act(async () => void (p = m.openModal<string>((c) => ((close = c), (<m.ModalFrame title="T" />)), { canClose: guard })));
    await flush();
    await act(async () => {
      close();
      close();
    });
    expect(await p).toBeUndefined();
    expect(guard).toHaveBeenCalledTimes(1);
  });

  it('abort closes it and the listener is removed on close', async () => {
    const ac = new AbortController();
    const remove = vi.spyOn(ac.signal, 'removeEventListener');
    let p!: Promise<unknown>;
    await act(async () => void (p = m.openModal(() => <m.ModalFrame title="T" />, { signal: ac.signal })));
    await flush();
    await act(async () => ac.abort());
    expect(await p).toBeUndefined();
    expect(remove).toHaveBeenCalledWith('abort', expect.any(Function));
  });

  it('stacked: Escape closes only the top modal', async () => {
    let settledA = false;
    let b!: Promise<unknown>;
    await act(async () => void m.openModal(() => <m.ModalFrame title="A" />).then(() => (settledA = true)));
    await flush();
    await act(async () => void (b = m.openModal(() => <m.ModalFrame title="B" />)));
    await flush();
    await escape();
    expect(await b).toBeUndefined();
    await flush();
    expect(settledA).toBe(false);
  });

  it('warns when no ModalHost is mounted', async () => {
    await act(async () => root.unmount());
    const err = vi.spyOn(console, 'error').mockImplementation(() => {});
    void m.openModal(() => null);
    expect(err).toHaveBeenCalledWith(expect.stringContaining('no <ModalHost />'));
    err.mockRestore();
    root = createRoot(document.createElement('div')); // afterEach unmounts it
  });
});
