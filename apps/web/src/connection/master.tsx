// The master as the page knows it: the machines kept from node.status, the master's name for "Can't reach the master", and
// making this Mac the master of a joined cluster (the desktop shell's promoteCluster).
import { useRead } from '@handline/app-data';
import type { Node } from '@handline/client';
import { createContext, useContext, useEffect } from 'react';
import { shell } from '#src/shell.ts';
import { confirm } from '#src/ui/modal.tsx';

/** The master last joined: its name ('' before one was), and whether the owner has more than one machine. */
export const MasterName = createContext({ name: '', several: false });

/** Hands node.status to `seen` whenever it changes, so the kept machines follow it. */
export function KeepMachines({ seen }: { seen: (s: { master: { id: string; number: number }; nodes: Node[] }) => void }) {
  const status = useRead('node.status', {}).data;
  useEffect(() => {
    if (status) seen(status);
  }, [status, seen]);
  return null;
}

/** Makes this Mac the master of joined cluster `id`: its engine takes the board over (node.promote); the page follows by itself once
 * the new master answers. */
export async function promoteHere(id: string) {
  const promote = shell()?.promoteCluster;
  if (!promote) throw new Error('Only the desktop app can make this Mac the master.');
  const r = await promote(id);
  if (!r.ok) throw new Error(r.message);
}

/** A device's Make master, asked once. */
export function confirmMakeMaster(name: string, run: () => Promise<unknown>) {
  void confirm({
    title: `Make ${name} the master?`,
    message: `The board moves to ${name}, and the master's agents follow as they next start. Work pauses for a moment.`,
    confirmText: 'Make master',
    run,
  });
}

export const useMasterName = () => useContext(MasterName);
