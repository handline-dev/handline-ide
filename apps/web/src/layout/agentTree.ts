/** The sidebar's agent tree: each agent under the agent it reports to (`parent`). Siblings stand in the
 * person's `order` (nav prefs); one not in it comes after, by name. */
export type AgentLike = { name: string; parent: string | null; state: string };
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

/** Which of four the agent is in, from the wire `state` (x-state-words; an unknown word is resting):
 * working (running, or about to), asleep (parked for a time, or no terminal), in trouble (stalled, blocked,
 * or a person must answer), or resting (idle at its prompt). */
export type Activity = 'working' | 'asleep' | 'error' | 'resting';
export function activityOf(state: string): Activity {
  if (state === 'running' || state === 'starting' || state === 'queued') return 'working';
  if (state === 'parked' || state === 'no_terminal') return 'asleep';
  if (state === 'stalled' || state === 'blocked' || state === 'waiting') return 'error';
  return 'resting';
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
  return nodes.reduce((n, c) => n + (activityOf(c.agent.state) === 'working' ? 1 : 0) + workingUnder(c.children), 0);
}
