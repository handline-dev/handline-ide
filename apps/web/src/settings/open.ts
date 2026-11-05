import { useSyncExternalStore } from 'react';

/** Whether Settings covers the app: opened from the sidebar or a link, closed by its X or Escape. */
let open = false;
const listeners = new Set<() => void>();
const set = (v: boolean) => {
  open = v;
  for (const l of listeners) l();
};
export const openSettings = () => set(true);
export const closeSettings = () => set(false);
export const useSettingsOpen = () =>
  useSyncExternalStore(
    (l) => (listeners.add(l), () => listeners.delete(l)),
    () => open,
  );
