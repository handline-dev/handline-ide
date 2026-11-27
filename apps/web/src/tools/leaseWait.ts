import type { Methods } from '@handline/client';
// A task waiting in line: for a turn at a shared run (run.list), or for a port (lease.get).
import { useNameOf, useRead } from '@handline/app-data';

type Runs = Methods['run.list']['result']['runs'];
type Kinds = Methods['lease.get']['result']['kinds'];

/** One place in a run's line: the holder first, then the waiters in order. */
export type InLine = { key: string; name: string; since?: string; task?: number; position?: number };

export type LeaseWait = { kind: 'run'; run: string; position: number; holders: InLine[]; waiting: InLine[] } | { kind: 'port' };

/** Each waiting task's wait, by task number: the run it waits for, its place in line and the line; or a port. */
export function leaseWaits(runs: Runs, kinds: Kinds, nameOf: (key: string) => string): Map<number, LeaseWait> {
  const out = new Map<number, LeaseWait>();
  for (const k of kinds) {
    if (k.kind !== 'port') continue;
    for (const q of k.queue) if (q.task != null) out.set(q.task, { kind: 'port' });
  }
  for (const r of runs) {
    const holders = r.held.map((h) => ({ key: h.agent, name: nameOf(h.agent), since: h.since }));
    const waiting = [...r.waiting]
      .sort((a, b) => a.position - b.position)
      .map((w) => ({ key: w.agent, name: nameOf(w.agent), task: w.task, position: w.position }));
    for (const w of r.waiting) {
      if (w.task == null) continue;
      out.set(w.task, { kind: 'run', run: r.name, position: w.position, holders, waiting });
    }
  }
  return out;
}

export function useLeaseWaits(): Map<number, LeaseWait> {
  const nameOf = useNameOf();
  const runs = useRead('run.list', {}, { refetchMs: 10_000 }).data?.runs ?? [];
  return leaseWaits(runs, useRead('lease.get', {}).data?.kinds ?? [], nameOf);
}
