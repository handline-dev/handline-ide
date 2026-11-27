import { createStore } from '#src/persist/store.ts';

/** Where Settings opens: a section, and in it one account, agent or device. Without it, the section it was last on. */
export const SECTIONS = [
  'general',
  'onboarding',
  'accounts',
  'agents',
  'schedules',
  'tools',
  'captures',
  'notifications',
  'projects',
  'devices',
] as const;
export type Section = (typeof SECTIONS)[number];
export const isSection = (s: unknown): s is Section => SECTIONS.some((x) => x === s);
/** `cluster`: whose settings, '' (this Mac's) when left out. */
export type SettingsTarget = { section?: Section; account?: string; device?: string; cluster?: string };

/** Whether Settings covers the app: opened from the sidebar or a link, closed by its X or Escape. */
const open = createStore(false);
let target: SettingsTarget = {};
export const openSettings = (to: SettingsTarget = {}) => {
  target = to;
  open.set(true);
};
export const closeSettings = () => open.set(false);
export const settingsTarget = () => target;
export const useSettingsOpen = open.useValue;
