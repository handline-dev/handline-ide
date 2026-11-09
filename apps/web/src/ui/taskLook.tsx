import { Check, Circle, CircleDot, Hand, Hourglass, X, type LucideProps } from 'lucide-react';
import { taskOutcome } from '@handline/app-core';
import { useTask } from '@handline/app-data';
import type { ReactNode } from 'react';

/** Doing: a filled dot, smaller than the square's other icons. */
const Dot = (p: LucideProps) => <Circle {...p} size={8} fill="currentColor" />;

/** Each state's look: the same coloured square the chat's task cards carry, and its word. */
export const STATE: Record<string, { icon: (p: LucideProps) => ReactNode; tone: string; word: string }> = {
  draft: { icon: CircleDot, tone: 'bg-muted', word: 'Draft' },
  queued: { icon: Hourglass, tone: 'bg-muted', word: 'Up next' },
  pending: { icon: Hourglass, tone: 'bg-muted', word: 'Pending' },
  doing: { icon: Dot, tone: 'bg-accent', word: 'Doing' },
  waiting: { icon: Hand, tone: 'bg-warning', word: 'Waiting' },
  blocked: { icon: Hand, tone: 'bg-warning', word: 'Blocked' },
  held: { icon: Hand, tone: 'bg-warning', word: 'Held' },
  done: { icon: Check, tone: 'bg-success', word: 'Done' },
  discarded: { icon: X, tone: 'bg-danger', word: 'Discarded' },
  failed: { icon: X, tone: 'bg-danger', word: 'Failed' },
};
/** "Failed: <the checker's one line>" for a done task whose evidence says FAIL, else null. */
export const failedLine = (evidence: string | null | undefined, title?: string | null): string | null => {
  const o = taskOutcome(evidence, title);
  if (o.verdict !== 'FAIL') return null;
  const line = o.rest.split('\n')[0]!.trim();
  return line ? `Failed: ${line}` : 'Failed';
};
/** The look of what the task is doing now: an open task something holds reads as waiting, whatever its state says; a done one that failed its check reads as failed. */
export const look = (t: {
  state: string;
  waiting?: unknown;
  block_reason: string | null;
  hold_reason: string | null;
  evidence?: string | null;
  title?: string;
}) => {
  const held = !['done', 'discarded'].includes(t.state) && !!(t.waiting || t.block_reason || t.hold_reason);
  const failed = t.state === 'done' && failedLine(t.evidence, t.title) !== null;
  return STATE[held ? 'waiting' : failed ? 'failed' : t.state] ?? { icon: CircleDot, tone: 'bg-muted', word: t.state };
};

/** Another task's state pill, read from that task so a held one reads as waiting. */
export function TaskState({ id }: { id: number }) {
  const t = useTask(id).data;
  if (!t) return null;
  const s = look(t);
  return (
    <span
      className={`mr-1.5 inline-flex h-4 items-center gap-1 rounded-sm px-1.5 align-text-bottom text-xs leading-none font-medium text-accent-fg ${s.tone}`}
    >
      <s.icon size={10} strokeWidth={3} />
      {s.word}
    </span>
  );
}
