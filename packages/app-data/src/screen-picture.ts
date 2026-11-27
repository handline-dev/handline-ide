// A screen capture's picture (screen.picture): bytes read once when it is opened, not kept as a cache entry.
import { useCallback } from 'react';
import { useStore } from '@handline/app-react';

/** Reads one capture's picture as a Blob of its type. */
export function useReadScreenPicture(): (id: number) => Promise<Blob> {
  const store = useStore();
  return useCallback(
    async (id: number) => {
      const got = await store.client.call('screen.picture', { id });
      return new Blob([Uint8Array.from(atob(got.data), (c) => c.charCodeAt(0))], { type: got.mime });
    },
    [store],
  );
}
