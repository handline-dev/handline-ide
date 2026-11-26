// What a task cost (task.get's spend), in the words a task page shows. Pure.
import type { TaskDetail } from '@handline/client';
import { spentText } from '#src/stats.ts';

export type TaskSpend = NonNullable<TaskDetail['spend']>;

/** Dollars as "$4.20"; an amount that rounds to no cent is "under $0.01", never a zero that reads as free. */
export const usdText = (usd: number): string => (usd > 0 && usd < 0.005 ? 'under $0.01' : spentText({ weekPct: null, cost: usd }));

/** The one line by a task's clock. The calls are priced at list price, which a subscription does not pay, so the line says whose price it is. */
export const taskCostText = (s: Pick<TaskSpend, 'usd'>): string => `${usdText(s.usd)} at API prices`;

/** One side of the breakdown (by agent, or by model), largest first and by name among equals: each key's dollars and its share of the whole. */
export function spendShares(by: Record<string, number>): { key: string; usd: number; share: number }[] {
  const all = Object.values(by).reduce((sum, usd) => sum + usd, 0);
  return Object.entries(by)
    .map(([key, usd]) => ({ key, usd, share: all > 0 ? usd / all : 0 }))
    .sort((a, b) => b.usd - a.usd || a.key.localeCompare(b.key));
}
