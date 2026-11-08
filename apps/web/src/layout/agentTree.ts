import { dotOf, whyOf } from '@handline/app-core';
/** The sidebar's agent tree: each agent under the agent it reports to (`parent`). Siblings stand in the
 * person's `order` (nav prefs); one not in it comes after, by name. */
type DotFacts = Parameters<typeof dotOf>[0];
export type AgentLike = { name: string; parent: string | null; state: string } & Partial<Omit<DotFacts, 'state'>>;
export type AgentNode<A extends AgentLike = AgentLike> = { agent: A; children: AgentNode<A>[] };

export function agentTree<A extends AgentLike>(agents: readonly A[], order: readonly string[] = []): AgentNode<A>[] {
  const rank = (n: string) => {
    const i = order.indexOf(n);
    return i < 0 ? Infinity : i;
  };
  const names = new Set(agents.map((a) => a.name));
  const kids = new Map<string | null, A[]>();
  for (const a of agents) {
    // A parent the list does not hold (removed, or another team's) puts the agent at the top level.
    const p = a.parent && names.has(a.parent) && a.parent !== a.name ? a.parent : null;
    kids.set(p, [...(kids.get(p) ?? []), a]);
  }
  const seen = new Set<string>();
  const build = (parent: string | null): AgentNode<A>[] =>
    (kids.get(parent) ?? [])
      .filter((a) => !seen.has(a.name) && seen.add(a.name))
      .sort((x, y) => rank(x.name) - rank(y.name) || x.name.localeCompare(y.name))
      .map((agent) => ({ agent, children: build(agent.name) }));
  return build(null);
}

/** Which mark the agent shows, from the daemon's status: working, asleep, you (a person must answer),
 * held (a wait by design), error (a real fault), or resting. */
export type Activity = 'working' | 'asleep' | 'you' | 'held' | 'error' | 'resting';
export function activityOf(a: Pick<AgentLike, keyof DotFacts>): Activity {
  // From the rail dot, which reads AgentView.status first: an agent at its prompt with background work running is working.
  const dot = dotOf({ ...a, has_dialog: a.has_dialog ?? false });
  if (dot === 'running' || dot === 'waking') return 'working';
  if (dot === 'parked' || dot === 'none') return 'asleep';
  if (dot === 'waiting') return 'you';
  // A usage wall or the account cap ends by itself, so it is a wait too.
  if (dot === 'blocked') return a.status?.state === 'waiting' || ['wall', 'cap'].includes(a.status?.reason ?? '') ? 'held' : 'error';
  return 'resting';
}

/** Why the agent's mark is not plain, in one line: the tasks it waits on, else the daemon's status line, else
 * its block's words; null while it works or rests. */
export function holdLine(a: Pick<AgentLike, keyof DotFacts> & Partial<Parameters<typeof whyOf>[0]>): string | null {
  const act = activityOf(a);
  if (act !== 'you' && act !== 'held' && act !== 'error') return null;
  const tasks = a.status?.reason === 'task' ? a.status.tasks : [];
  if (tasks.length) return `Waiting on ${tasks.map((t) => `T${t}`).join(', ')}`;
  const line = a.status?.text.trim() || whyOf({ blocker: a.blocker, wall_reason: a.wall_reason, state_detail: a.state_detail ?? '' });
  return line ? line[0]!.toUpperCase() + line.slice(1) : null;
}

/** Splits the tree for the sidebar: the pinned agents in pin order, each with its own sub-agents, and the rest of the tree without them. */
export function splitPinned<A extends AgentLike>(tree: AgentNode<A>[], pins: readonly string[]) {
  const pinned: AgentNode<A>[] = [];
  const take = (nodes: AgentNode<A>[]): AgentNode<A>[] =>
    nodes.flatMap((n) => {
      const kept = { ...n, children: take(n.children) };
      if (!pins.includes(n.agent.name)) return [kept];
      pinned.push({ ...n, children: unpinned(n.children) });
      return [];
    });
  // A pinned team leaves out its own sub-agents that are pinned: they show once, on their own line.
  const unpinned = (nodes: AgentNode<A>[]): AgentNode<A>[] =>
    nodes.filter((c) => !pins.includes(c.agent.name)).map((c) => ({ ...c, children: unpinned(c.children) }));
  const rest = take(tree);
  pinned.sort((x, y) => pins.indexOf(x.agent.name) - pins.indexOf(y.agent.name));
  return { pinned, rest };
}

/** Only the branch down to `name`: what a folded team still shows, so the picked agent stays in sight. */
export function pathTo<A extends AgentLike>(nodes: AgentNode<A>[], name: string | null): AgentNode<A>[] {
  if (!name) return [];
  return nodes.flatMap((n) => {
    if (n.agent.name === name) return [{ ...n, children: [] }];
    const below = pathTo(n.children, name);
    return below.length ? [{ ...n, children: below }] : [];
  });
}

/** A name shown under `parent` drops the parent's prefix: under a, a.b is b and a.b.c is b.c. */
export function shortName(name: string, parent: string | null): string {
  return parent && name.startsWith(`${parent}.`) ? name.slice(parent.length + 1) : name;
}

/** How many agents under these nodes, at any depth, are working. */
export function workingUnder(nodes: AgentNode[]): number {
  return nodes.reduce((n, c) => n + (activityOf(c.agent) === 'working' ? 1 : 0) + workingUnder(c.children), 0);
}
