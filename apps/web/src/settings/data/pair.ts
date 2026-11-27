// Connect a device: a one-time code and its one link. The daemon picks the address the link names.
import { useWrite } from '@handline/app-data';
import { useState } from 'react';
import { useKeyed } from '#src/settings/data/keyed.ts';

export interface Pairing {
  code: string;
  expiresAt: number;
  link: string;
  /** The self-made certificate's fingerprint on a Wi-Fi link; '' on a Tailscale link. */
  fingerprint: string;
}

export function usePair() {
  const pair = useWrite('device.pair');
  const { pending, run } = useKeyed();
  const [error, setError] = useState<string | null>(null);
  return {
    pending: pending !== null,
    error,
    start: async (host?: string): Promise<Pairing> => {
      setError(null);
      try {
        const r = await run('pair', () => pair.mutateAsync(host ? { host } : {}));
        return { code: r.code, expiresAt: Date.parse(r.expires_at), link: r.web_link, fingerprint: r.fingerprint };
      } catch (e) {
        setError(String(e));
        throw e;
      }
    },
  };
}
