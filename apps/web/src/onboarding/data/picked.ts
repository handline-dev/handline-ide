// The project picked on the project step, for the agent step; kept for this window's life.
import { createStore } from '#src/persist/store.ts';

const picked = createStore<{ path: string; name: string } | null>(null);
export const setPicked = picked.set;
export const usePicked = picked.use;
