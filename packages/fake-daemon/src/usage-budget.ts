// stats.outlook's plan money: each account's weekly budget (its monthly fee x 12 / 52) and what its week percent used spends of it, as
// the account rows read (alpha and beta Pro, kappa an Ultra with no 5-hour window), and the plan money per day. Beside it, every case of
// extra usage a page draws: alpha has it on and spent some this month, beta has it on with nothing spent, kappa has it off.
import type { Outlook } from '@handline/client';

const WEEK = 12 / 52;
type Extra = NonNullable<Outlook['budget']>['accounts'][number]['extra'];
const ROWS: [account: string, plan: string, fee: number, used: number, five: boolean, extra: Extra][] = [
  ['alpha', 'Pro', 20, 62, true, { on: true, cap: 50, spent: 14.5, currency: 'USD' }],
  ['beta', 'Pro', 20, 30, true, { on: true, cap: null, spent: 0, currency: 'USD' }],
  ['kappa', 'Ultra', 200, 40, false, { on: false, cap: null, spent: null, currency: null }],
];
const cents = (n: number) => Math.round(n * 100) / 100;

export function budget(): NonNullable<Outlook['budget']> {
  const accounts = ROWS.map(([account, plan, fee, used, five, extra]) => {
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
      extra,
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
    extra_spent: { USD: 14.5 },
  };
}

/** The plan money per day: the list-price days scaled down, as plan money runs about seventy times under list price; alpha's $14.50 of
 *  extra usage was spent on the last two days. */
export function planCostPerDay(days: Outlook['cost_per_day']): NonNullable<Outlook['plan_cost_per_day']> {
  const last = days.length - 1;
  return days.map((d, i) => {
    const extra: Record<string, number> = {};
    if (i >= last - 1) extra.USD = i === last ? 4.5 : 10;
    return { day: d.day, agents: Object.fromEntries(Object.entries(d.agents).map(([a, v]) => [a, cents(v / 70)])), extra };
  });
}
