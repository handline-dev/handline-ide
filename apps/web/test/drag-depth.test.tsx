import { act, type DragEvent } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useFileDrop } from '#src/ui/fileDrop.tsx';
import { useSpringOpen } from '#src/ui/springOpen.ts';

// These run through React Compiler, as the app does: it once turned `depth.current++ === 0` into a pre-increment.
const files = { dataTransfer: { types: ['Files'] } } as unknown as DragEvent;

let root: Root;
beforeEach(() => {
  const el = document.createElement('div');
  document.body.append(el);
  root = createRoot(el);
});
afterEach(() => act(() => root.unmount()));

function mount<T>(use: () => T): { current: T } {
  const out = {} as { current: T };
  function Probe() {
    out.current = use();
    return null;
  }
  act(() => root.render(<Probe />));
  return out;
}

describe('a drag of files over an area', () => {
  it('shows the drop hint on the first dragenter and keeps it over children', () => {
    const h = mount(() => useFileDrop(() => {}));
    act(() => h.current.handlers.onDragEnter(files));
    expect(h.current.over).toBe(true);
    act(() => h.current.handlers.onDragEnter(files));
    act(() => h.current.handlers.onDragLeave(files));
    expect(h.current.over).toBe(true);
    act(() => h.current.handlers.onDragLeave(files));
    expect(h.current.over).toBe(false);
  });

  it('opens a held row after a moment, from the first dragenter', () => {
    vi.useFakeTimers();
    const open = vi.fn();
    const h = mount(() => useSpringOpen(open));
    act(() => h.current.handlers.onDragEnter(files));
    expect(h.current.hovering).toBe(true);
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(open).toHaveBeenCalledOnce();
    vi.useRealTimers();
  });
});
