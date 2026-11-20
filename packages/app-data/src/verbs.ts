// The writes of the agent verbs (wake, interrupt, restart, close, park, move account, clear stall), shared by the web menu and the phone's.
import { callOf, type Verb, type WorktreeAnswer } from '@handline/app-core';
import { useWrite } from '#src/write.ts';

/** What a caller may ask to hear of a verb's write: it settled, or the daemon refused it. */
export interface VerbDone {
  onSuccess?: () => void;
  onError?: (e: unknown) => void;
}

/** Starts a verb's write. */
export function useVerbRun(agent: string) {
  const interrupt = useWrite('session.interrupt', agent);
  const stop = useWrite('session.stop', agent);
  const restart = useWrite('session.restart', agent);
  const park = useWrite('session.park', agent);
  const move = useWrite('agent.move', agent);
  const unstall = useWrite('agent.unstall', agent);
  const forceStart = useWrite('agent.force_start', agent);
  return (v: Verb, account?: string, on?: VerbDone) => {
    const p = callOf(v, agent, account).params as never;
    if (v.method === 'session.interrupt') interrupt.mutate(p, on);
    else if (v.method === 'session.stop') stop.mutate(p, on);
    else if (v.method === 'session.restart') restart.mutate(p, on);
    else if (v.method === 'session.park') park.mutate(p, on);
    else if (v.method === 'agent.move') move.mutate(p, on);
    else if (v.method === 'agent.unstall') unstall.mutate(p, on);
    else if (v.method === 'agent.force_start') forceStart.mutate(p, on);
  };
}

/** Clears the agent's stall (Wake it) and resolves with the daemon's answer, which comes once its start has begun or failed. A promise,
 * not mutate's callbacks: the stall clearing can close the screen that asked before the answer lands. */
export function useUnstall(agent: string) {
  const unstall = useWrite('agent.unstall', agent);
  return () => unstall.mutateAsync({ agent });
}

/** Retires the agent (the founder's Delete agent): the daemon refuses while it has a running session or others hang under it, and says why.
 * A worktree that holds work refuses it too (gate dirty, Failure.held) until the call says what to do with it. */
export function useRetireAgent(agent: string) {
  const retire = useWrite('agent.retire', agent);
  // The agent leaving closes the screen that asked, which would drop mutate's own callbacks: the promise still answers.
  return (on?: VerbDone, worktree?: WorktreeAnswer) =>
    void retire.mutateAsync({ agent, worktree }).then(
      () => on?.onSuccess?.(),
      (e: unknown) => on?.onError?.(e),
    );
}
