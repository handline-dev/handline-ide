// A workspace at a glance: what needs you, five numbers, and who waits on whom; its folders and rules under Settings.
import { type RuleRow, useAgents, useRead, useTasks, useWorkspaceRules } from '@handline/app-data';
import type { AgentView as Agent, PendingRequest, Stats, Task } from '@handline/client';
import type { ReactNode } from 'react';
import { useTabState } from '#src/persist/tabState.ts';
import { activityOf } from '#src/layout/agentList.ts';
import { useNow } from '#src/format/time.ts';
import { useOpenable } from '#src/layout/openable.tsx';
import { PickProvider } from '#src/requests/RowMenu.tsx';
import { TurnLine } from '#src/requests/rows.tsx';
import { Folders } from '#src/WorkspaceFolders.tsx';
import { WorkspaceGraph } from '#src/workspace/WorkspaceGraph.tsx';

const WEEK = 7 * 86_400_000;
/** Needs you shows the oldest few; the rest are in Requests. */
const SHOWN = 5;

function AllRequests({ more }: { more: number }) {
  const open = useOpenable('requests');
  return (
    <button type="button" {...open} className="self-start py-2 text-sm text-accent outline-none hover:underline focus-visible:underline">
      {more} more in Requests
    </button>
  );
}

/** The five numbers, from this workspace's agents and tasks. */
export function numbersOf(
  agents: readonly Agent[],
  open: readonly Task[],
  closed: readonly Task[],
  stats: Stats | undefined,
  needs: readonly PendingRequest[],
  now: number,
) {
  const keys = new Set(agents.map((a) => a.key));
  const acts = agents.map(activityOf);
  const on = (w: string) => open.filter((t) => t.waiting?.on === w).length;
  const week = stats?.use.filter((u) => keys.has(u.agent) && u.week_pct !== null) ?? [];
  return {
    landed: closed.filter((t) => t.state === 'done' && t.done && now - Date.parse(t.done) < WEEK).length,
    working: acts.filter((a) => a === 'working' || a === 'waking').length,
    idle: acts.filter((a) => a === 'asleep' || a === 'resting').length,
    stopped: acts.filter((a) => a === 'error').length,
    waiting: acts.filter((a) => a === 'you' || a === 'held').length,
    // A task with a row in Needs you waits on you, whatever else holds it.
    onYou: new Set([
      ...needs.flatMap((p) => (p.task === null ? [] : [p.task])),
      ...open.filter((t) => t.waiting?.on === 'founder').map((t) => t.id),
    ]).size,
    onTask: on('task'),
    wait: stats?.your_wait_s ?? null,
    week: week.length ? week.reduce((s, u) => s + (u.week_pct ?? 0), 0) : null,
  };
}

/** Seconds as the shortest whole unit: 45s, 18m, 3h, 2d. */
export const span = (s: number) =>
  s < 60 ? `${s}s` : s < 3600 ? `${Math.round(s / 60)}m` : s < 86_400 ? `${Math.round(s / 3600)}h` : `${Math.round(s / 86_400)}d`;

function Tile({ label, value, sub }: { label: string; value: ReactNode; sub: string }) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5 rounded-md border border-border px-3 py-2.5">
      <span className="truncate text-xs text-muted">{label}</span>
      <span className="text-xl font-semibold tabular-nums text-fg">{value}</span>
      <span className="truncate text-xs text-muted">{sub}</span>
    </div>
  );
}

function Heading({ children }: { children: ReactNode }) {
  return <h2 className="sticky top-0 z-10 bg-bg py-2 text-sm font-medium text-fg">{children}</h2>;
}

function Overview({ ws }: { ws: string }) {
  const agents = (useAgents().data?.agents ?? []).filter((a) => a.workspace === ws);
  const keys = new Set(agents.map((a) => a.key));
  const mine = (t: Task) => keys.has(t.to);
  const open = (useTasks({ state: 'open' }).data?.items ?? []).filter(mine);
  const closed = (useTasks({ state: 'closed', order: 'closed' }).data?.items ?? []).filter(mine);
  const stats = useRead('stats.get', { window: '7d' }).data;
  const needs = (useRead('requests.pending', {}).data?.answer ?? [])
    .filter((p: PendingRequest) => p.agent && keys.has(p.agent))
    .sort((a, b) => a.last_at.localeCompare(b.last_at));
  const n = numbersOf(agents, open, closed, stats, needs, useNow());
  return (
    <div className="flex flex-col gap-6">
      <section>
        <Heading>
          Needs you, oldest first <span className="font-normal text-muted tabular-nums">{needs.length}</span>
        </Heading>
        {needs.length ? (
          <PickProvider>
            <div className="flex flex-col text-sm">
              {needs.slice(0, SHOWN).map((p) => (
                <TurnLine key={`${p.task}:${p.id}`} p={p} />
              ))}
              {needs.length > SHOWN && <AllRequests more={needs.length - SHOWN} />}
            </div>
          </PickProvider>
        ) : (
          <p className="py-2 text-sm text-muted">Nothing waits on you here.</p>
        )}
      </section>
      <section className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
        <Tile label="Landed this week" value={n.landed} sub="tasks done" />
        <Tile
          label="Working now"
          value={`${n.working} / ${agents.length}`}
          sub={`${n.waiting} waiting · ${n.idle} idle · ${n.stopped} stopped`}
        />
        <Tile label="Tasks blocked" value={n.onYou + n.onTask} sub={`${n.onYou} on you · ${n.onTask} on a task`} />
        <Tile label="Your wait" value={n.wait === null ? '–' : span(n.wait)} sub="median before you answer" />
        <Tile label="Week used" value={n.week === null ? '–' : `${Math.round(n.week)}%`} sub="this workspace's part of it" />
      </section>
      <section>
        <Heading>
          Who waits on whom <span className="font-normal text-muted">an arrow: waits on · a dot: said something to you, unread</span>
        </Heading>
        <WorkspaceGraph agents={agents} open={open} asking={new Set(needs.flatMap((p) => (p.agent ? [p.agent] : [])))} />
      </section>
    </div>
  );
}

const RULE_NAMES: Record<RuleRow['key'], string> = {
  accounts: 'Accounts',
  agent_cap: 'Agents at once',
  tools_deny: 'Tools denied',
  mode_max: 'Most permissive mode',
  review_required: 'Review required',
  land_by: 'Lands by',
  land_check: 'Check before landing',
};

const valueOf = (r: RuleRow) =>
  !r.on ? 'None' : Array.isArray(r.value) ? r.value.join(', ') : typeof r.value === 'boolean' ? (r.value ? 'Yes' : 'No') : String(r.value);

function Settings({ ws }: { ws: string }) {
  const rules = useWorkspaceRules(ws).data?.rules ?? [];
  return (
    <div className="flex flex-col gap-6">
      <section>
        <Heading>Folders</Heading>
        <Folders wsKey={ws} />
      </section>
      <section>
        <Heading>Rules</Heading>
        <div className="flex flex-col border-t border-border text-sm">
          {rules.map((r) => (
            <div key={r.key} className="flex min-h-10 items-center justify-between gap-3 border-b border-border py-2">
              <span className="text-fg">{RULE_NAMES[r.key]}</span>
              <span className="truncate text-muted">
                {valueOf(r)}
                {r.on && r.source === 'machine' && ' · machine default'}
              </span>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}

export function WorkspacePage({ ws }: { ws: string }) {
  const list = useRead('workspaces.list', {}).data;
  const name = list?.workspaces.find((w) => w.key === ws)?.name;
  const [tab, setTab] = useTabState<'overview' | 'settings'>('view', 'overview');
  if (!list) return null;
  if (!name) return <p className="p-6 text-sm text-muted">This workspace is gone.</p>;
  return (
    <div className="h-full w-full overflow-auto">
      <div className="mx-auto flex w-full max-w-6xl flex-col gap-4 px-6 pb-8">
        <div role="tablist" className="sticky top-0 z-20 flex items-center justify-end gap-4 border-b border-border bg-bg pt-3 text-sm">
          {(['overview', 'settings'] as const).map((k) => (
            <button
              key={k}
              type="button"
              role="tab"
              aria-selected={tab === k}
              onClick={() => setTab(k)}
              className={`-mb-px border-b-2 py-2 capitalize ${tab === k ? 'border-accent font-medium text-fg' : 'border-transparent text-muted hover:text-fg'}`}
            >
              {k}
            </button>
          ))}
        </div>
        {tab === 'overview' ? <Overview ws={ws} /> : <Settings ws={ws} />}
      </div>
    </div>
  );
}
