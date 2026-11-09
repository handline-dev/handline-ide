// Under the Usage graph, one of four views at a time: the accounts, who used it (by agent), which models used it (each agent's use under
// the model it runs on now), and the cost per day stacked by agent. The figures are the daemon's (stats.get, stats.outlook).
import { costByAgent, spentText } from '@handline/app-core';
import type { Stats } from '@handline/client';
import { useAgents, useOutlook, useStats } from '@handline/app-data';
import { Bot, ChartColumn, ChevronRight, Cpu, Info, UserRound } from 'lucide-react';
import { HoverCard } from '#src/ui/HoverCard.tsx';
import { useState, type ReactNode } from 'react';
import { useTabState } from '#src/persist/tabState.ts';

/** The colours that tell agents apart in a stack (the chart series). */
const SERIES = ['bg-series-1', 'bg-series-2', 'bg-series-3', 'bg-series-4', 'bg-series-5', 'bg-series-6'];

export const VIEWS = ['accounts', 'agents', 'models', 'cost'] as const;
export type View = (typeof VIEWS)[number];
/** Each view's icon in the switch. */
export const VIEW_ICON: Record<View, ReactNode> = {
  accounts: <UserRound size={12} />,
  agents: <Bot size={12} />,
  models: <Cpu size={12} />,
  cost: <ChartColumn size={12} />,
};
export const VIEW_WORD: Record<View, string> = { accounts: 'Accounts', agents: 'Agents', models: 'Models', cost: 'Cost per day' };

/** A small switch of choices, the one in force raised. */
export function Switch<T extends string>({
  value,
  options,
  word,
  onChange,
  label,
  icon,
}: {
  value: T;
  options: readonly T[];
  word: (t: T) => string;
  onChange: (t: T) => void;
  label: string;
  /** An icon before each choice's word. */
  icon?: (t: T) => ReactNode;
}) {
  return (
    <div role="radiogroup" aria-label={label} className="flex w-fit gap-0.5 rounded-lg bg-surface p-1">
      {options.map((o) => (
        <button
          key={o}
          type="button"
          role="radio"
          aria-checked={o === value}
          onClick={() => onChange(o)}
          className="flex h-6 items-center justify-center gap-1.5 rounded-sm px-2 text-xs text-muted hover:text-fg aria-checked:bg-bg aria-checked:text-fg"
        >
          {icon?.(o)}
          {word(o)}
        </button>
      ))}
    </div>
  );
}

export type Win = Stats['window'];
export const WINDOWS: readonly Win[] = ['24h', '7d'];

/** A list's rows while it loads: the shape of a name and a bar each, pulsing. */
export function RowsLoading({ rows = 5 }: { rows?: number }) {
  return (
    <div className="flex flex-col" aria-busy="true" aria-label="Loading">
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="flex h-14 animate-pulse items-center gap-4 border-b border-border">
          <div className="flex w-44 flex-col gap-1.5">
            <span className="h-3 w-24 rounded-sm bg-surface" />
            <span className="h-2.5 w-16 rounded-sm bg-surface" />
          </div>
          <span className="h-1.5 flex-1 rounded-full bg-surface" />
          <span className="h-3 w-24 rounded-sm bg-surface" />
        </div>
      ))}
    </div>
  );
}

/** A row of a ranked list: its name, the figures, and a bar of its share. */
function Ranked({
  name,
  sub,
  figures,
  fill,
  tone = 'bg-accent',
}: {
  name: ReactNode;
  sub?: string;
  figures: string;
  fill: number;
  tone?: string;
}) {
  return (
    <div className="flex items-center gap-4 border-b border-border py-2 text-sm">
      <div className="flex w-44 min-w-0 flex-col">
        <span className="truncate text-fg">{name}</span>
        {sub && <span className="truncate text-xs text-muted">{sub}</span>}
      </div>
      <div className="h-1.5 min-w-20 flex-1 overflow-hidden rounded-full bg-surface">
        <div className={`h-full ${tone}`} style={{ width: `${Math.min(1, fill) * 100}%` }} />
      </div>
      <span className="w-36 shrink-0 text-right text-muted tabular-nums">{figures}</span>
    </div>
  );
}

/** Calls of no agent: the founder's own sessions and the usage poll. */
const NO_AGENT = 'Your own sessions';

type Use = { cost: number; week: number | null };
const add = (a: Use, b: { cost: number; week_pct: number | null }): Use => ({
  cost: a.cost + b.cost,
  week: b.week_pct === null ? a.week : (a.week ?? 0) + b.week_pct,
});
const size = (u: Use) => u.week ?? u.cost;

/** Who used it in the window, as a tree: each team (an agent and the agents named under it, `handline` and `handline.*`) with its total,
 * and under it each agent with its total and, in one line, the accounts it ran on. A team opens and closes. */
export function ByAgent({ win }: { win: Win }) {
  const s = useStats(win).data;
  const [shut, setShut] = useTabState<string[]>('shut', []);
  type Agent = Use & { name: string; accounts: { account: string; use: Use }[] };
  const agents = new Map<string, Agent>();
  for (const u of s?.use ?? []) {
    const name = u.agent || NO_AGENT;
    const a = agents.get(name) ?? { name, cost: 0, week: null, accounts: [] };
    Object.assign(a, add(a, u));
    a.accounts.push({ account: u.account, use: add({ cost: 0, week: null }, u) });
    agents.set(name, a);
  }
  const teams = new Map<string, Use & { agents: Agent[] }>();
  for (const a of agents.values()) {
    const team = a.name === NO_AGENT ? NO_AGENT : a.name.split('.')[0]!;
    const t = teams.get(team) ?? { cost: 0, week: null, agents: [] };
    t.cost += a.cost;
    if (a.week !== null) t.week = (t.week ?? 0) + a.week;
    t.agents.push(a);
    teams.set(team, t);
  }
  // The use of no agent is one plain row at the end, not a team.
  const mine = teams.get(NO_AGENT);
  teams.delete(NO_AGENT);
  const ordered = [...teams].sort((x, y) => size(y[1]) - size(x[1]));
  const top = Math.max(1e-9, ...ordered.map(([, t]) => size(t)));
  const figures = (u: Use) => spentText({ weekPct: u.week, cost: u.cost });
  return (
    <div className="flex flex-col gap-2">
      {!s && <RowsLoading />}
      {s && !ordered.length && <p className="text-sm text-muted">Nothing used in this window.</p>}
      <div className="flex flex-col">
        {ordered.map(([team, t]) => {
          const open = !shut.includes(team);
          return (
            <div key={team} className="flex flex-col">
              <button
                type="button"
                aria-expanded={open}
                onClick={() => setShut((x) => (x.includes(team) ? x.filter((n) => n !== team) : [...x, team]))}
                className="flex items-center gap-1 text-left hover:bg-hover/30"
              >
                <ChevronRight size={14} className={`shrink-0 text-muted transition-transform ${open ? 'rotate-90' : ''}`} />
                <div className="min-w-0 flex-1">
                  <Ranked name={<span className="font-medium">{team}</span>} figures={figures(t)} fill={size(t) / top} />
                </div>
              </button>
              {open &&
                t.agents
                  .sort((x, y) => size(y) - size(x))
                  .map((a) => (
                    <div key={a.name} className="pl-5">
                      <Ranked
                        name={a.name === team ? `${a.name} (itself)` : a.name}
                        sub={a.accounts
                          .sort((x, y) => size(y.use) - size(x.use))
                          .map((c) => `${c.account} ${figures(c.use)}`)
                          .join(' · ')}
                        figures={figures(a)}
                        fill={size(a) / top}
                        tone="bg-muted"
                      />
                    </div>
                  ))}
            </div>
          );
        })}
      </div>
      {mine && (
        <div className="pl-5" title="Your Claude sessions outside any agent, and the usage poll">
          <Ranked
            name={NO_AGENT}
            sub="Outside any agent, and the usage poll"
            figures={figures(mine)}
            fill={size(mine) / top}
            tone="bg-muted"
          />
        </div>
      )}
    </div>
  );
}

/** Which models used it: each agent's use put under the model it runs on now (the daemon does not record a model per call). */
export function ByModel({ win }: { win: Win }) {
  const s = useStats(win).data;
  const agents = useAgents().data?.agents ?? [];
  const modelOf = new Map(agents.map((a) => [a.name, [a.model, a.effort].filter(Boolean).join(' ') || 'Unknown']));
  const by = new Map<string, { cost: number; week: number | null; agents: Set<string> }>();
  for (const u of s?.use ?? []) {
    const m = modelOf.get(u.agent) ?? 'Unknown';
    const v = by.get(m) ?? { cost: 0, week: null, agents: new Set<string>() };
    v.cost += u.cost;
    if (u.week_pct !== null) v.week = (v.week ?? 0) + u.week_pct;
    if (u.agent) v.agents.add(u.agent);
    by.set(m, v);
  }
  const rows = [...by].sort((a, b) => (b[1].week ?? b[1].cost) - (a[1].week ?? a[1].cost));
  const top = Math.max(1, ...rows.map(([, v]) => v.week ?? v.cost));
  return (
    <div className="flex flex-col gap-2">
      {!s && <RowsLoading />}
      <div className="flex flex-col">
        {rows.map(([m, v]) => (
          <Ranked
            key={m}
            name={m}
            sub={`${v.agents.size} agents`}
            figures={spentText({ weekPct: v.week, cost: v.cost })}
            fill={(v.week ?? v.cost) / top}
          />
        ))}
      </div>
      <p className="text-xs text-muted">Each agent counts under the model it runs on now.</p>
    </div>
  );
}

export type CostRange = '7d' | '14d' | '30d';
export const COST_RANGES: readonly CostRange[] = ['7d', '14d', '30d'];

/** The daemon's name for cost whose folder no agent's worktree holds now. */
const UNMATCHED = '(unattributed)';
const costName = (a: string) => (a === UNMATCHED ? 'Removed or outside' : a || 'Your own sessions');

/** The unmatched cost's name, with what it is on hover. */
function Unmatched() {
  return (
    <span className="flex items-center gap-1">
      Removed or outside
      <HoverCard
        trigger={
          <button
            type="button"
            aria-label="What this is"
            className="grid size-5 place-items-center rounded-sm text-muted hover:bg-hover/30 hover:text-fg"
          >
            <Info size={13} />
          </button>
        }
      >
        <p className="w-56 p-2 text-sm">Removed agents, moved worktrees, or Claude used outside Handline.</p>
      </HoverCard>
    </span>
  );
}

/** The cost of each day stacked by agent, and under it each agent's total; agents in the chart colours. */
export function CostPerDay({ range }: { range: CostRange }) {
  const [hover, setHover] = useState<{ day: number; agent: string } | null>(null);
  const o = useOutlook(range).data;
  if (!o)
    return (
      <div className="flex flex-col gap-2">
        <div className="h-48 animate-pulse rounded-md bg-surface" />
        <RowsLoading rows={4} />
      </div>
    );
  const { agents, sums, all } = costByAgent(o);
  const order = agents.map((a, i) => ({ a, sum: sums[i]! })).sort((x, y) => y.sum - x.sum);
  const tone = new Map(order.map((x, i) => [x.a, SERIES[i % SERIES.length]!]));
  const dayTotal = (d: (typeof o.cost_per_day)[number]) => agents.reduce((n, a) => n + (d.agents[a] ?? 0), 0);
  const top = Math.max(1, ...o.cost_per_day.map(dayTotal));
  return (
    <div className="flex flex-col gap-3">
      {/* Hovering a piece reads it beside the pointer: whose, how much, and the day's total; the other agents dim. */}
      <div
        className="relative flex h-48 items-end gap-1"
        role="img"
        aria-label="Cost per day, by agent"
        onMouseLeave={() => setHover(null)}
      >
        {o.cost_per_day.map((d, i) => (
          <div key={d.day} className="flex h-full min-w-0 flex-1 flex-col justify-end gap-1">
            <div className="flex flex-col-reverse overflow-hidden rounded-sm" style={{ height: `${(dayTotal(d) / top) * 100}%` }}>
              {order.map(({ a }) => (
                <span
                  key={a}
                  onMouseEnter={() => setHover({ day: i, agent: a })}
                  className={`${tone.get(a)} transition-opacity ${hover && hover.agent !== a ? 'opacity-30' : ''}`}
                  style={{ flexGrow: d.agents[a] ?? 0 }}
                />
              ))}
            </div>
            <span className="truncate text-center text-xs text-muted">{d.day.slice(5)}</span>
          </div>
        ))}
        {hover &&
          (() => {
            const d = o.cost_per_day[hover.day]!;
            const at = ((hover.day + 0.5) / o.cost_per_day.length) * 100;
            return (
              <div
                className="pointer-events-none absolute top-0 z-10 flex flex-col rounded-md border border-border bg-bg px-2 py-1 text-xs whitespace-nowrap shadow-popover"
                style={{ left: `${at}%`, transform: at > 70 ? 'translateX(calc(-100% - 8px))' : 'translateX(8px)' }}
              >
                <span className="flex items-center gap-1.5 text-fg">
                  <span className={`size-2 rounded-sm ${tone.get(hover.agent)}`} />
                  {costName(hover.agent)} ${(d.agents[hover.agent] ?? 0).toFixed(2)}
                </span>
                <span className="text-muted">
                  {d.day} · ${dayTotal(d).toFixed(2)} that day
                </span>
              </div>
            );
          })()}
      </div>
      <div className="flex flex-col">
        {order.map(({ a, sum }) => (
          <Ranked
            key={a}
            name={a === UNMATCHED ? <Unmatched /> : costName(a)}
            figures={`$${sum.toFixed(2)} · ${Math.round((sum / (all || 1)) * 100)}%`}
            fill={sum / (order[0]?.sum || 1)}
            tone={tone.get(a)}
          />
        ))}
      </div>
    </div>
  );
}
