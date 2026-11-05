import { useAccounts, useServes, useWrite } from '@handline/app-data';
import { ArrowLeftRight, ChevronDown, ChevronRight, Gauge, TriangleAlert, UserRound } from 'lucide-react';
import { useState } from 'react';
import { CardAction, chip, MenuFact, MenuHeading, MenuRule } from '#src/chat/agent/menuParts.tsx';
import { focusIn } from '#src/layout/focus.ts';
import { howOf, useOpenTab } from '#src/layout/openTab.tsx';
import { HoverCard } from '#src/ui/HoverCard.tsx';
import { FIVE_HOURS, HIGH, UsageBar, WEEK } from '#src/usage/UsageBar.tsx';

/** The account the agent runs on: its name, with a mark when its 5-hour window runs high, the login is gone, or the daemon is
 * about to move the agent off it. Resting on it shows the account's windows, who shares it, and this agent's last moves, with a
 * move to another account and a way to the Usage tab. A click keeps that card open. */
export function AccountItem({ agent, account, fire }: { agent: string; account: string; fire: string | null }) {
  const accounts = useAccounts().data?.accounts ?? [];
  const openTab = useOpenTab();
  const mine = accounts.find((a) => a.name === account);
  const usage = (how: 'here' | 'side') => {
    focusIn('usage', '', account);
    openTab('usage', how);
  };
  const high = (mine?.five_hour?.used_pct ?? 0) >= HIGH;
  const login = mine?.login_state === 'needs_login';
  const moving = fire === 'usage' || fire === 'usage_7d';
  const moves = (mine?.moves ?? [])
    .filter((m) => m.agent === agent)
    .slice(-2)
    .reverse();
  return (
    <HoverCard
      pin
      trigger={
        <button type="button" className={chip}>
          <UserRound size={13} className="shrink-0" />
          <span className="truncate">{mine?.label ?? account}</span>
          {login ? (
            <TriangleAlert size={12} className="shrink-0 text-danger" aria-label="needs login" />
          ) : moving ? (
            <ArrowLeftRight size={12} className="shrink-0 text-warning" aria-label="moves to another account soon" />
          ) : (
            high && <span className="size-1.5 shrink-0 rounded-full bg-warning" aria-label="5-hour window running high" />
          )}
        </button>
      }
    >
      <div className="flex min-w-64 flex-col gap-2 px-2 pt-1 pb-2">
        {login && <p className="text-xs text-danger">Needs login</p>}
        {moving && <p className="text-xs text-warning">Near its limit: moves to another account soon</p>}
        <UsageBar name="5-hour" span={FIVE_HOURS} w={mine?.five_hour ?? null} />
        <UsageBar name="Week" span={WEEK} w={mine?.weekly ?? null} />
      </div>
      {mine && <MenuFact name="Agents on it">{`${mine.agents.length} (up to ${mine.cap_effective} at once)`}</MenuFact>}
      {moves.length > 0 && (
        <>
          <MenuRule />
          <MenuHeading>Last moves</MenuHeading>
          {moves.map((m) => (
            <MenuFact
              key={m.at}
              name={new Date(m.at).toLocaleString([], { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' })}
            >
              {`${m.from ?? '—'} → ${m.to}, ${m.reason}`}
            </MenuFact>
          ))}
        </>
      )}
      <MenuRule />
      <MoveTo agent={agent} account={account} />
      <CardAction icon={<Gauge size={14} />} onClick={(e) => usage(howOf(e))}>
        Open usage
      </CardAction>
    </HoverCard>
  );
}

/** "Move to another account", unfolding the other accounts in place with the room each has left. */
function MoveTo({ agent, account }: { agent: string; account: string }) {
  const [open, setOpen] = useState(false);
  const accounts = useAccounts().data?.accounts ?? [];
  const move = useWrite('agent.move', agent);
  const served = useServes('agent.move').served;
  const others = accounts.filter((a) => a.name !== account && a.login_state !== 'needs_login');
  if (!served || !others.length) return null;
  return (
    <>
      <CardAction icon={open ? <ChevronDown size={14} /> : <ChevronRight size={14} />} onClick={() => setOpen(!open)}>
        Move to another account
      </CardAction>
      {open &&
        others.map((a) => (
          <button
            key={a.name}
            type="button"
            onClick={() => move.mutate({ agent, account: a.name })}
            className="flex h-7 w-full items-center justify-between gap-6 rounded-sm pr-2 pl-8 text-sm text-fg outline-none hover:bg-hover focus-visible:bg-hover"
          >
            {a.label}
            <span className="text-xs text-muted tabular-nums">{a.five_hour ? `${100 - Math.round(a.five_hour.used_pct)}% left` : ''}</span>
          </button>
        ))}
    </>
  );
}
