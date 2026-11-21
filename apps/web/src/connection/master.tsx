// The master as the page knows it: the machines kept from node.status, the master's name for "Can't reach the master", and
// "Make this Mac the master" when this Mac's daemon is one of the owner's runners (the desktop shell's promote).
import type { ShellDaemon } from '@handline/app-core';
import { Hooks } from '@handline/app-data';
import type { Node } from '@handline/client';
import { createContext, useContext, useEffect, useState } from 'react';
import { shell } from '#src/shell.ts';
import { Confirm } from '#src/settings/DeviceControls.tsx';
import { openModal } from '#src/ui/modal.tsx';
import { Button } from '#src/ui/Button.tsx';

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

/** This Mac's daemon when it is one of the owner's runners, else null. */
export function useThisMacRunner(): ShellDaemon['runner'] {
  const [d, setD] = useState<ShellDaemon | null>(null);
  useEffect(() => shell()?.onDaemon?.(setD), []);
  return typeof shell()?.promote === 'function' ? (d?.runner ?? null) : null;
}

/** Asks first, then makes this Mac the master; the page follows by itself once the new master answers. */
export function confirmPromote(masterGone: boolean) {
  void openModal<void>((close) => (
    <Confirm
      title="Make this Mac the master?"
      text={
        masterGone
          ? 'This Mac takes over the board from its copy. What the master wrote in its last second or two may be lost.'
          : 'The master hands the board over. Writes pause for a moment.'
      }
      act="Make master"
      busyWord="Handing over…"
      go={async () => {
        const promote = shell()?.promote;
        if (!promote) throw new Error('Only the desktop app can make this Mac the master.');
        const r = await promote();
        if (!r.ok) throw new Error(r.message);
      }}
      close={() => close()}
    />
  ));
}

/** The button where the master cannot be reached, when this Mac can become it. */
export function PromoteButton({ className = '' }: { className?: string }) {
  const runner = useThisMacRunner();
  if (!runner) return null;
  return (
    <Button className={`w-fit ${className}`} onClick={() => confirmPromote(true)}>
      Make this Mac the master
    </Button>
  );
}

export const useMasterName = () => useContext(MasterName);
