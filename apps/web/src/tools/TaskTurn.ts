import { isPerson } from '@handline/app-core';
import type { TaskDetail } from '@handline/client';
import type { Reply } from '#src/tools/TaskComment.tsx';
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
