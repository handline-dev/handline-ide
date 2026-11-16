// The "Who used it" lines of the Stats page and the numbers beside each. Pure.

/** One line of "Who used it": an agent's use of one account; its bar is the share of a full weekly limit, empty without a reading. */
export type UseLine = { key: string; agent: string; account: string | null; weekPct: number | null; cost: number; fill: number };

/** The numbers beside a line: the share of the weekly limit, then what the calls cost at list price. */
export const spentText = (r: Pick<UseLine, 'weekPct' | 'cost'>): string => {
  const cost = `$${r.cost.toFixed(2)}`;
  return r.weekPct === null ? cost : `${r.weekPct < 10 ? Math.round(r.weekPct * 10) / 10 : Math.round(r.weekPct)}% · ${cost}`;
};
