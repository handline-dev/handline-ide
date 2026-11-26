// What the agents' heavy runs cost the Mac (daemon.status run_load, run.load), as the status bar, its popover and Settings say it.
import type { Methods, RunLoad } from '@handline/client';
import { shortSpan } from '#src/time.ts';

export type RunPolicy = RunLoad['policy'];
export type RunLoadView = NonNullable<Methods['run.load']['result']>;

export const RUN_POLICIES = ['observe', 'priority', 'turns', 'fixed'] as const satisfies readonly RunPolicy[];
export const RUN_POLICY_WORD: Record<RunPolicy, string> = {
  observe: 'Only tell me',
  priority: 'Lower priority',
  turns: 'Take turns',
  fixed: 'Always take turns',
};
/** What each policy does, in a sentence or two. */
export const RUN_POLICY_LINE: Record<RunPolicy, string> = {
  observe: 'Nothing waits and nothing changes. The status bar says when the Mac is slow.',
  priority:
    "While the Mac is slow, the heavy runs going get a lower priority for the rest of their run, so the app and the agents' own turns stay quick. Nothing waits.",
  turns:
    'While the Mac is slow, new heavy runs wait their turn and the ones going get a lower priority for the rest of their run. The limit follows the Mac and goes once it is calm.',
  fixed: 'Heavy runs always take turns, slow or not. A run waits at most 15 minutes.',
};

const count = (n: number, one: string) => `${n} ${one}${n === 1 ? '' : 's'}`;

/** The status bar's words, null when there is nothing to say: a calm Mac with no run waiting. */
export function runLoadChip(r: RunLoad | null | undefined): { text: string; slow: boolean } | null {
  if (!r) return null;
  const waiting = r.waiting ? ` · ${r.waiting} waiting` : '';
  if (r.slow) return { text: `Mac is slow · ${count(r.heavy, 'heavy run')}${waiting}`, slow: true };
  return r.waiting ? { text: `${count(r.waiting, 'run')} waiting for a turn`, slow: false } : null;
}

/** Each cause as a sentence with its figure, in the daemon's order. */
export function causeLines(l: Pick<RunLoadView, 'causes' | 'late' | 'timed_out' | 'loop_ms'>): string[] {
  return l.causes.map((c) =>
    c === 'runs'
      ? `${count(l.late, 'heavy run')} ${l.late === 1 ? 'is' : 'are'} taking longer than usual`
      : c === 'timeouts'
        ? `${count(l.timed_out, 'heavy run')} ran out of time in the last 5 minutes`
        : `Handline's own loop took ${shortSpan(l.loop_ms / 1000)}`,
  );
}

/** What the Mac says of itself, to read beside the causes. */
export function hostLine(h: RunLoadView['host']): string {
  const parts = [`Load ${h.load_per_core.toFixed(1)} per core`];
  if (h.memory_pressure && h.memory_pressure !== 'normal') parts.push(`memory pressure ${h.memory_pressure}`);
  if (h.compressed_pages_per_s >= 1000) parts.push(`compressing ${Math.round(h.compressed_pages_per_s / 1000)}k pages/s`);
  return parts.join(' · ');
}

/** What is done about heavy runs right now, in one line. */
export function policyNow(l: Pick<RunLoadView, 'policy' | 'slow' | 'cap'>): string {
  switch (l.policy) {
    case 'priority':
      return l.slow ? 'The heavy runs going now have a lower priority.' : 'Heavy runs get a lower priority when the Mac is slow.';
    case 'turns':
      if (l.cap === null) return 'New heavy runs take turns when the Mac is slow.';
      return `New heavy runs take turns: at most ${l.cap} at once${l.slow ? '' : ', more as the Mac stays calm'}.`;
    case 'fixed':
      return `Heavy runs take turns: at most ${l.cap ?? 0} at once.`;
    default:
      return 'Nothing waits: Handline only tells you.';
  }
}

/** How long a run has gone against what its shape usually takes: "9m · usually 6m", or "40s" when no usual time is known. */
export function runTime(started: string, usualS: number | null, now = Date.now()): string {
  const ran = shortSpan((now - Date.parse(started)) / 1000);
  return usualS ? `${ran} · usually ${shortSpan(usualS)}` : ran;
}
