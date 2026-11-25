// What the town draws, from what the app already reads: a house per workspace with its lead and its people, and the founder's decisions,
// each in the house of the agent that asks.
import type { AgentView, PendingRequest, Workspace } from '@handline/client';
import { activityOf, type Activity } from '#src/layout/agentList.ts';

/** `queue`: the tasks it owns that are not done; `cached`: its prompt cache still holds its conversation. */
export type Person = { key: string; name: string; activity: Activity; doing: string | null; queue: number; cached: boolean };
export type Decision = { id: string; task: number | null; title: string; agent: string | null; house: string | null };
/** `hq`: the founder's HQ, where the pinned agents gather and every decision lands. */
export type House = {
  key: string;
  name: string;
  lead: Person | null;
  people: Person[];
  working: number;
  decisions: Decision[];
  hq?: boolean;
};
export type Town = { houses: House[]; decisions: Decision[] };

const personOf = (a: AgentView, now: number): Person => ({
  key: a.key,
  name: a.name,
  activity: activityOf(a),
  doing: a.task?.title ?? a.doing?.text ?? null,
  queue: a.task_count ?? 0,
  cached: !!a.cache_until && Date.parse(a.cache_until) > now,
});

/** The town from workspaces.list, agents.list and the founder's turn, at `now`. A request whose agent is in no house is only in the
 *  founder's. */
export function townOf(
  workspaces: readonly Workspace[],
  agents: readonly AgentView[],
  turn: readonly PendingRequest[],
  now = Date.now(),
): Town {
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
    const people = agents.filter((a) => a.workspace === w.key).map((a) => personOf(a, now));
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
