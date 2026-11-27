// What the town draws, from what the app already reads: a house per workspace with its lead and its people, and the founder's decisions,
// each in the house of the agent that asks.
import type { AgentView, PendingRequest, Workspace } from '@handline/client';
import { activityOf, holdLine, type Activity } from '#src/layout/agentList.ts';

/** What a person is doing, in a word, under their name. */
export const WORDS: Record<Activity, string> = {
  working: 'Working',
  waking: 'Starting',
  asleep: 'Asleep',
  you: 'Waiting for you',
  held: 'On hold',
  error: 'Stopped',
  resting: 'Idle',
};

/** `queue`: the tasks it owns that are not done; `cached`: its prompt cache still holds its conversation; `task`: the task it holds now;
 *  `why`: why it waits or stopped, in the daemon's words. For its card: `team`, its workspace's name; `role`, the first line of its role
 *  (empty for none); its `model` and `effort`; `ctx`, the whole percent of its context window in use, null when not known. */
export type Person = {
  key: string;
  name: string;
  activity: Activity;
  doing: string | null;
  queue: number;
  cached: boolean;
  task?: number | null;
  why?: string | null;
  team?: string;
  role?: string;
  model?: string | null;
  effort?: string | null;
  ctx?: number | null;
};
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

const personOf = (a: AgentView, now: number, team: string): Person => ({
  key: a.key,
  name: a.name,
  activity: activityOf(a),
  doing: a.task?.title ?? a.doing?.text ?? null,
  queue: a.task_count ?? 0,
  cached: !!a.cache_until && Date.parse(a.cache_until) > now,
  task: a.task?.id ?? null,
  why: holdLine(a),
  team,
  role: a.role?.trim() ?? '',
  model: a.model ?? null,
  effort: a.effort ?? null,
  ctx: a.ctx_pct ?? null,
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
    const people = agents.filter((a) => a.workspace === w.key).map((a) => personOf(a, now, w.name));
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
