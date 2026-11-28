// The one button that gets a stopped, held or sleeping agent going: the chat's hold card and a person's card in the town show the same.
import { roomRows, setGoingOf, type SetGoing } from '@handline/app-core';
import { useAccounts, useAgents, useUnstall, useWrite, why } from '@handline/app-data';
import { AlarmClock, ArrowRightLeft, RotateCw } from 'lucide-react';
import type { ComponentProps } from 'react';
import { activityOf } from '#src/layout/agentList.ts';
import { openSettings } from '#src/settings/open.ts';
import { useAccountLabel } from '#src/usage/AccountName.tsx';
import { Button, useAction } from '#src/ui/Button.tsx';
import { toast } from '#src/ui/toast.tsx';
import { uuid } from '#src/ui/uuid.ts';
import { useOpenTerminal } from '#src/chat/agent/openTerminal.ts';

/** `room`: the whole percent left on that account in its tighter window, null when not known. */
export type Going = { go: SetGoing; account: string | null; room: number | null };

/** What gets `agent` going, with the account a switch would take (the switch card's first pick); null when nothing fits. */
export function useSetGoing(agent: string): Going | null {
  const a = useAgents().data?.agents.find((x) => x.key === agent);
  const accounts = useAccounts().data?.accounts ?? [];
  if (!a) return null;
  const act = activityOf(a);
  const login = act === 'error' && accounts.find((x) => x.name === a.account)?.login_state === 'needs_login';
  const s = a.account_switch;
  const offer = a.needs_you?.kind === 'switch_offer' ? a.needs_you : null;
  const spare = s ? roomRows(accounts, s.from, offer?.account).find((r) => !r.current) : undefined;
  const go = setGoingOf(a, act, login, !!spare);
  return go ? { go, account: spare?.name ?? null, room: spare?.room ?? null } : null;
}

/** `terminal`: what opens the agent's terminal where this button is. A refusal is a toast: the card may be gone by the answer. */
export function SetGoingButton({
  agent,
  going: { go, account, room },
  terminal,
}: {
  agent: string;
  going: Going;
  terminal: ComponentProps<typeof Button>;
}) {
  const label = useAccountLabel();
  const a = useAgents().data?.agents.find((x) => x.key === agent);
  const unstall = useUnstall(agent);
  // A closed agent's session resumes; one with none is started anew.
  const start = useWrite('session.start', agent);
  const switchNow = useWrite('agent.switch_now', agent);
  const run = useAction();
  const moveIn = useOpenTerminal(agent);
  const call = (f: () => Promise<unknown>) => void run.go(() => f().catch((e: unknown) => toast(why(e), { tone: 'warning' })));
  if (go === 'retry')
    return (
      <Button busy={run.busy} onClick={() => call(unstall)}>
        <RotateCw size={14} />
        Try again
      </Button>
    );
  if (go === 'wake')
    return (
      <Button busy={run.busy} onClick={() => call(() => start.mutateAsync({ agent }))}>
        <AlarmClock size={14} />
        Wake
      </Button>
    );
  if (go === 'login')
    return <Button onClick={() => openSettings({ section: 'accounts', account: a?.account ?? undefined })}>Log in</Button>;
  if (go === 'terminal') {
    // A Chat agent's conversation moves into its terminal first; the terminal opens here either way.
    const onClick: typeof terminal.onClick = (e) => {
      if (moveIn.chat) moveIn.open();
      terminal.onClick?.(e);
    };
    return (
      <Button {...terminal} onClick={onClick}>
        Open terminal
      </Button>
    );
  }
  return (
    <Button busy={switchNow.isPending} onClick={() => account && call(() => switchNow.mutateAsync({ agent, account, request_id: uuid() }))}>
      <ArrowRightLeft size={14} />
      Switch to {account && label(account)}
      {room !== null && ` · ${room}% left`}
    </Button>
  );
}
