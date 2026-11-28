// One agent's screen source, watched (screen.attach until unmounted): its newest frame, who holds it, whether the agent uses it, and its last action.
import { useEffect, useState } from 'react';
import { useStore } from '@handline/app-react';
import type { ScreenBox, ScreenFrame, ScreenHolder, ScreenInput, ScreenSource } from '@handline/client';

export type Action = { kind: string; box: ScreenBox; n: number };
export type Watched = { frame: ScreenFrame | null; holder: ScreenHolder | null; action: Action | null; ended: boolean; failed: string };

/** Views open per source on this client: the daemon is told to stop only when the last one closes. */
const views = new Map<string, number>();

const EMPTY: Watched = { frame: null, holder: null, action: null, ended: false, failed: '' };

export function useScreen(agent: string, source: ScreenSource | null): Watched {
  const store = useStore();
  const [w, setW] = useState<Watched & { key: string }>({ ...EMPTY, key: '' });
  const key = source ? `${agent}/${source}` : '';
  useEffect(() => {
    if (!source) return;
    let live = true;
    let n = 0;
    const set = (f: (w: Watched) => Partial<Watched>) => live && setW((p) => ({ ...p, ...f(p), key }));
    const off = store.client.screen.add((e) => {
      if (e.agent !== agent || e.source !== source) return;
      if (e.kind === 'screen_frame') set(() => ({ frame: { data: e.data, width: e.width, height: e.height } }));
      else if (e.kind === 'screen_holder') set(() => ({ holder: e.holder }));
      else if (e.kind === 'screen_ended') set(() => ({ ended: true }));
      else if (e.box) set(() => ({ action: { kind: e.kind, box: e.box!, n: ++n } }));
    });
    const attach = () =>
      store.client
        .call('screen.attach', { agent, source })
        .then((s) => set(() => ({ frame: s.frame, holder: s.holder, ended: false, failed: '' })))
        .catch((e: unknown) => set(() => ({ failed: (e as Error).message })));
    views.set(key, (views.get(key) ?? 0) + 1);
    void attach();
    const back = store.client.onResume(() => void attach()); // a new connection watches nothing yet
    return () => {
      live = false;
      off();
      back();
      const left = (views.get(key) ?? 1) - 1;
      if (left) views.set(key, left);
      else {
        views.delete(key);
        void store.client.call('screen.detach', { agent, source }).catch(() => {});
      }
    };
  }, [store, agent, source, key]);
  return w.key === key ? w : EMPTY;
}

/** screen.input for the person who holds the source; a refusal is left to the holder notice. */
export function useScreenInput(agent: string, source: ScreenSource) {
  const store = useStore();
  return (event: ScreenInput) =>
    void store.client.call('screen.input', { agent, source, event, request_id: crypto.randomUUID() }).catch(() => {});
}
