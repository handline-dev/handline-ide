import { Spinner } from '#src/ui/Loading.tsx';
import { Refs } from '#src/ui/refs.tsx';
import { clockTime, moveSteps, roomRows, switchWords, waitLine, VERBS, type RoomRow } from '@handline/app-core';
import { useAccounts, useAgents, useVerbRun, useWrite } from '@handline/app-data';
import type { TranscriptItem } from '@handline/client';
import { ArrowRight, ArrowRightLeft, Check, ChevronDown, Circle, Clock, TriangleAlert } from 'lucide-react';
import { clock, useNow } from '#src/format/time.ts';
import { motion } from 'framer-motion';
import { useRef, useState } from 'react';
import { Facts, HandoffLines } from '#src/chat/CompactionLine.tsx';
import { spring } from '#src/motion.ts';
import { Ago } from '#src/ui/Ago.tsx';
import { ModalFrame, openModal } from '#src/ui/modal.tsx';
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

/** A move under way as its four steps, the current one with its seconds; narrow, the title and the current step alone. Null for a wait. */
export function MoveSteps({ agent }: { agent: string }) {
  const s = useSwitch(agent)?.account_switch;
  const tick = useNow(1000);
  const m = s ? moveSteps(s) : null;
  if (!s || !m) return null;
  const secs = Math.max(0, Math.floor((tick - Date.parse(s.since)) / 1000));
  const took = `${Math.floor(secs / 60)}:${String(secs % 60).padStart(2, '0')}`;
  const now = m.steps[m.at];
  return (
    <span role="status" className="@container flex min-w-0 flex-1 items-center gap-2 text-xs text-muted">
      <span className="shrink-0 font-medium text-fg">{m.title}</span>
      <span className="hidden min-w-0 items-center gap-1.5 truncate @xl:flex">
        {m.steps.map((st, i) => (
          <span key={st.name} className={`flex shrink-0 items-center gap-1 ${st.state === 'now' ? 'text-fg' : ''}`}>
            {i > 0 && <span aria-hidden>—</span>}
            {st.state === 'done' ? (
              <Check size={12} className="text-success" aria-label="done" />
            ) : st.state === 'now' ? (
              <Spinner size={12} label="now" />
            ) : (
              <Circle size={12} aria-label="next" />
            )}
            {st.name}
            {st.state === 'now' && <span className="tabular-nums">{took}</span>}
          </span>
        ))}
      </span>
      <span className="min-w-0 truncate @xl:hidden">
        · {m.at + 1}/4 {now.name} · <span className="tabular-nums">{took}</span>
      </span>
    </span>
  );
}

/** While the agent waits at its limit: a card with the wait and the account with the most room to switch to now, the others in a list by
 * the room left in their tighter window; with no other account, the wait alone in one line. */
export function SwitchWait({ agent }: { agent: string }) {
  const a = useSwitch(agent);
  const accounts = useAccounts().data?.accounts ?? [];
  const switchNow = useWrite('agent.switch_now', agent);
  const now = useNow();
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
  const wait = waitLine(s.from, s.until, now);
  const until = s.until ? `${s.from} resets at ${clockTime(s.until)}` : undefined;
  return (
    <div className="mb-2 flex min-w-0 flex-col gap-2 text-sm">
      {!best && (
        <div role="status" className="flex items-center gap-1.5 text-xs text-muted" title={until}>
          <Clock size={14} className="shrink-0" />
          {wait}
        </div>
      )}
      {/* The wait is not the end: whenever another account is there, the card offers it, with the wait as its line. */}
      {best && (
        <div role="status" className="flex flex-col gap-2 rounded-lg border border-border p-3">
          <div>
            <div className="flex items-center gap-1.5 font-medium" title={offer?.title ? undefined : until}>
              <Clock size={14} className="shrink-0" />
              {offer?.title ?? wait}
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

/** The handoff note of the session a move started: the first such session before the next move. */
export function moveNoteAfter(rows: readonly ({ item: TranscriptItem } | object)[], i: number): string | null | undefined {
  for (let j = i + 1; j < rows.length; j++) {
    const r = rows[j];
    if (!('item' in r)) continue;
    if (r.item.account_moved) return undefined;
    if (r.item.session?.cause === 'move') return r.item.session.handoff_note ?? null;
  }
  return undefined;
}

/** A move in the chat: the account it landed on (and that it began without a handoff note when so), or a kept move that could not run. */
export function MoveLine({
  agent,
  at,
  moved,
  failed,
  note,
}: {
  agent: string;
  at: string;
  moved?: TranscriptItem['account_moved'];
  failed?: TranscriptItem['move_failed'];
  /** The handoff note the session after the move started from: null when none was written, undefined when not loaded. */
  note?: string | null;
}) {
  const [shown, setShown] = useState(false);
  // A move that did not run is something to see to: a card, as the chat's other holds are; a move that ran is a line between rules.
  if (!moved) return <MoveFailed agent={agent} at={at} failed={failed} />;
  const m = moved;
  const open = () =>
    void openModal((close) => (
      <ModalFrame title={`Moved to ${m.to}`} onClose={() => close()}>
        <div className="flex flex-col gap-6 pb-1">
          <Facts
            rows={[
              ['By', m.cause === 'manual' ? 'You' : 'Handline'],
              ['Why', m.reason || 'Not recorded'],
              ['From', m.from],
              ['To', m.to],
              ['When', new Date(at).toLocaleString()],
            ]}
          />
          {(note !== undefined || !m.note_written) && <HandoffLines text={note ?? null} folded={false} />}
        </div>
      </ModalFrame>
    ));
  return (
    <div className="my-3 flex h-10 items-center gap-4 text-sm text-muted">
      <div className="h-px flex-1 bg-border" />
      <button
        type="button"
        onClick={open}
        onPointerEnter={() => setShown(true)}
        onPointerLeave={() => setShown(false)}
        onFocus={() => setShown(true)}
        onBlur={() => setShown(false)}
        className="flex flex-col items-center rounded-md px-2 py-0.5 transition-colors outline-none hover:bg-hover/30 hover:text-fg focus-visible:ring-2 focus-visible:ring-accent"
      >
        <span className="flex items-center gap-1.5">
          <ArrowRightLeft size={14} />
          {`Moved to ${m.to}${m.note_written ? '' : ' · no handoff note'}`}
        </span>
        <motion.time
          dateTime={at}
          initial={false}
          animate={shown ? { height: 'auto', opacity: 1 } : { height: 0, opacity: 0 }}
          transition={spring}
          className="overflow-hidden text-xs"
        >
          <Ago at={at} />
        </motion.time>
      </button>
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
          {failed?.reason && (
            <div className="text-muted">
              <Refs text={failed.reason} />
            </div>
          )}
        </div>
        <SwitchPick rows={rows} pending={false} go={go} />
      </div>
    </div>
  );
}
