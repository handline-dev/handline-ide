// Seeds for an account switch under way: each phase the working line draws, and the offer when no account has room.
import type { FakeDaemon } from '#src/daemon.ts';

type Seed = (d: FakeDaemon) => void;

const ARCH = 'handline.architect'; // local-ref-ok

const inTwoHours = () => new Date(Date.now() + 2 * 3_600_000).toISOString();

/** Puts the architect, on alpha at its 5-hour limit, in an account switch. */
function switching(
  d: FakeDaemon,
  sw: { phase: 'saving' | 'switching' | 'starting' | 'waiting'; to: string | null; until?: string; waits?: string },
): void {
  const s = d.sessions.get(ARCH)!;
  const since = new Date(Date.now() - 20_000).toISOString();
  s.fx.account = sw.to === 'beta' && sw.phase === 'starting' ? 'beta' : 'alpha';
  s.fx.view = { ...s.fx.view, account_switch: { cause: 'usage', from: 'alpha', since, until: null, ...sw } };
}

export const SWITCH_SEEDS: Record<string, Seed> = {
  /** The architect's account switch at each phase: what the working line draws from limit to the new session. */
  'switch-saving': (d) => switching(d, { phase: 'saving', to: null, waits: 'background: shell: vite dev' }),
  /** A due compact that has waited 34 minutes for the architect's turn to end. */
  'compact-waiting': (d) => {
    const s = d.sessions.get(ARCH)!;
    s.fx.view = { ...s.fx.view, compact_waiting_since: new Date(Date.now() - 34 * 60_000).toISOString() };
  },
  'switch-switching': (d) => switching(d, { phase: 'switching', to: 'beta' }),
  'switch-starting': (d) => switching(d, { phase: 'starting', to: 'beta' }),
  'switch-waiting': (d) => switching(d, { phase: 'waiting', to: null, until: inTwoHours() }),
  /** The same wait with no account to spare: the card offers beta, and agent.switch_now takes it. */
  'switch-offer': (d) => {
    switching(d, { phase: 'waiting', to: null, until: inTwoHours() });
    const s = d.sessions.get(ARCH)!;
    const since = new Date(Date.now() - 60_000).toISOString();
    s.fx.view = {
      ...s.fx.view,
      needs_you: {
        kind: 'switch_offer',
        since,
        title: 'No account has room to spare',
        text: 'beta is the closest, and may reach its limit again soon.',
        source: 'daemon',
        account: 'beta',
      },
    };
  },
};
