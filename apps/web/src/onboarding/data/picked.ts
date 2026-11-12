// The project picked on the project step, for the agent step; kept for this window's life.
import { useSyncExternalStore } from 'react';

type Picked = { path: string; name: string } | null;
let picked: Picked = null;
const pickedListeners = new Set<() => void>();
export const setPicked = (p: Picked) => {
  picked = p;
  for (const l of pickedListeners) l();
};
export const usePicked = () =>
  useSyncExternalStore(
    (l) => (pickedListeners.add(l), () => pickedListeners.delete(l)),
    () => picked,
  );
