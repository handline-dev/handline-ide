import { dotOf, restartingLine, whyOf } from '@handline/app-core';
type DotFacts = Parameters<typeof dotOf>[0];
export type AgentLike = { key: string; name: string; state: string } & Partial<Omit<DotFacts, 'state'>>;

/** The sidebar's agents in the person's `order` (nav prefs); one not in it comes after, by name. */
export function ordered<A extends AgentLike>(agents: readonly A[], order: readonly string[] = []): A[] {
  const rank = (n: string) => {
    const i = order.indexOf(n);
    return i < 0 ? Infinity : i;
  };
  return [...agents].sort((x, y) => rank(x.key) - rank(y.key) || x.name.localeCompare(y.name));
}

/** Which mark the agent shows, from the daemon's status: working, waking (starting or restarting), asleep, you (a person must answer),
 * held (a wait by design), error (a real fault), or resting. */
export type Activity = 'working' | 'waking' | 'asleep' | 'you' | 'held' | 'error' | 'resting';
export function activityOf(a: Pick<AgentLike, keyof DotFacts>): Activity {
  // From the rail dot, which reads AgentView.status first: an agent at its prompt with background work running is working.
  const dot = dotOf({ ...a, has_dialog: a.has_dialog ?? false });
  if (dot === 'running') return 'working';
  if (dot === 'waking') return 'waking';
  if (dot === 'parked' || dot === 'none') return 'asleep';
  if (dot === 'waiting') return 'you';
  // A usage wall or the account cap ends by itself, so it is a wait too.
  if (dot === 'blocked') return a.status?.state === 'waiting' || ['wall', 'cap'].includes(a.status?.reason ?? '') ? 'held' : 'error';
  return 'resting';
}

// The handoffs that are restarts; one for a full context or a move has its own line in the chat.
const RESTARTS = new Set(['settings_applied', 'restart']);
/** Whether a restart is under way: from the setting's change until the new session is up. */
export const restarting = (a: Pick<AgentLike, 'handoff'>): boolean => !!a.handoff && RESTARTS.has(a.handoff.cause);

/** Why the agent's mark is not plain, in one line: a restart under way, else the tasks it waits on, else the daemon's status line, else
 * its block's words; null while it works or rests. */
export function holdLine(a: Pick<AgentLike, keyof DotFacts> & Partial<Parameters<typeof whyOf>[0]>): string | null {
  // A restart says so from the setting's change until the new session is up, while its mark shows it working.
  if (a.handoff && restarting(a)) return restartingLine(a.handoff);
  const act = activityOf(a);
  if (act !== 'you' && act !== 'held' && act !== 'error') return null;
  const tasks = a.status?.reason === 'task' ? a.status.tasks : [];
  if (tasks.length) return `Waiting on ${tasks.map((t) => `T${t}`).join(', ')}`;
  const line = a.status?.text.trim() || whyOf({ blocker: a.blocker, wall_reason: a.wall_reason, state_detail: a.state_detail ?? '' });
  return line ? line[0]!.toUpperCase() + line.slice(1) : null;
}

/** Splits the list for the sidebar: the pinned agents in pin order, and the rest as they stood. */
export function splitPinned<A extends AgentLike>(agents: readonly A[], pins: readonly string[]) {
  const pinned = agents.filter((a) => pins.includes(a.key)).sort((x, y) => pins.indexOf(x.key) - pins.indexOf(y.key));
  return { pinned, rest: agents.filter((a) => !pins.includes(a.key)) };
}
