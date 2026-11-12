// Every read method of the contract, once: what its cache entry is called and what keeps it fresh. A hook never builds a key by hand.
import type { Topic } from '@handline/client';
import { reads } from '#src/generated/reads.ts';

export type Key = readonly unknown[];

export interface ReadSpec {
  /** The second part of the key after the daemon id. */
  resource: string;
  /** The one param that names the entry (`agent.get` by agent); without it the whole params object is part of the key. */
  id?: string;
  /** The topic whose events keep this entry current; without one the entry is a snapshot and goes stale after 30 s. */
  feed?: Topic;
}

export { reads };

/** Reads that are not cache entries: the handshake, the subscription, the event log (the bridge reads events itself), the terminal stream, and the agent's own progress note (`hl progress`; no screen draws it). */
export const NOT_QUERIES = [
  'hello',
  'whoami',
  'subscribe',
  'unsubscribe',
  'events.feed',
  'progress.get',
  'task.memo.list', // the old name of task.comment.list; the screens read the new one
  // An image's bytes are read once per image into an object URL (attachments.ts), not kept as a cache entry.
  'attachment.get',
  // Logins found on this Mac are read when onboarding's AI step opens (useFoundLogins), not kept.
  'accounts.discover',
  'terminal.attach',
  'terminal.history',
  'terminal.detach',
  // A shared document is a stream like the terminal: the editor holds it (doc.open, doc.pull catch-up, doc.close).
  'doc.open',
  'doc.pull',
  'doc.close',
  // Reads no screen draws yet: the terminal's address (member) and the next account pick (agent).
  'terminal.open',
  'status.pick',
  // No screen draws a task screenshot yet.
  'task.shot',
  // `hl inbox` and `hl read`: an agent's queue and one request whole. The app draws the founder's inbox from inbox.list.
  'inbox.queue',
  'request.read',
  // `hl hook PermissionRequest`: an agent's session waiting on the founder's answer, which the app gives through prompt.answer.
  'permission.ask',
  // `hl use`: an agent's wait for a lease, its giving back and --extend; `hl team` and the hook read lease.get. No screen yet.
  'lease.take',
  'lease.release',
  'lease.extend',
  'lease.get',
  // `hl launch` asks the daemon for the command it becomes; not a read of state, and no screen calls it.
  'session.launch_prepare',
] as const;

export type ReadMethod = keyof typeof reads;

const hasParams = (p: unknown) => !!p && Object.values(p).some((v) => v !== undefined);

/** The cache key of one read: `['d', daemon, resource, id-or-params?]`. */
export function keyOf(daemon: string, method: ReadMethod, params?: unknown): Key {
  const spec: ReadSpec = reads[method];
  const base = ['d', daemon, spec.resource] as const;
  if (spec.id) return [...base, (params as Record<string, unknown>)[spec.id]];
  return hasParams(params) ? [...base, params] : base;
}

const resourceKey = (d: string, method: ReadMethod) => ['d', d, reads[method].resource] as const;

/** The named keys the event bridge and the writes patch. */
export const qk = {
  agents: (d: string) => keyOf(d, 'agents.list'),
  agent: (d: string, agent: string) => keyOf(d, 'agent.get', { agent }),
  status: (d: string) => keyOf(d, 'daemon.status'),
  accounts: (d: string) => keyOf(d, 'accounts.list'),
  /** Every inbox list, whatever its filter. */
  inboxes: (d: string) => resourceKey(d, 'inbox.list'),
  inbox: (d: string, filter?: object) => keyOf(d, 'inbox.list', filter),
  /** Every proposal list, whatever its filter. */
  hrs: (d: string) => resourceKey(d, 'proposal.list'),
  tasks: (d: string) => resourceKey(d, 'tasks.list'),
  task: (d: string, task: number) => keyOf(d, 'task.get', { task }),
  taskHistory: (d: string, task: number) => keyOf(d, 'task.history', { task }),
  taskTime: (d: string, task: number) => keyOf(d, 'task.time', { task }),
  memos: (d: string, task: number) => keyOf(d, 'task.comment.list', { task }),
  taskViews: (d: string) => keyOf(d, 'task.view.list'),
  prefs: (d: string) => keyOf(d, 'prefs.get'),
  prompts: (d: string, agent: string) => keyOf(d, 'prompt.list', { agent }),
  operation: (d: string, operation: string) => keyOf(d, 'operation.get', { operation }),
  chatMessagesOf: (d: string, agent: string) => [...resourceKey(d, 'chat.messages'), agent] as const,
  chatMessages: (d: string, agent: string, scope: string) => [...resourceKey(d, 'chat.messages'), agent, scope] as const,
  transcript: (d: string, agent: string) => keyOf(d, 'chat.transcript', { agent }),
  /** Everything of this daemon, for a gap or a reconnect. */
  all: (d: string) => ['d', d] as const,
  resource: resourceKey,
};
