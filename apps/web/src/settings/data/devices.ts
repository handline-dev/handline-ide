// Settings > Devices: the paired devices (the one this page runs on marked `current`) and revoking one.
import type { Device } from '@handline/client';
import { hooks, useServes } from '@handline/app-data';
import { useStore } from '@handline/app-react';
import { useEffect, useState } from 'react';
import { useKeyed } from '#src/settings/data/keyed.ts';

export const STALE_DAYS = 30;
export const isStale = (d: Pick<Device, 'last_seen' | 'connected'>, now = Date.now()): boolean =>
  !d.connected && d.last_seen !== null && now - Date.parse(d.last_seen) > STALE_DAYS * 86_400_000;

export function useDevices() {
  const list = hooks.useDevicesList();
  const me = useThisDevice();
  const revoke = hooks.useDeviceRevoke();
  const { pending, run } = useKeyed();
  return {
    loading: list.isPending,
    error: list.isError,
    devices: (list.data?.devices ?? []).map((d) => ({ ...d, current: d.id === me, stale: isStale(d) })),
    pushPlatforms: list.data?.push_platforms ?? [],
    revoke: (id: string) => run(id, () => revoke.mutateAsync({ device: id })).then(() => {}),
    pending,
  };
}

/** The id the daemon knows this browser by (whoami); read again when the connection comes back, so an early miss does not stay. */
function useThisDevice(): string | undefined {
  const { client } = useStore();
  const [id, setId] = useState<string>();
  const { open } = useServes('whoami');
  useEffect(() => {
    if (id !== undefined) return;
    let live = true;
    client.call('whoami', {}).then(
      (r) => live && setId(r.caller.device),
      () => {},
    );
    return () => {
      live = false;
    };
  }, [client, id, open]);
  return id;
}
