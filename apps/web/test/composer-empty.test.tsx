import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import type { Attachments } from '#src/chat/attachments.tsx';
import { Composer } from '#src/chat/Composer.tsx';

vi.mock('@handline/app-data', async (real) => ({ ...(await real<object>()), useAgents: () => ({ data: { agents: [] } }) }));

let root: Root;
let el: HTMLDivElement;
beforeEach(() => {
  el = document.createElement('div');
  document.body.append(el);
  root = createRoot(el);
});
afterEach(() => {
  act(() => root.unmount());
  el.remove();
});

const attach = (over: Partial<Attachments> = {}) =>
  ({ ids: [], sendable: true, uploads: [], folders: [], add() {}, clear() {}, remove() {}, ...over }) as unknown as Attachments;

function box(draft: string, over: { attach?: Attachments; locked?: string } = {}) {
  const on = { send: vi.fn(), empty: vi.fn() };
  act(() =>
    root.render(
      <Composer
        agent="task:1"
        draft={draft}
        setDraft={() => {}}
        attach={over.attach ?? attach()}
        locked={over.locked}
        onSend={on.send}
        sendLabel="Answer"
        onEmpty={{ label: 'Give back', run: on.empty }}
      />,
    ),
  );
  const send = el.querySelector<HTMLButtonElement>('button[type=submit]')!;
  return { on, send };
}

it('empty, the send button is the empty action and runs it; written, it is the label and sends', () => {
  const empty = box('');
  expect(empty.send.textContent).toBe('Give back');
  act(() => empty.send.click());
  expect(empty.on.empty).toHaveBeenCalledTimes(1);
  const written = box('B) one line');
  expect(written.send.textContent).toBe('Answer');
  act(() => written.send.click());
  expect(written.on.send).toHaveBeenCalledWith('B) one line', []);
  expect(written.on.empty).not.toHaveBeenCalled();
});

it('a picture still going up, or a locked box, is not empty: nothing is given back', () => {
  const going = box('', { attach: attach({ sendable: false }) });
  expect(going.send.textContent).toBe('Answer');
  expect(going.send.disabled).toBe(true);
  const locked = box('', { locked: 'Giving it back' });
  expect(locked.send.disabled).toBe(true);
  act(() => locked.send.click());
  expect(locked.on.empty).not.toHaveBeenCalled();
});
