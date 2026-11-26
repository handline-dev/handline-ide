// The work an agent runs beside its own right now, for the strip above its chat box. Pure.
import type { TranscriptItem } from '@handline/client';

/** The tasks whose work beside the agent's own has started and not ended, oldest first, each with when it started. */
export function runningAlongside(items: readonly Pick<TranscriptItem, 'at' | 'concurrent_work'>[]): { task: number; at: string }[] {
  const running = new Map<number, string>();
  for (const it of items) {
    const w = it.concurrent_work;
    if (!w) continue;
    if (w.phase === 'ended') running.delete(w.task_id);
    else if (!running.has(w.task_id)) running.set(w.task_id, it.at);
  }
  return [...running].map(([task, at]) => ({ task, at })).sort((a, b) => Date.parse(a.at) - Date.parse(b.at));
}
