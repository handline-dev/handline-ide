import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import type { ShellUpdate } from '@handline/app-core';
import { afterEach, expect, test, vi } from 'vitest';
import { UpdateRow, useShellUpdate } from '#src/sidebar/UpdateButton.tsx';

const view = (phase: ShellUpdate['phase'], over: Partial<ShellUpdate> = {}): ShellUpdate => ({
  phase,
  step: '',
  fraction: 0,
  reason: '',
  ...over,
});

let root: Root | null = null;
let box: HTMLDivElement;
function Probe() {
  const u = useShellUpdate();
  return u ? <UpdateRow u={u} /> : null;
}
const buttons = () => [...box.querySelectorAll('button')];
const press = (label: string) => {
  const b = buttons().find((x) => x.textContent === label);
  expect(b).toBeTruthy();
  act(() => b!.click());
};

function fakeShell() {
  let push: (v: ShellUpdate) => void = () => {};
  const updateAct = vi.fn(() => Promise.resolve({ ok: true as const }));
  (window as { handline?: unknown }).handline = {
    api: 1,
    updateAct,
    onUpdate: (cb: (v: ShellUpdate) => void) => (cb(view('idle')), (push = cb), () => {}),
  };
  box = document.body.appendChild(document.createElement('div'));
  root = createRoot(box);
  act(() => root!.render(<Probe />));
  return { updateAct, send: (v: ShellUpdate) => act(() => push(v)) };
}

afterEach(() => {
  act(() => root?.unmount());
  box.remove();
  delete (window as { handline?: unknown }).handline;
});

test('a ready update is a row with Restart now, which restarts; a put-off one still is', () => {
  const s = fakeShell();
  expect(box.textContent).toBe('');
  for (const phase of ['ready', 'later'] as const) {
    s.send(view(phase));
    expect(box.textContent).toContain('A Handline update is ready.');
    press('Restart now');
  }
  expect(s.updateAct.mock.calls).toEqual([['restart'], ['restart']]);
});

test('while it builds the row shows the step; a failure offers Try again; idle has no row', () => {
  const s = fakeShell();
  s.send(view('preparing', { step: 'Building (2 of 3)' }));
  expect(box.textContent).toBe('Building (2 of 3)');
  s.send(view('failed', { reason: 'The disk is full.' }));
  press('Try again');
  expect(s.updateAct).toHaveBeenCalledWith('retry');
  s.send(view('idle'));
  expect(box.textContent).toBe('');
});
