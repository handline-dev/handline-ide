// A task's plan as task.get draws it (TaskDetail.plan_state, plan, plan_ask, core_ask), from what the fake keeps on the row.
import type { TaskDetail } from '@handline/contract';

export type PlanVersion = { text: string; by: string; at: string };
export type PlanAsk = { at: string; by: string; to: string; sent_at: string | null };
/** What a row keeps of its plan: its versions oldest first, the OK of the last one, and the open asks. */
export type PlanRow = {
  plan?: { versions: PlanVersion[]; ok: { by: string; at: string } | null };
  planAsk?: PlanAsk | null;
  coreAsk?: (PlanAsk & { note: string | null }) | null;
};
type PlanFields = Pick<TaskDetail, 'plan_state' | 'plan' | 'plan_ask' | 'core_ask'>;

export function planState(r: PlanRow): TaskDetail['plan_state'] {
  if (r.planAsk) return r.planAsk.sent_at ? 'writing' : 'asked';
  if (!r.plan) return 'none';
  return r.plan.ok ? 'ok' : 'written';
}

export function planFields(r: PlanRow): PlanFields {
  const versions = r.plan?.versions ?? [];
  const cur = versions.at(-1);
  return {
    plan_state: planState(r),
    plan: cur ? { ...cur, version: versions.length, ok: r.plan!.ok, versions } : null,
    plan_ask: r.planAsk ?? null,
    core_ask: r.coreAsk ?? null,
  };
}
