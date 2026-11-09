import { act } from 'react';
import { createRoot } from 'react-dom/client';
import type { TaskDetail } from '@handline/client';
import { afterEach, expect, it, vi } from 'vitest';
import { ChoicePicker } from '#src/tools/TaskChoices.tsx';
import { turnReply } from '#src/tools/TaskTurn.ts';
import { choices } from '#src/tools/TaskWaiting.tsx';

const ask = 'Bell: how should each pause show?\nA) One line per pause\nB. One line per agent';

afterEach(() => {
  document.body.innerHTML = '';
});

it('reads the choices an ask lays out, one a line, by A) B. 1:; one alone, or a key on a line of its own, is no choice', () => {
  expect(choices(ask)).toEqual([
    { key: 'A', text: 'One line per pause' },
    { key: 'B', text: 'One line per agent' },
  ]);
  expect(choices('Pick:\n1: yes\n2: no').map((c) => c.key)).toEqual(['1', '2']);
  expect(choices('Go ahead?')).toEqual([]);
  expect(choices('Only:\nA) this')).toEqual([]);
  expect(choices('A)\nfoo\nB)\nbar')).toEqual([]);
});

it('the turn as what the box answers: its question, by whoever asked, marked as the turn', () => {
  const t = { memos: [], turns: [{ agent: 'human', ask, by: 'handline.arch', since: new Date().toISOString() }] } as unknown as TaskDetail;
  expect(turnReply(t)).toEqual({ id: undefined, author: 'handline.arch', text: ask, turn: true });
  expect(turnReply({ memos: [], turns: [] } as unknown as TaskDetail)).toBeNull();
});

const pick = async (options = choices(ask)) => {
  const on = { onPick: vi.fn(), onOther: vi.fn(), onCancel: vi.fn() };
  const el = document.createElement('div');
  document.body.append(el);
  await act(async () => createRoot(el).render(<ChoicePicker by="a" options={options} {...on} />));
  const box = el.querySelector('[role=listbox]')!;
  const key = (k: string, more: KeyboardEventInit = {}, on2: Element = box) =>
    act(async () => void on2.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true, ...more })));
  return { el, on, key };
};

it('picks as Claude Code asks: numbered, Something else last; a number or Enter picks, Esc steps out', async () => {
  const { el, on, key } = await pick();
  expect([...el.querySelectorAll('[role=option]')].map((o) => o.textContent)).toEqual([
    '1.One line per pause',
    '2.One line per agent',
    '3.Something else',
  ]);
  await key('2');
  expect(on.onPick).toHaveBeenLastCalledWith({ key: 'B', text: 'One line per agent' });
  await key('ArrowUp');
  await key('Enter');
  expect(on.onOther).toHaveBeenCalledTimes(1);
  await key('Escape');
  expect(on.onCancel).toHaveBeenCalledTimes(1);
});

it("keys on the × are its own, a number with Cmd is the browser's, and a number picks the choice it keys", async () => {
  const { el, on, key } = await pick();
  await key('Enter', {}, el.querySelector('[aria-label="Drop the answer"]')!);
  await key('1', { metaKey: true });
  expect(on.onPick).not.toHaveBeenCalled();
  const keyed = await pick([
    { key: '2', text: 'two' },
    { key: '3', text: 'three' },
  ]);
  await keyed.key('3');
  expect(keyed.on.onPick).toHaveBeenLastCalledWith({ key: '3', text: 'three' });
});
