import { useAgents, usePending, useServes, useWrite } from '@handline/app-data';
import { toast } from '#src/ui/toast.tsx';

/** Where a compaction stands, for the message box: `writing` from the ask (the daemon waits for the turn to end) while the agent writes its handoff, `starting`
 * while the new one comes up; null when none, and the box takes messages again. */
export function useCompacting(agent: string): 'writing' | 'starting' | null {
  const a = useAgents().data?.agents.find((x) => x.key === agent);
  const asked = usePending('session.compact').some((p) => p.agent === agent);
  if (a?.handoff?.phase === 'starting' || (!asked && a?.state === 'starting')) return 'starting';
  if (asked || a?.handoff?.phase === 'writing') return 'writing';
  return null;
}

export const COMPACTING_WORDS = { writing: 'Getting ready to compact…', starting: 'Waking up again…' } as const;

/** Compact now: the daemon's full-context handoff (the note, then a fresh session); null when the daemon does not serve it. */
export function useCompactNow(agent: string): (() => void) | null {
  const compact = useWrite('session.compact', agent);
  if (!useServes('session.compact').served) return null;
  return () =>
    compact.mutate(
      { agent },
      { onError: (e) => toast(`Compact did not start: ${e instanceof Error ? e.message : String(e)}`, { tone: 'warning' }) },
    );
}
