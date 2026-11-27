import type { ReactNode } from 'react';
import { RotateCw } from 'lucide-react';
import { useRefs } from '#src/ui/refs.tsx';
import { useAccounts, useAgentDetail, useAgents, useUnstall, why } from '@handline/app-data';
import { activityOf, holdLine, restarting } from '#src/layout/agentList.ts';
import { openSettings } from '#src/settings/open.ts';
import { useOpenable } from '#src/layout/openable.tsx';
import { Button, useAction } from '#src/ui/Button.tsx';
import { ActivityIcon } from '#src/ui/activityIcon.tsx';
import { toast } from '#src/ui/toast.tsx';
import { Dots } from '#src/chat/Dots.tsx';

/** Whether the agent's account needs a login: the card's Log in. */
function useNeedsLogin(a: { account: string | null } | undefined, act: string | null) {
  const accounts = useAccounts().data?.accounts;
  return act === 'error' && accounts?.find((x) => x.name === a?.account)?.login_state === 'needs_login';
}

/** While working with background jobs and no other hold line: how many. */
function jobsLine(a: { background?: unknown[] }, act: string | null) {
  const n = a.background?.length ?? 0;
  return act === 'working' && n > 0 ? `Waiting on ${n} background ${n === 1 ? 'job' : 'jobs'}` : null;
}

/** A wait by design, for the foot's dots row while the turn is still: its mark and its line; null when the card says it. */
export function useHoldLine(agent: string) {
  const a = useAgents().data?.agents.find((x) => x.key === agent);
  const act = a ? activityOf(a) : null;
  const login = useNeedsLogin(a, act);
  const refs = useRefs();
  const asked = !!useAgentDetail(agent).data?.prompt; // the prompt card says it
  const line = a && !restarting(a) && (act === 'waking' ? wakingLine(a) : (holdLine(a) ?? jobsLine(a, act))); // a restart shows in the turn's row already
  if (!a || !act || !line || login || asked || a.needs_you?.source === 'daemon') return null;
  return { act, line: refs(line) };
}

/** A closed agent a message woke, until the message is typed: who, and the daemon's step. */
const wakingLine = (a: Parameters<typeof holdLine>[0] & { name: string }) => [`Waking ${a.name}…`, holdLine(a)].filter(Boolean).join(' · ');

/** Over a task's comment box while its holder wakes for the comment: the chat foot's mark and words. */
export function WakeLine({ agent }: { agent: string }) {
  const hold = useHoldLine(agent);
  if (hold?.act !== 'waking') return null;
  return (
    <span role="status" className="flex items-center gap-2 text-xs text-muted">
      <span className="grid size-4 shrink-0 place-items-center">
        <ActivityIcon act="waking" />
      </span>
      <span className="min-w-0 truncate">{hold.line}</span>
    </span>
  );
}

/** The chat foot's row, always there so nothing moves: while the turn runs, the dots with the restart, the held task and its minutes,
 * and the newest tool call; while it is still, what the agent waits on, with its mark in the dots' place. */
export function FootRow({
  busy,
  hold,
  restart,
  move,
  held,
  doing,
  turn,
}: {
  /** A move under way, drawn as its steps in place of the restart's words. */
  move?: ReactNode;
  busy: boolean;
  hold: ReturnType<typeof useHoldLine>;
  restart: string | null;
  held: { id: number } | null | undefined;
  doing: string | undefined;
  turn: string;
}) {
  return (
    <div className={`h-5 transition-opacity ${busy || hold ? 'opacity-100' : 'opacity-0'}`} aria-hidden={!busy && !hold}>
      {!busy && hold ? (
        <span className="flex items-center gap-2 text-xs text-muted">
          <span className="grid size-4 shrink-0 place-items-center">
            <ActivityIcon act={hold.act} />
          </span>
          <span className="min-w-0 truncate">{hold.line}</span>
        </span>
      ) : move ? (
        <span className="flex items-center gap-2">
          <Dots />
          {move}
        </span>
      ) : (
        <span className="flex items-center gap-2">
          <Dots />
          <span className="min-w-0 truncate text-xs text-muted">
            {restart && <span className="font-medium text-fg">{restart}</span>}
            {restart && (held || doing) && ' · '}
            {held && (
              <span className="font-medium text-fg">
                T{held.id} · {turn}
              </span>
            )}
            {held && doing && ' · '}
            {doing}
          </span>
        </span>
      )}
    </div>
  );
}

/** At the chat's foot, beside the pause and prompt cards: a fault the daemon raised (or an account to log in to), why in full, and the
 * one button that gets the agent going. */
export function HoldCard({ agent }: { agent: string }) {
  const a = useAgents().data?.agents.find((x) => x.key === agent);
  const refs = useRefs();
  const unstall = useUnstall(agent);
  const terminal = useOpenable('shell');
  const wake = useAction();
  // A switch offer is the switch card's (SwitchWait), drawn with its accounts; here it would show the same offer twice.
  const need = a?.needs_you?.source === 'daemon' && a.needs_you.kind !== 'switch_offer' ? a.needs_you : null;
  const act = a ? activityOf(a) : null;
  const needsLogin = useNeedsLogin(a, act);
  if (!a || !act || (!need && !needsLogin)) return null;
  const button =
    need?.kind === 'start_failed' || need?.kind === 'no_session' ? (
      <Button
        busy={wake.busy}
        // A refused wake has cleared the stall all the same, so this card may be gone by the answer: the reason is a toast.
        onClick={() => void wake.go(() => unstall().catch((e: unknown) => toast(why(e), { tone: 'warning' })))}
      >
        <RotateCw size={14} />
        Try again
      </Button>
    ) : needsLogin ? (
      <Button onClick={() => openSettings({ section: 'accounts', account: a.account ?? undefined })}>Log in</Button>
    ) : need?.kind === 'trust' || need?.kind === 'dialog' ? (
      <Button {...terminal}>Open terminal</Button>
    ) : null;
  return (
    <div role="status" className="flex max-w-150 flex-col gap-3 rounded-lg border border-border p-3 text-sm">
      <div className="flex gap-2.5">
        <span className="mt-0.5 grid size-4 shrink-0 place-items-center">
          <ActivityIcon act={act} />
        </span>
        <div className="flex min-w-0 flex-col gap-0.5 break-words">
          <span className="font-medium">{refs(need?.title ?? holdLine(a) ?? '')}</span>
          {need?.text && <span className="text-muted">{refs(need.text)}</span>}
        </div>
      </div>
      {button && <div className="flex justify-end">{button}</div>}
    </div>
  );
}
