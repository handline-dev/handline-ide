import { useSyncExternalStore } from 'react';

/** Where Settings opens: a section, and in Accounts one account's row. Without it, the section it was last on. */
export type SettingsTarget = { section?: 'accounts' | 'machine' | 'devices'; account?: string };

/** Whether Settings covers the app: opened from the sidebar or a link, closed by its X or Escape. */
let open = false;
let target: SettingsTarget = {};
const listeners = new Set<() => void>();
const set = (v: boolean) => {
  open = v;
  for (const l of listeners) l();
};
export const openSettings = (to: SettingsTarget = {}) => {
  target = to;
  set(true);
};
export const closeSettings = () => set(false);
export const settingsTarget = () => target;
export const useSettingsOpen = () =>
  useSyncExternalStore(
    (l) => (listeners.add(l), () => listeners.delete(l)),
    () => open,
  );
