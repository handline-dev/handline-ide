import { useSyncExternalStore } from 'react';

/** Where Settings opens: a section, and in it one account, agent or device. Without it, the section it was last on. */
export const SECTIONS = ['onboarding', 'accounts', 'agents', 'tools', 'notifications', 'projects', 'devices'] as const;
export type Section = (typeof SECTIONS)[number];
export const isSection = (s: unknown): s is Section => SECTIONS.some((x) => x === s);
export type SettingsTarget = { section?: Section; account?: string; agent?: string; device?: string };

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

/** The Settings a warning without a place of its own opens: an agent's page for a step-down offer, else Settings as it was left. */
export const settingsFor = (w: { kind: string; agent?: string } | null | undefined): SettingsTarget =>
  w?.kind === 'review' && w.agent ? { section: 'agents', agent: w.agent } : {};
export const openSettingsFor = (w: Parameters<typeof settingsFor>[0]) => openSettings(settingsFor(w));
