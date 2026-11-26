// stats.outlook's plan money: each account's weekly budget (its monthly fee x 12 / 52) and what its week percent used spends of it, as
// the account rows read (alpha and beta Pro, kappa an Ultra with no 5-hour window), and the plan money per day.
import type { Outlook } from '@handline/client';

const WEEK = 12 / 52;
const ROWS: [account: string, plan: string, fee: number, used: number, five: boolean][] = [
  ['alpha', 'Pro', 20, 62, true],
  ['beta', 'Pro', 20, 30, true],
  ['kappa', 'Ultra', 200, 40, false],
];
const cents = (n: number) => Math.round(n * 100) / 100;

export function budget(): NonNullable<Outlook['budget']> {
  const accounts = ROWS.map(([account, plan, fee, used, five]) => {
    const week = cents(fee * WEEK);
    const spent = cents((week * used) / 100);
    return {
      account,
      plan,
      fee_usd: fee,
      week_usd: week,
      used_pct: used,
      spent_usd: spent,
      left_usd: cents(week - spent),
      five_hour: five,
    };
  });
  const week_usd = cents(accounts.reduce((n, a) => n + a.week_usd, 0));
  const spent_usd = cents(accounts.reduce((n, a) => n + a.spent_usd, 0));
  return {
    accounts,
    week_usd,
    spent_usd,
    left_usd: cents(week_usd - spent_usd),
    left_pct: Math.round(((week_usd - spent_usd) / week_usd) * 100),
  };
}

/** The plan money per day: the list-price days scaled down, as plan money runs about seventy times under list price. */
export const planCostPerDay = (days: Outlook['cost_per_day']): NonNullable<Outlook['plan_cost_per_day']> =>
  days.map((d) => ({ day: d.day, agents: Object.fromEntries(Object.entries(d.agents).map(([a, v]) => [a, cents(v / 70)])) }));
