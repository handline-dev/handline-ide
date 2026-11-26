// Usage in plan money: each account's weekly budget is its monthly fee x 12 / 52, and what is used of it is its week percent of that. Pure.
import type { Outlook } from '@handline/client';

type Budget = NonNullable<Outlook['budget']>;

const usd = (n: number) => `$${n.toFixed(2)}`;

/** The top line's money: "$31.20 of $46.15 left this week"; null with no account that counts. */
export function budgetLeftWords(b: { week_usd: number; left_usd: number | null } | null | undefined): string | null {
  if (!b || !(b.week_usd > 0) || b.left_usd === null) return null;
  return `${usd(b.left_usd)} of ${usd(b.week_usd)} left this week`;
}

/** The weekly budget of each account, by name, for turning a week percent into plan money. */
export const weekBudgets = (b: Pick<Budget, 'accounts'> | null | undefined): Map<string, number> =>
  new Map((b?.accounts ?? []).map((a) => [a.account, a.week_usd]));

/** An agent's plan money on an account: its share of the week times the account's weekly budget; 0 with no share or no budget. */
export const planUsd = (weekPct: number | null, weekUsd: number | undefined): number =>
  weekPct === null || !weekUsd ? 0 : (weekPct / 100) * weekUsd;
