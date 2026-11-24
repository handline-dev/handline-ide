// What the town draws, from what the app already reads: a house per workspace with its lead and its people, and the founder's decisions,
// each in the house of the agent that asks.
import type { AgentView, PendingRequest, Workspace } from '@handline/client';
import { activityOf, type Activity } from '#src/layout/agentList.ts';

export type Person = { key: string; name: string; activity: Activity; doing: string | null };
export type Decision = { id: string; task: number | null; title: string; agent: string | null; house: string | null };
export type House = { key: string; name: string; lead: Person | null; people: Person[]; working: number; decisions: Decision[] };
export type Town = { houses: House[]; decisions: Decision[] };

const personOf = (a: AgentView): Person => ({
  key: a.key,
  name: a.name,
  activity: activityOf(a),
  doing: a.task?.title ?? a.doing?.text ?? null,
});

/** The town from workspaces.list, agents.list and the founder's turn. A request whose agent is in no house is only in the founder's. */
export function townOf(workspaces: readonly Workspace[], agents: readonly AgentView[], turn: readonly PendingRequest[]): Town {
  const byKey = new Map(agents.map((a) => [a.key, a]));
  // `agent` is a key, or a name the daemon resolved; either finds the agent.
  const find = (who: string | null | undefined) => (who ? (byKey.get(who) ?? agents.find((a) => a.name === who)) : undefined);
  const decisions: Decision[] = turn.map((p, i) => {
    const a = find(p.agent);
    return {
      id: p.id ?? `${p.task ?? 'x'}-${i}`,
      task: p.task,
      title: p.title,
      agent: a?.name ?? p.agent ?? null,
      house: a?.workspace ?? null,
    };
  });
  const houses = workspaces.map((w): House => {
    const people = agents.filter((a) => a.workspace === w.key).map(personOf);
    const lead = people.find((p) => p.key === w.lead) ?? null;
    return {
      key: w.key,
      name: w.name,
      lead,
      people,
      working: people.filter((p) => p.activity === 'working').length,
      decisions: decisions.filter((d) => d.house === w.key),
    };
  });
  return { houses, decisions };
}
