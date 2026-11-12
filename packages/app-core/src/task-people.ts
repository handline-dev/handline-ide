// Who a task's detail offers: the people to assign it to (who acted on it, who shares its workspace, everyone else)
// and the people worth a reply (everyone the task's history names). Decided here so the screens only draw it.

export interface PeopleTask {
  from: string;
  to: string;
  events: readonly { at: string; agent?: string | null; actor?: { kind: string; name: string } | null }[];
  memos?: readonly { at: string; author: string }[];
}
/** An agent and the workspace it is in (AgentView.workspace). */
export interface PeopleAgent {
  key: string;
  workspace: string;
}
export interface AssignGroup {
  key: 'acted' | 'near' | 'rest';
  label: string;
  names: string[];
}

/** Names that are the system itself, not someone to talk to. */
const SYSTEM = new Set(['sys', 'schedule', 'system']);
/** The daemon's own senders (the task board's digests and nudges, the schedule): bookkeeping, not anyone to read or pick. */
export const isSystem = (name: string) => SYSTEM.has(name);

/** Everyone the task's history names, the most recent first, then its opener and holder; each once. */
export function involved(t: PeopleTask): string[] {
  const seen: { name: string; at: string }[] = [];
  for (const e of t.events)
    for (const name of [e.agent, e.actor && e.actor.kind !== 'daemon' ? e.actor.name : null]) if (name) seen.push({ name, at: e.at });
  for (const m of t.memos ?? []) seen.push({ name: m.author, at: m.at });
  const byTime = seen.sort((a, b) => (a.at < b.at ? 1 : a.at > b.at ? -1 : 0)).map((s) => s.name);
  return [...new Set([...byTime, t.to, t.from])].filter((n) => n && !SYSTEM.has(n));
}

/** How many agents of the task's workspace the short list shows after those who worked on the task; the others wait with the rest. */
export const NEAR_SHOWN = 6;

/** The assign picker: who acted on the task (latest first), then a few agents of the holder's or opener's workspace by name, then the rest by name. */
export function assignGroups(t: PeopleTask, agents: readonly PeopleAgent[], me: string): AssignGroup[] {
  const all = agents.map((a) => a.key).filter((n) => n !== me && !SYSTEM.has(n));
  const acted = involved(t).filter((n) => all.includes(n) && n !== t.to);
  const left = all.filter((n) => !acted.includes(n) && n !== t.to);
  const ws = new Map(agents.map((a) => [a.key, a.workspace]));
  const home = new Set([ws.get(t.to), ws.get(t.from)]);
  const near = left
    .filter((n) => home.has(ws.get(n)))
    .sort((a, b) => a.localeCompare(b))
    .slice(0, NEAR_SHOWN);
  const rest = left.filter((n) => !near.includes(n)).sort((a, b) => a.localeCompare(b));
  return [
    { key: 'acted' as const, label: 'Worked on this task', names: acted },
    { key: 'near' as const, label: 'Likely', names: near },
    { key: 'rest' as const, label: 'Other teams', names: rest },
  ].filter((g) => g.names.length);
}

/** The reply chips: the people the task names, other than the person looking. */
export const replyTargets = (t: PeopleTask, me: string): string[] => involved(t).filter((n) => n !== me && n !== 'human');
