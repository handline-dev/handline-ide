import { Check, Circle, CircleDot, Hand, Hourglass, X, type LucideProps } from 'lucide-react';
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
};
/** The look of what the task is doing now: an open task something holds reads as waiting, whatever its state says. */
export const look = (t: { state: string; waiting?: unknown; block_reason: string | null; hold_reason: string | null }) => {
  const held = !['done', 'discarded'].includes(t.state) && !!(t.waiting || t.block_reason || t.hold_reason);
  return STATE[held ? 'waiting' : t.state] ?? { icon: CircleDot, tone: 'bg-muted', word: t.state };
};
