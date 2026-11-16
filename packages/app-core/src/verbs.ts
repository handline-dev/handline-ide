// The agent verbs of the bridge's AGENT_KEYS (tui.py): each one's words and the call it makes.

export type VerbId = 'interrupt' | 'close' | 'restart' | 'restart_fresh' | 'park' | 'fold' | 'move' | 'clear_stall' | 'force_start';

export interface Verb {
  id: VerbId;
  label: string;
  /** The daemon method behind it; a verb whose method the daemon does not list shows disabled. */
  method:
    | 'session.interrupt'
    | 'session.stop'
    | 'session.restart'
    | 'session.park'
    | 'agent.fold'
    | 'agent.move'
    | 'agent.unstall'
    | 'agent.force_start';
  /** Shown while the write is in flight, in the state cell. */
  gerund: string;
  /** One sentence of consequence for the confirmation; absent when the verb asks nothing. */
  ask?: string;
}

const V = (id: VerbId, label: string, method: Verb['method'], gerund: string, ask?: string): [VerbId, Verb] => [
  id,
  { id, label, method, gerund, ask },
];

export const VERBS: Record<VerbId, Verb> = Object.fromEntries([
  V('interrupt', 'Interrupt', 'session.interrupt', 'interrupting…'),
  V('close', 'Close', 'session.stop', 'closing…', 'Its session ends; the terminal, worktree and account stay.'),
  V('restart', 'Restart', 'session.restart', 'restarting…', 'Its session ends and starts again, resuming the conversation.'),
  V(
    'restart_fresh',
    'Restart fresh',
    'session.restart',
    'restarting…',
    "Its session ends and starts again; the session's context is dropped.",
  ),
  V('park', 'Park', 'session.park', 'parking…', 'Its session ends; it wakes on the next task.'),
  V('fold', 'Fold into handoff', 'agent.fold', 'folding…'),
  V('move', 'Move account', 'agent.move', 'moving…'),
  V('force_start', 'Start now', 'agent.force_start', 'starting…', 'Starts this work now, once, even though it is being held.'),
  V('clear_stall', 'Wake it', 'agent.unstall', 'waking…', 'The stall is cleared and the agent is woken.'),
] as [VerbId, Verb][]) as Record<VerbId, Verb>;

/** The call a verb makes. */
export function callOf(v: Verb, agent: string, account?: string): { method: Verb['method']; params: Record<string, unknown> } {
  const params: Record<string, unknown> = { agent };
  if (v.id === 'close') params.mode = 'close';
  if (v.id === 'restart_fresh') params.fresh = true;
  if (v.id === 'move') params.account = account;
  return { method: v.method, params };
}
