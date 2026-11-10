import type { TaskDetail } from '@handline/client';
import { expect, it } from 'vitest';
import { turnReply } from '#src/tools/TaskTurn.ts';
import { choices } from '#src/tools/TaskWaiting.tsx';

const ask = 'Bell: how should each pause show?\nA) One line per pause\nB. One line per agent';

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
