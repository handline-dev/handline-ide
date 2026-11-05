// The pair sheet: the addresses this machine can be reached on, and a one-time code for the one picked.
import { hooks } from '@handline/app-data';
import { useState } from 'react';
import { useKeyed } from '#src/settings/data/keyed.ts';

export function usePairAddresses() {
  const q = hooks.useConnectionAddresses();
  return { addresses: q.data?.addresses ?? [], loading: q.isPending, error: q.isError };
}

export interface Pairing {
  code: string;
  expiresAt: number;
  uri: string;
  link: string | null;
}

export function usePair() {
  const pair = hooks.useDevicePair();
  const { pending, run } = useKeyed();
  const [error, setError] = useState<string | null>(null);
  return {
    pending: pending !== null,
    error,
    start: async (address?: string): Promise<Pairing> => {
      setError(null);
      try {
        const r = await run('pair', () => pair.mutateAsync(address ? { address } : {}));
        return { code: r.code, expiresAt: Date.parse(r.expires_at), uri: r.uri, link: r.web_link };
      } catch (e) {
        setError(String(e));
        throw e;
      }
    },
  };
}
