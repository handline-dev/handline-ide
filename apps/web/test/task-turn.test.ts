import type { TaskDetail } from '@handline/client';
import { expect, it } from 'vitest';
import { turnReply } from '#src/tools/TaskTurn.ts';

const ask = 'Bell: how should each pause show?\nA) One line per pause\nB. One line per agent';

it('the turn as what the box answers: its question, by whoever asked, marked as the turn', () => {
  const t = { memos: [], turns: [{ agent: 'human', ask, by: 'handline.arch', since: new Date().toISOString() }] } as unknown as TaskDetail;
  expect(turnReply(t)).toEqual({ author: 'handline.arch', text: ask, turn: true });
  expect(turnReply({ memos: [], turns: [] } as unknown as TaskDetail)).toBeNull();
});
