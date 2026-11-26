import { unusedLine } from '@handline/app-core';
import { useAgents, useNameOf, useOutlook } from '@handline/app-data';
import { activityOf } from '#src/layout/agentList.ts';
import type { Account } from '@handline/client';
import type { ReactNode } from 'react';
import { Pencil } from 'lucide-react';
import { chatOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { AccountEmail } from '#src/usage/AccountEmail.tsx';
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
  const mine = useOutlook('24h').data?.by_account?.find((b) => b.account === a.name);
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
        {note}
      </div>
      {bars && !out && (
        <div className="flex min-w-0 flex-1 flex-wrap gap-x-4 gap-y-2">
          <div className="min-w-24 flex-1">
            <UsageBar stacked name="5-hour" span={FIVE_HOURS} w={a.five_hour} age={a.age_s} none={noFive} />
          </div>
          <div className="min-w-24 flex-1">
            <UsageBar stacked name="Week" span={WEEK} w={a.weekly} age={a.age_s} foot={unused} />
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

/** The agents on an account as initial circles, filled while one works (its sidebar mark), and how many work over them. */
function AgentsBar({ account: a }: { account: Account }) {
  const nameOf = useNameOf();
  const views = new Map((useAgents().data?.agents ?? []).map((x) => [x.key, x]));
  const working = (k: string) => {
    const v = views.get(k);
    return !!v && activityOf(v) === 'working';
  };
  const busy = a.agents.filter(working).length;
  // Working ones first; past SHOWN the rest is a count, so the stack stays in its column.
  const ordered = [...a.agents.filter(working), ...a.agents.filter((k) => !working(k))];
  const shown = ordered.length > SHOWN ? ordered.slice(0, SHOWN - 1) : ordered;
  const more = ordered.length - shown.length;
  return (
    <div className="flex flex-col gap-1">
      <div className="flex justify-between gap-6 text-xs">
        <span className="text-muted">Agents</span>
        {a.agents.length > 0 && <span className="text-fg tabular-nums">{a.paused ? 'moves next turn' : `${busy} working`}</span>}
      </div>
      {a.agents.length ? (
        // On hover, every agent on it by name, each opening its chat.
        <div className="group/agents relative flex">
          {shown.map((k) => (
            <span
              key={k}
              className={`-mr-1 grid size-6 place-items-center rounded-full border-2 border-bg text-xs font-medium uppercase ${
                working(k) ? 'bg-accent text-accent-fg' : 'bg-hover text-muted'
              }`}
            >
              {nameOf(k).slice(0, 1)}
            </span>
          ))}
          {more > 0 && (
            <span className="grid h-6 place-items-center rounded-full border-2 border-bg bg-hover px-1.5 text-xs text-muted tabular-nums">
              +{more}
            </span>
          )}
          <ul className="absolute top-full right-0 z-30 hidden max-h-64 min-w-44 flex-col overflow-y-auto rounded-md border border-border bg-bg py-1 group-hover/agents:flex">
            {ordered.map((k) => (
              <AgentLink key={k} agent={k} working={working(k)} />
            ))}
          </ul>
        </div>
      ) : (
        <span className="text-xs text-muted">{a.paused ? 'paused' : 'No agents'}</span>
      )}
    </div>
  );
}

/** One agent in an account's list: its name and whether it works now; a click opens its chat the app's way. */
function AgentLink({ agent, working }: { agent: string; working: boolean }) {
  const open = useOpenable(chatOf(agent));
  return (
    <li>
      <button type="button" {...open} className="flex w-full items-center gap-2 px-3 py-1 text-left text-xs text-fg hover:bg-hover">
        <span className="min-w-0 flex-1 truncate">{useNameOf()(agent)}</span>
        {working && <span className="text-muted">working</span>}
      </button>
    </li>
  );
}
