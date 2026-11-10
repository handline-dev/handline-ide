import type { TaskDetail } from '@handline/client';
import type { Reply } from '#src/tools/TaskComment.tsx';
import { keptScroll } from '#src/ui/keptScroll.ts';

export const GO_TURN = 'handline:go-turn';

/** Asks the open tab of a task to go to its turn; with none open, the mark left for the tab that opens. */
export function goToTurnOf(task: number) {
  keptScroll.set(`turn:${task}`, 'end');
  setTimeout(() => window.dispatchEvent(new CustomEvent(GO_TURN, { detail: task })));
}

/** The comment that made this the founder's turn: the last one by whoever asked, written by the time the turn began and within ten
 * minutes before it; none when there is no such comment (the slot is then all that shows). */
export function askingMemo(t: TaskDetail) {
  const turn = t.turns?.at(-1);
  if (!turn) return undefined;
  const since = Date.parse(turn.since);
  return t.memos.filter((m) => m.author === turn.by && Date.parse(m.at) <= since && Date.parse(m.at) >= since - 10 * 60_000).at(-1);
}

/** The founder's turn as what the comment box answers: its question, tied to the asking comment when there is one. */
export function turnReply(t: TaskDetail): Reply | null {
  const turn = t.turns?.at(-1);
  if (!turn?.ask) return null;
  return { id: askingMemo(t)?.id, author: turn.by, text: turn.ask, turn: true };
}

/** The asking comment into view and marked; else the slot that holds the question, marked, with no scrolling to an unrelated comment. */
export function goToTurn(t: TaskDetail) {
  const m = askingMemo(t);
  if (m && showMemo(m.id)) return;
  flash(document.querySelector('[data-turn-slot]'));
}

function flash(el: Element | null | undefined) {
  if (!el) return;
  el.classList.add('ring-2', 'ring-warning');
  setTimeout(() => el.classList.remove('ring-2', 'ring-warning'), 1500);
}

/** Marks a comment faintly while the pointer rests on what goes to it; `on` false takes the mark off. */
export function hintMemo(id: number, on: boolean) {
  document.querySelector(`[data-memo="${id}"]`)?.classList.toggle('bg-warning/5', on);
}

/** Brings a comment into view and marks it for a moment; false when it is not on the page. */
export function showMemo(id: number) {
  const el = document.querySelector(`[data-memo="${id}"]`);
  if (!el) return false;
  el.scrollIntoView({ block: 'center', behavior: 'smooth' });
  el.classList.add('bg-warning/10');
  setTimeout(() => el.classList.remove('bg-warning/10'), 1500);
  return true;
}
