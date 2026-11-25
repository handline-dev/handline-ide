// The pair sheet: the addresses this machine can be reached on, and a one-time code for the one picked.
import { Hooks } from '@handline/app-data';
import { useState } from 'react';
import { useKeyed } from '#src/settings/data/keyed.ts';

export function usePairAddresses() {
  const q = Hooks.useConnectionAddresses();
  return { addresses: q.data?.addresses ?? [], loading: q.isPending, error: q.isError };
}

/** Makes sure an address is served: network_addresses becomes the hosts listening now plus this one. A no-op when it already listens. */
export function useOpenAddress() {
  const set = Hooks.useSettingsSet();
  const q = Hooks.useConnectionAddresses();
  return async (host: string) => {
    const now = q.data?.addresses ?? [];
    if (now.find((a) => a.host === host)?.listening) return;
    const listening = now.filter((a) => a.listening).map((a) => a.host);
    await set.mutateAsync({ changes: { network_addresses: [...listening, host] } });
  };
}

export interface Pairing {
  code: string;
  expiresAt: number;
  link: string;
}

export function usePair() {
  const pair = Hooks.useDevicePair();
  const { pending, run } = useKeyed();
  const [error, setError] = useState<string | null>(null);
  return {
    pending: pending !== null,
    error,
    start: async (address?: string): Promise<Pairing> => {
      setError(null);
      try {
        const r = await run('pair', () => pair.mutateAsync(address ? { address } : {}));
        return { code: r.code, expiresAt: Date.parse(r.expires_at), link: r.web_link };
      } catch (e) {
        setError(String(e));
        throw e;
      }
    },
  };
}
