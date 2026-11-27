import { Mark } from '#src/ui/Face.tsx';
import { creditsLine, foldSeats, unusedLine, type FoldedAgent } from '@handline/app-core';
import { useNameOf, useOutlook, useRead } from '@handline/app-data';
import { activityOf } from '#src/layout/agentList.ts';
import type { Account } from '@handline/client';
import type { ReactNode } from 'react';
import { Pencil } from 'lucide-react';
import { chatOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { AccountEmail } from '#src/usage/AccountEmail.tsx';
import { MachinesOf } from '#src/connection/PlaceWord.tsx';
import { StopTime } from '#src/usage/StopTime.tsx';
import { FIVE_HOURS, UsageBar, WEEK } from '#src/usage/UsageBar.tsx';

/** The day a subscription ends, as "Nov 30"; null when it does not end. */
function endsOn(a: Account): string | null {
  if (!a.cycle.on || !a.cycle.end) return null;
  return new Date(`${a.cycle.end}T00:00`).toLocaleDateString('en', { month: 'short', day: 'numeric' });
}

/** The row both Usage and Settings draw for an account; `bars` off leaves its name alone, for a row whose Log in button names the email. */
export function AccountFacts({
  account: a,
  plan,
  bars = true,
  note,
  onRename,
}: {
  account: Account;
  plan?: string | null;
  bars?: boolean;
  note?: ReactNode;
  /** A pencil right after the name, shown on hover in a slot kept for it. */
  onRename?: () => void;
}) {
  const out = a.login_state === 'needs_login';
  const ends = endsOn(a);
  const outlook = useOutlook('24h').data;
  const mine = outlook?.by_account?.find((b) => b.account === a.name);
  const credits = creditsLine(outlook?.budget?.accounts.find((b) => b.account === a.name)?.extra);
  const noFive = mine?.five_hour === false;
  const unused = unusedLine(mine?.forecast);
  return (
    <div
      className={`group/account flex min-w-0 flex-1 flex-col gap-3 @md:flex-row @md:items-center @md:gap-6 ${a.paused ? 'opacity-60' : ''}`}
    >
      <div className="flex min-w-0 flex-col @md:w-48 @md:shrink-0">
        <span className="flex items-center gap-1.5 text-sm font-medium text-fg">
          <span className="truncate">{a.label}</span>
          {onRename && (
            <IconButton
              label={`Rename ${a.label}`}
              size="sm"
              className="opacity-0 group-hover/account:opacity-100 focus-visible:opacity-100"
              onClick={(e) => (e.stopPropagation(), onRename())}
            >
              <Pencil size={12} />
            </IconButton>
          )}
          {plan && <span className="shrink-0 rounded border border-border px-1 text-xs font-normal text-muted capitalize">{plan}</span>}
          {out && <span className="shrink-0 rounded-full bg-warning/15 px-2 text-xs font-normal text-warning">Logged out</span>}
          {a.paused && <span className="shrink-0 rounded-full bg-fg px-2 text-xs font-normal text-bg">Paused</span>}
        </span>
        {bars && (
          <span className="flex min-w-0 gap-1 text-xs text-muted">
            <AccountEmail account={a} />
            {ends && <span className="shrink-0">{`${a.email ? '· ' : ''}ends ${ends}`}</span>}
          </span>
        )}
        {bars && <MachinesOf ids={a.machines ?? []} lead="Logged in on" />}
        {note}
      </div>
      {bars && !out && (
        <div className="flex min-w-0 flex-1 flex-wrap gap-x-4 gap-y-2">
          <div className="min-w-24 flex-1">
            <UsageBar stacked name="5-hour" span={FIVE_HOURS} w={a.five_hour} age={a.age_s} none={noFive} />
            {!noFive && <StopTime account={a} />}
          </div>
          <div className="min-w-24 flex-1">
            <UsageBar stacked name="Week" span={WEEK} w={a.weekly} age={a.age_s} foot={unused} />
            {credits && (
              <span className={`block text-xs tabular-nums ${credits.spent ? 'text-warning' : 'text-muted'}`}>{credits.text}</span>
            )}
          </div>
          <div className="min-w-24 flex-1">
            <AgentsBar account={a} />
          </div>
        </div>
      )}
    </div>
  );
}

/** At most this many circles in the stack, a +N one included. */
const SHOWN = 6;

/** The agents on an account as Town faces, one per agent (its temps folded in, their count on the circle), ringed while one works,
 * and how many sessions work over them. */
function AgentsBar({ account: a }: { account: Account }) {
  const all = useRead('agents.list', {}).data?.agents ?? []; // temps too: they run on the account under their agent's name
  const views = new Map(all.map((x) => [x.key, x]));
  const busy = (k: string) => {
    const v = views.get(k);
    return !!v && activityOf(v) === 'working';
  };
  const folded = foldSeats({ agents: all }, a.agents, busy);
  const working = folded.reduce((n, f) => n + f.working, 0);
  // Working ones first; past SHOWN the rest is a count, so the stack stays in its column.
  const ordered = [...folded.filter((f) => f.working), ...folded.filter((f) => !f.working)];
  const shown = ordered.length > SHOWN ? ordered.slice(0, SHOWN - 1) : ordered;
  const more = ordered.length - shown.length;
  return (
    <div className="flex flex-col gap-1">
      <div className="flex justify-between gap-6 text-xs">
        <span className="text-muted">Agents</span>
        {a.agents.length > 0 && <span className="text-fg tabular-nums">{a.paused ? 'moves next turn' : `${working} working`}</span>}
      </div>
      {a.agents.length ? (
        // On hover, every agent on it by name, each opening its chat.
        <div className="group/agents relative flex">
          {shown.map((f) => (
            <span key={f.key} className={`relative -mr-1 rounded-full ring-2 ${f.working ? 'ring-accent' : 'ring-bg'}`}>
              <Mark who={f.key} />
              {f.sessions > 1 && (
                <span className="absolute -top-2 -right-1.5 z-10 grid h-4 min-w-4 place-items-center rounded-full border border-bg bg-fg px-0.5 text-xs leading-none font-normal text-bg tabular-nums">
                  {f.sessions}
                </span>
              )}
            </span>
          ))}
          {more > 0 && (
            <span className="grid h-6 place-items-center rounded-full border-2 border-bg bg-hover px-1.5 text-xs text-muted tabular-nums">
              +{more}
            </span>
          )}
          <ul className="absolute top-full right-0 z-30 hidden max-h-64 min-w-44 flex-col overflow-y-auto rounded-md border border-border bg-bg py-1 group-hover/agents:flex">
            {ordered.map((f) => (
              <AgentLink key={f.key} agent={f} />
            ))}
          </ul>
        </div>
      ) : (
        <span className="text-xs text-muted">{a.paused ? 'paused' : 'No agents'}</span>
      )}
    </div>
  );
}

/** What an agent's row says: "working", "3 working" when several of its sessions do, "4 sessions" when several run and none works. */
const doing = (f: FoldedAgent) =>
  f.working > 1 ? `${f.working} working` : f.working ? 'working' : f.sessions > 1 ? `${f.sessions} sessions` : '';

/** One agent in an account's list: its name and whether it works now; a click opens its chat the app's way. */
function AgentLink({ agent }: { agent: FoldedAgent }) {
  const open = useOpenable(chatOf(agent.key));
  return (
    <li>
      <button type="button" {...open} className="flex w-full items-center gap-2 px-3 py-1 text-left text-xs text-fg hover:bg-hover">
        <Mark who={agent.key} size={16} />
        <span className="min-w-0 flex-1 truncate">{useNameOf()(agent.key)}</span>
        {doing(agent) && <span className="text-muted tabular-nums">{doing(agent)}</span>}
      </button>
    </li>
  );
}
