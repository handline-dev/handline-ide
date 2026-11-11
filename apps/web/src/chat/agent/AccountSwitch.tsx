import { roomRows, switchWords, waitLine } from '@handline/app-core';
import { useAccounts, useAgents, useWrite } from '@handline/app-data';
import { ArrowRight, Check, ChevronDown, Clock } from 'lucide-react';
import { useRef } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { Menu, MenuItem } from '#src/ui/Menu.tsx';
import { toast } from '#src/ui/toast.tsx';
import { uuid } from '#src/ui/uuid.ts';

const useSwitch = (agent: string) => useAgents().data?.agents.find((a) => a.key === agent);

/** The message box's words while the agent saves, switches or starts on a new account; null when it takes messages. */
export function useSwitchLock(agent: string): string | null {
  const s = useSwitch(agent)?.account_switch;
  return s ? switchWords(s) : null;
}

/** While the agent waits at its limit: the wait in one line, and when no account has room to spare, the account with the most
 * room to switch to now, with the others in a list by the room left in their tighter window. */
export function SwitchWait({ agent }: { agent: string }) {
  const a = useSwitch(agent);
  const accounts = useAccounts().data?.accounts ?? [];
  const switchNow = useWrite('agent.switch_now', agent);
  const split = useRef<HTMLSpanElement>(null);
  const s = a?.account_switch;
  if (s?.phase !== 'waiting') return null;
  const offer = a?.needs_you?.kind === 'switch_offer' ? a.needs_you : null;
  const rows = roomRows(accounts, s.from, offer?.account);
  const go = (account: string) =>
    switchNow.mutate(
      { agent, account, request_id: uuid() },
      { onError: (e) => toast(`The switch did not start: ${e instanceof Error ? e.message : String(e)}`, { tone: 'warning' }) },
    );
  const best = rows.find((r) => !r.current);
  return (
    <div className="mb-2 flex min-w-0 flex-col gap-2 text-sm">
      <div role="status" className="flex items-center gap-1.5 text-xs text-muted">
        <Clock size={14} className="shrink-0" />
        {waitLine(s.from, s.until)}
      </div>
      {offer && best && (
        <div className="flex flex-col gap-2 rounded-lg border border-border p-3">
          <div>
            <div className="font-medium">{offer.title}</div>
            {offer.text && <div className="text-muted">{offer.text}</div>}
          </div>
          <div className="flex items-center gap-2">
            <span ref={split} className="inline-flex">
              <Button variant="primary" className="rounded-r-none" disabled={switchNow.isPending} onClick={() => go(best.name)}>
                Switch to {best.name}
                {best.room !== null && ` · ${best.room}% left`}
              </Button>
              <Menu
                side="top"
                align="start"
                anchor={split}
                trigger={
                  <Button
                    variant="primary"
                    className="rounded-l-none border-l border-accent-fg/30 px-1.5"
                    disabled={switchNow.isPending}
                    aria-label="Other accounts"
                  >
                    <ChevronDown size={14} />
                  </Button>
                }
              >
                <div className="px-2 pt-1 pb-0.5 text-xs text-muted">Switch to…</div>
                {rows.map((r) => (
                  <MenuItem
                    key={r.name}
                    disabled={r.current}
                    icon={r.current ? <Check size={14} /> : <ArrowRight size={14} />}
                    onClick={() => go(r.name)}
                  >
                    <span className="flex items-center gap-3">
                      <span className={`w-24 truncate ${r.name === best.name ? 'font-semibold' : ''}`}>
                        {r.name}
                        {r.current && ' · now'}
                      </span>
                      <span className="text-xs whitespace-nowrap text-muted">
                        {r.room === null ? 'no reading yet' : `${r.room}% left${r.resets ? ` · ${r.resets}` : ''}`}
                      </span>
                    </span>
                  </MenuItem>
                ))}
              </Menu>
            </span>
          </div>
        </div>
      )}
    </div>
  );
}
