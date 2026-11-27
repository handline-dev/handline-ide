// The one action that gets a stopped, held or sleeping agent going again, the same in every view that offers it.
import type { AgentView } from '@handline/client';

export type SetGoing = 'retry' | 'login' | 'terminal' | 'switch' | 'wake';

/** The fix of a need the daemon raised: try a failed start again, or answer a trust screen or a dialog at the terminal. */
export const needFix = (kind: string): 'retry' | 'terminal' | null =>
  kind === 'start_failed' || kind === 'no_session' ? 'retry' : kind === 'trust' || kind === 'dialog' ? 'terminal' : null;

/** `act`: the agent's activity word (asleep, error, held, ...); `login`: its account needs a login; `spare`: another account to switch
 *  to while it waits for one. A need the daemon raised is read first, whatever the act: the daemon draws one as waiting for a person.
 *  Null when nothing gets it going: it works, rests, waits for a person's answer, or its hold ends by itself. */
export function setGoingOf(
  a: Pick<AgentView, 'needs_you' | 'state' | 'status' | 'account_switch'>,
  act: string,
  login: boolean,
  spare: boolean,
): SetGoing | null {
  if (act === 'working' || act === 'waking') return null;
  const fix = a.needs_you?.source === 'daemon' ? needFix(a.needs_you.kind) : null;
  if (fix === 'retry') return 'retry';
  if (login) return 'login';
  if (fix) return fix;
  // One waiting at its limit for an account reads as parked or held: the switch, never a wake onto the walled account.
  if (a.account_switch?.phase === 'waiting') return spare ? 'switch' : null;
  // Only one with no session is woken: a parked one may still hold its session, and wakes by its own clock.
  if (act === 'asleep') return a.state === 'no_terminal' || a.status?.state === 'off' ? 'wake' : null;
  if (act === 'error') return a.state === 'stalled' || a.status?.reason === 'stalled' ? 'retry' : null;
  return null;
}
