// The master as the page knows it: the machines kept from node.status, the master's name for "Can't reach the master", and
// "Make this Mac the master" of a joined cluster (the desktop shell's promoteCluster).
import { Hooks } from '@handline/app-data';
import type { Node } from '@handline/client';
import { createContext, useContext, useEffect } from 'react';
import { shell } from '#src/shell.ts';
import { confirm } from '#src/ui/modal.tsx';

/** The master last joined: its name ('' before one was), and whether the owner has more than one machine. */
export const MasterName = createContext({ name: '', several: false });

/** Hands node.status to `seen` whenever it changes, so the kept machines follow it. */
export function KeepMachines({ seen }: { seen: (s: { master: { id: string; number: number }; nodes: Node[] }) => void }) {
  const status = Hooks.useNodeStatus().data;
  useEffect(() => {
    if (status) seen(status);
  }, [status, seen]);
  return null;
}

/** Asks first, then makes this Mac the master of joined cluster `id` (its engine takes the board over); the page follows by itself once
 * the new master answers. */
export function confirmPromote(id: string, masterGone: boolean) {
  void confirm({
    title: 'Make this Mac the master?',
    message: masterGone
      ? 'This Mac takes over the board from its copy. What the master wrote in its last second or two may be lost.'
      : 'The master hands the board over. Writes pause for a moment.',
    confirmText: 'Make master',
    run: async () => {
      const promote = shell()?.promoteCluster;
      if (!promote) throw new Error('Only the desktop app can make this Mac the master.');
      const r = await promote(id);
      if (!r.ok) throw new Error(r.message);
    },
  });
}

export const useMasterName = () => useContext(MasterName);
