// The dot on a tab that has something to read. Pure.
import type { Task } from '@handline/client';
import { foundersTurn, type Turns } from '#src/turns.ts';

/** What a tab's dot stands on, or null for a kind with nothing to read; `within` is the workspace's agent, whose own chat `chat` opens. */
export type TabTarget = { on: 'agent'; name: string } | { on: 'task'; id: number } | { on: 'requests' } | null;

export function tabTarget(opens: string | undefined, within: string | null): TabTarget {
  if (!opens) return null;
  if (opens === 'requests') return { on: 'requests' };
  if (opens === 'chat') return within ? { on: 'agent', name: within } : null;
  if (opens.startsWith('chat:')) return { on: 'agent', name: opens.slice(5) };
  const id = opens.startsWith('task:') ? Number(opens.slice(5)) : NaN;
  return Number.isInteger(id) && id > 0 ? { on: 'task', id } : null;
}

type DotData = {
  me: string;
  /** The agent's founder_unread, the sidebar's number. */
  agentUnread?: number;
  /** The task as task.get reads it for the founder. */
  task?: Turns & Pick<Task, 'unread'>;
  /** The bell's unread inbox items. */
  inboxUnread: number;
};

export function tabHasDot(target: NonNullable<TabTarget>, d: DotData): boolean {
  if (target.on === 'agent') return (d.agentUnread ?? 0) > 0;
  if (target.on === 'task') return !!d.task && ((d.task.unread ?? 0) > 0 || foundersTurn(d.task, d.me));
  return d.inboxUnread > 0;
}
