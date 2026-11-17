import { roomRows, switchWords, waitLine, VERBS, type RoomRow } from '@handline/app-core';
import { useAccounts, useAgents, useVerbRun, useWrite } from '@handline/app-data';
import type { TranscriptItem } from '@handline/client';
import { clock } from '#src/format/time.ts';
import { ArrowRight, ArrowRightLeft, Check, ChevronDown, Clock, TriangleAlert } from 'lucide-react';
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

/** While the agent waits at its limit: a card with the wait and the account with the most room to switch to now, the others in a list by
 * the room left in their tighter window; with no other account, the wait alone in one line. */
export function SwitchWait({ agent }: { agent: string }) {
  const a = useSwitch(agent);
  const accounts = useAccounts().data?.accounts ?? [];
  const switchNow = useWrite('agent.switch_now', agent);
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
      {!best && (
        <div role="status" className="flex items-center gap-1.5 text-xs text-muted">
          <Clock size={14} className="shrink-0" />
          {waitLine(s.from, s.until)}
        </div>
      )}
      {/* The wait is not the end: whenever another account is there, the card offers it, with the wait as its line. */}
      {best && (
        <div role="status" className="flex flex-col gap-2 rounded-lg border border-border p-3">
          <div>
            <div className="flex items-center gap-1.5 font-medium">
              <Clock size={14} className="shrink-0" />
              {offer?.title ?? waitLine(s.from, s.until)}
            </div>
            {offer?.text && <div className="text-muted">{offer.text}</div>}
          </div>
          <SwitchPick rows={rows} pending={switchNow.isPending} go={go} />
        </div>
      )}
    </div>
  );
}

/** The account with the most room as a button, the others in its menu by the room left in their tighter window. */
export function SwitchPick({ rows, pending, go }: { rows: RoomRow[]; pending: boolean; go: (account: string) => void }) {
  const split = useRef<HTMLSpanElement>(null);
  const best = rows.find((r) => !r.current);
  if (!best) return null;
  return (
    <div className="flex items-center gap-2">
      <span ref={split} className="inline-flex">
        <Button variant="primary" className="rounded-r-none" disabled={pending} onClick={() => go(best.name)}>
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
              disabled={pending}
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
  );
}

/** A person's move waiting to run, in the turn's row at the chat's foot: to which account and when, with Move now and Cancel while it
 * waits for the turn to end; once Move now stopped the turn, that it waits for the handoff note. */
export function PendingMove({ agent }: { agent: string }) {
  const m = useSwitch(agent)?.pending_move;
  const now = useWrite('agent.move_now', agent);
  const cancel = useWrite('agent.move_cancel', agent);
  if (!m) return null;
  const failed = (what: string) => (e: unknown) => toast(`${what}: ${e instanceof Error ? e.message : String(e)}`, { tone: 'warning' });
  const stopping = m.phase === 'stopping';
  const busy = now.isPending || cancel.isPending;
  const link = 'shrink-0 rounded px-1 hover:bg-hover disabled:opacity-50';
  return (
    <div role="status" className="flex h-5 min-w-0 items-center gap-2 text-xs text-muted">
      <ArrowRightLeft size={12} className="shrink-0" />
      <span className="min-w-0 truncate">
        {stopping ? `Moving to ${m.to}: waiting for the handoff note…` : `Moves to ${m.to} when this turn ends, after its handoff note`}
      </span>
      {!stopping && (
        <>
          <button
            type="button"
            className={`${link} font-medium text-accent`}
            disabled={busy}
            onClick={() => now.mutate({ agent, account: m.to, request_id: uuid() }, { onError: failed('Did not move now') })}
          >
            Move now
          </button>
          <button
            type="button"
            className={`${link} text-fg`}
            disabled={busy}
            onClick={() => cancel.mutate({ agent, request_id: uuid() }, { onError: failed('Not cancelled') })}
          >
            Cancel
          </button>
        </>
      )}
    </div>
  );
}

/** A move in the chat: the account it landed on (and that it began without a handoff note when so), or a kept move that could not run. */
export function MoveLine({
  agent,
  at,
  moved,
  failed,
}: {
  agent: string;
  at: string;
  moved?: TranscriptItem['account_moved'];
  failed?: TranscriptItem['move_failed'];
}) {
  // A move that did not run is something to see to: a card, as the chat's other holds are; a move that ran is a line between rules.
  if (!moved) return <MoveFailed agent={agent} at={at} failed={failed} />;
  return (
    <div className="my-3 flex items-center gap-4 text-sm text-muted">
      <div className="h-px flex-1 bg-border" />
      <span title={clock(at)} className="flex items-center gap-1.5">
        <ArrowRightLeft size={14} />
        {`Moved to ${moved.to}${moved.note_written ? '' : ' · no handoff note'}`}
      </span>
      <div className="h-px flex-1 bg-border" />
    </div>
  );
}

/** A kept move that could not run, as a card that does not end there: the account with the most room to move to now, the others in its menu. */
function MoveFailed({ agent, at, failed }: { agent: string; at: string; failed?: TranscriptItem['move_failed'] }) {
  const a = useSwitch(agent);
  const accounts = useAccounts().data?.accounts ?? [];
  const run = useVerbRun(agent);
  const rows = roomRows(accounts, a?.account ?? '');
  const go = (account: string) =>
    run(VERBS.move, account, {
      onError: (e) => toast(`The move did not start: ${e instanceof Error ? e.message : String(e)}`, { tone: 'warning' }),
    });
  return (
    <div role="status" title={clock(at)} className="my-3 flex items-start gap-2 rounded-lg bg-warning/10 px-3 py-2 text-sm">
      <TriangleAlert size={16} className="mt-0.5 shrink-0 text-warning" />
      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <div>
          <div className="font-medium">{`Did not move to ${failed?.to}`}</div>
          {failed?.reason && <div className="text-muted">{failed.reason}</div>}
        </div>
        <SwitchPick rows={rows} pending={false} go={go} />
      </div>
    </div>
  );
}
