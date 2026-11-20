import { Check, CircleDot, Hand, Hourglass, Layers, LoaderCircle, X, type LucideProps } from 'lucide-react';
import { taskOutcome } from '@handline/app-core';
import type { ReactNode } from 'react';

/** Doing: a circle turning slowly, the sidebar's working agent; still when motion is reduced. */
const Turning = (p: LucideProps) => <LoaderCircle {...p} className={`spin-slow ${p.className ?? ''}`} />;

/** Each state's look: the same coloured square the chat's task cards carry, and its word. */
export const STATE: Record<string, { icon: (p: LucideProps) => ReactNode; tone: string; word: string }> = {
  draft: { icon: CircleDot, tone: 'bg-muted', word: 'Draft' },
  queued: { icon: Layers, tone: 'bg-muted', word: 'Up next' },
  pending: { icon: Hourglass, tone: 'bg-muted', word: 'Pending' },
  doing: { icon: Turning, tone: 'bg-accent', word: 'Doing' },
  waiting: { icon: Hand, tone: 'bg-warning', word: 'Waiting' },
  blocked: { icon: Hourglass, tone: 'bg-muted', word: 'Blocked' },
  done: { icon: Check, tone: 'bg-success', word: 'Done' },
  discarded: { icon: X, tone: 'bg-danger', word: 'Discarded' },
  failed: { icon: X, tone: 'bg-danger', word: 'Failed' },
};
/** "Failed: <the checker's one line>" for a done task whose evidence says FAIL, else null. */
export const failedLine = (evidence: string | null | undefined, title?: string | null): string | null => {
  const o = taskOutcome(evidence, title);
  if (o.verdict !== 'FAIL') return null;
  const line = o.rest.split('\n')[0].trim();
  return line ? `Failed: ${line}` : 'Failed';
};
export type Lookable = {
  state: string;
  waiting?: { on: string } | null;
  turns?: unknown[] | null;
  acting?: string | null;
  block_reason: string | null;
  hold_reason: string | null;
  evidence?: string | null;
  title?: string;
};
/** The task's look, the same everywhere: the hand only when the founder is needed; other waits read as queued or blocked. */
export const look = (t: Lookable) => {
  const open = !['done', 'discarded'].includes(t.state);
  const onFounder = open && (t.waiting?.on === 'founder' || (!!t.turns?.length && t.acting === 'human'));
  const blocked = open && t.state !== 'queued' && !!(t.waiting || t.block_reason);
  const failed = t.state === 'done' && failedLine(t.evidence, t.title) !== null;
  const key = onFounder ? 'waiting' : blocked ? 'blocked' : failed ? 'failed' : t.state;
  return STATE[key] ?? { icon: CircleDot, tone: 'bg-muted', word: t.state };
};

/** Another task's state pill, from the TaskLook its row carries. */
export function TaskState({ t }: { t: Lookable }) {
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
