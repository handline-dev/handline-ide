// The sidebar's unread view and a folded workspace's marks. Pure.

type Unreadish = { key: string; founder_unread?: number | null; last_to_founder?: { at: string } | null };

/** Agents with a message to the founder not yet read, newest message first; `kept` are ones read while the view is on, which stay
 * (their pill gone) until it is left, so no row moves under the pointer. */
export function unreadAgents<T extends Unreadish>(agents: readonly T[], kept: ReadonlySet<string>): T[] {
  return agents
    .filter((a) => (a.founder_unread ?? 0) > 0 || kept.has(a.key))
    .sort((a, b) => (b.last_to_founder?.at ?? '').localeCompare(a.last_to_founder?.at ?? ''));
}

/** How many agents have unread messages. */
export const unreadCount = (agents: readonly Unreadish[]) => agents.filter((a) => (a.founder_unread ?? 0) > 0).length;

/** A folded workspace's marks, in their order, only those above zero: who needs the founder, who is in error, who works. */
export function foldMarks(acts: readonly string[]): { act: 'you' | 'error' | 'working'; n: number }[] {
  return (['you', 'error', 'working'] as const).map((act) => ({ act, n: acts.filter((x) => x === act).length })).filter((m) => m.n > 0);
}
