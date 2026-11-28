import { Spinner } from '#src/ui/Loading.tsx';
import { toneOf } from '#src/ui/Meter.tsx';
import { since } from '#src/format/time.ts';
import { useAccounts, useServes, useWrite, why } from '@handline/app-data';
import { ArrowLeftRight, Check, ChevronDown, ChevronRight, Gauge, Pin, TriangleAlert, UserRound } from 'lucide-react';
import { useAccountPin } from '#src/chat/agent/accountPin.ts';
import { useState } from 'react';
import { CardAction, chip, MenuFact, MenuHeading, MenuRule } from '#src/chat/agent/menuParts.tsx';
import { focusIn } from '#src/layout/focus.ts';
import { useOpenable } from '#src/layout/openable.tsx';
import { HoverCard } from '#src/ui/HoverCard.tsx';
import { useAction } from '#src/ui/Button.tsx';
import { FIVE_HOURS, UsageBar, WEEK } from '#src/usage/UsageBar.tsx';
import { Refs } from '#src/ui/refs.tsx';
import { AccountName } from '#src/usage/AccountName.tsx';
import { openSettings } from '#src/settings/open.ts';

/** The account the agent runs on: its name, with a mark when its 5-hour window runs high, the login is gone, or the daemon is
 * about to move the agent off it. Resting on it shows the account's windows, who shares it, and this agent's last moves, with a
 * move to another account and a way to the Usage tab. A click keeps that card open. */
export function AccountItem({ agent, account, fire }: { agent: string; account: string; fire: string | null }) {
  const accounts = useAccounts().data?.accounts ?? [];
  const mine = accounts.find((a) => a.name === account);
  const usage = useOpenable('usage', { focus: () => focusIn('usage', '', account) });
  const high = toneOf(mine?.five_hour?.used_pct ?? 0) !== 'ok';
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
        <UsageBar name="5-hour" span={FIVE_HOURS} w={mine?.five_hour ?? null} age={mine?.age_s} />
        <UsageBar name="Week" span={WEEK} w={mine?.weekly ?? null} age={mine?.age_s} />
      </div>
      {mine?.email && <MenuFact name="Email">{mine.email}</MenuFact>}
      {mine && <MenuFact name="Agents on it">{mine.agents.length}</MenuFact>}
      {moves.length > 0 && (
        <>
          <MenuRule />
          <MenuHeading>Last moves</MenuHeading>
          {moves.map((m) => (
            <MenuFact key={m.at} name={since(m.at)}>
              {m.from ? <AccountName name={m.from} /> : '—'} → <AccountName name={m.to} />, <Refs text={m.reason} />
            </MenuFact>
          ))}
        </>
      )}
      <MenuRule />
      <PinItem agent={agent} />
      <MoveTo agent={agent} account={account} />
      <CardAction icon={<Gauge size={14} />} {...usage}>
        Open usage
      </CardAction>
      <CardAction icon={<UserRound size={14} />} onClick={() => openSettings({ section: 'accounts', account })}>
        Account details
      </CardAction>
    </HoverCard>
  );
}

/** "Move to another account", unfolding the other accounts in place with the room each has left. */
function MoveTo({ agent, account }: { agent: string; account: string }) {
  const [open, setOpen] = useState(false);
  const accounts = useAccounts().data?.accounts ?? [];
  const move = useWrite('agent.move', agent);
  const moving = useAction();
  const [to, setTo] = useState<string | null>(null);
  const served = useServes('agent.move').served;
  const others = accounts.filter((a) => a.name !== account && a.login_state !== 'needs_login');
  if (!served || !others.length) return null;
  return (
    <>
      <CardAction
        icon={open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        onClick={() => {
          setOpen(!open);
          moving.reset();
        }}
      >
        Move to another account
      </CardAction>
      {open &&
        others.map((a) => (
          <button
            key={a.name}
            type="button"
            disabled={moving.busy}
            aria-busy={moving.busy && to === a.name}
            onClick={() => {
              setTo(a.name);
              void moving.go(() => move.mutateAsync({ agent, account: a.name }));
            }}
            className="flex h-7 w-full items-center justify-between gap-6 rounded-sm pr-2 pl-8 text-sm text-fg outline-none hover:bg-hover focus-visible:bg-hover"
          >
            {a.label}
            <span className="grid text-xs text-muted tabular-nums">
              <span className={`col-start-1 row-start-1 ${moving.busy && to === a.name ? 'invisible' : ''}`}>
                {a.five_hour ? `${100 - Math.round(a.five_hour.used_pct)}% left` : ''}
              </span>
              {moving.busy && to === a.name && <Spinner size={12} className="col-start-1 row-start-1 justify-self-end" />}
            </span>
          </button>
        ))}
      {open && !!moving.error && <p className="pl-8 text-xs text-danger">{why(moving.error)}</p>}
    </>
  );
}

/** "Pin to this account": the agent stays on this account at a limit and waits for its reset, instead of moving. */
function PinItem({ agent }: { agent: string }) {
  const pin = useAccountPin(agent);
  if (!pin.ready) return null;
  return (
    <CardAction icon={<Pin size={14} />} aria-pressed={pin.pinned} disabled={pin.saving} onClick={() => pin.set(!pin.pinned)}>
      <span className="flex-1 text-left">Pin to this account</span>
      {pin.pinned && <Check size={14} className="text-accent" aria-hidden />}
    </CardAction>
  );
}
