// A task waiting in line for a machine resource (lease.get): which kind, and who uses it now, said in one line.
import { hooks, useNameOf } from '@handline/app-data';

const NOUN: Record<string, string> = { heavy: 'a test or build run', simulator: 'a simulator', browser: 'a browser', port: 'a port' };

type Kinds = NonNullable<ReturnType<typeof hooks.useLeaseGet>['data']>['kinds'];

export type LeaseWait = { kind: string; text: string };

/** Each waiting task's line, by task number: "Waiting for a simulator · in use by builder". */
export function leaseWaits(kinds: Kinds, nameOf: (key: string) => string): Map<number, LeaseWait> {
  const out = new Map<number, LeaseWait>();
  for (const k of kinds) {
    const users = [...new Set(k.held.filter((h) => !h.reserved_for).map((h) => nameOf(h.agent)))];
    for (const q of k.queue) {
      if (q.task == null) continue;
      const ahead = q.position > 1 ? ` · ${q.position - 1} ahead` : '';
      out.set(q.task, {
        kind: k.kind,
        text: `Waiting for ${NOUN[k.kind] ?? k.kind}${users.length ? ` · in use by ${users.join(', ')}` : ''}${ahead}`,
      });
    }
  }
  return out;
}

export function useLeaseWaits(): Map<number, LeaseWait> {
  const nameOf = useNameOf();
  return leaseWaits(hooks.useLeaseGet().data?.kinds ?? [], nameOf);
}
