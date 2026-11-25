import { isPerson } from '@handline/app-core';
import type { TaskDetail } from '@handline/client';
import type { Reply } from '#src/tools/TaskComment.tsx';
import { useEffect } from 'react';
import { keptScroll } from '#src/ui/keptScroll.ts';

export const GO_TURN = 'handline:go-turn';

// Tasks whose comment box is to take the keys when the tab goes to the turn.
const typing = new Set<number>();

/** Asks the open tab of a task to go to its turn, and with `type` to put the keys in its comment box; with none open, the mark left
 * for the tab that opens. */
export function goToTurnOf(task: number, type = false) {
  keptScroll.set(`turn:${task}`, 'end');
  if (type) typing.add(task);
  setTimeout(() => window.dispatchEvent(new CustomEvent(GO_TURN, { detail: task })));
}

export const GO_MEMO = 'handline:go-memo';

/** Asks the tab of `task` to bring comment `memo` into view: an open one at once, one that opens once it is drawn. */
export function goToMemoOf(task: number, memo: number) {
  keptScroll.set(`memo:${task}`, memo);
  setTimeout(() => window.dispatchEvent(new CustomEvent(GO_MEMO, { detail: task })));
}

/** Whether the comment box of `task` was asked to take the keys; asked once. */
export const takeTyping = (task: number) => typing.delete(task);

/** The founder's turn as what the comment box answers: its question, by whoever asked. */
export function turnReply(t: TaskDetail): Reply | null {
  const turn = t.turns?.at(-1);
  if (!turn?.ask) return null;
  return { author: turn.by, text: turn.ask, turn: true };
}

/** The Activity row of the founder's open turn: the comment it was passed with, or for a turn passed with none, when its row was
 * written (the last push to them by whoever holds it back). */
export type OpenTurn = { memo: number } | { at: string };
export function openTurnRow(t: TaskDetail, me: string): OpenTurn | undefined {
  const turn = t.turns?.at(-1);
  if (!turn || !isPerson(turn.agent, me)) return undefined;
  if (turn.memo != null) return { memo: turn.memo };
  const at = t.timeline
    .filter((e) => {
      const [by, to] = e.who.split('→').map((s) => s.trim());
      return e.kind === 'turn' && e.turn === 'push' && by === turn.by && isPerson(to ?? '', me);
    })
    .at(-1)?.at;
  return at ? { at } : undefined;
}

/** The task's open turn row brought into view, in the task's own tab. */
export function goToTurn(task: number) {
  document.querySelector(`[data-task-scroll="${task}"] [data-turn-open]`)?.scrollIntoView({ block: 'center', behavior: 'smooth' });
}

/** Brings a comment into view and marks it for a moment. */
export function showMemo(id: number) {
  const el = document.querySelector(`[data-memo="${id}"]`);
  if (!el) return;
  el.scrollIntoView({ block: 'center', behavior: 'smooth' });
  el.classList.add('ring-2', 'ring-warning');
  setTimeout(() => el.classList.remove('ring-2', 'ring-warning'), 1500);
}

/** After the founder comments: the page goes to its end, where the new comment is, once it is drawn. */
export function toBottom(id: number) {
  setTimeout(() => {
    const el = document.querySelector(`[data-task-scroll="${id}"]`);
    el?.scrollTo({ top: el.scrollHeight, behavior: 'smooth' });
  }, 50);
}

/** Once the task is drawn: a comment asked for (a rule's quote in Settings) comes into view; else the founder's turn, when asked or on a
 *  first open while it is theirs. */
export function useGoToMark(id: number, loaded: boolean, onTurn: boolean) {
  useEffect(() => {
    if (!loaded) return;
    const memo = keptScroll.take(`memo:${id}`);
    const marked = keptScroll.take(`turn:${id}`);
    if (typeof memo === 'number') {
      const at = setTimeout(() => showMemo(memo), 100);
      return () => clearTimeout(at);
    }
    if (!marked && !(onTurn && keptScroll.get(`task:${id}`) === undefined)) return;
    const at = setTimeout(() => goToTurn(id), 100);
    return () => clearTimeout(at);
  }, [loaded, onTurn, id]);
  // Already drawn when asked: there at once.
  useEffect(() => {
    const on = (e: Event) => {
      if ((e as CustomEvent<number>).detail !== id || !loaded) return;
      const memo = keptScroll.take(`memo:${id}`);
      if (typeof memo === 'number') showMemo(memo);
    };
    window.addEventListener(GO_MEMO, on);
    return () => window.removeEventListener(GO_MEMO, on);
  }, [id, loaded]);
}
