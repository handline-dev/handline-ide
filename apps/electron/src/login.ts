// Open at login, behind the two calls Electron makes, so a test drives them: the setting goes to the system only for an installed app
// (an unpackaged run would add the Electron binary itself as a login item), and a launch the system made at login opens no window.

export interface LoginItems {
  setLoginItemSettings(s: { openAtLogin: boolean }): void;
  getLoginItemSettings(): { openAtLogin: boolean; wasOpenedAtLogin?: boolean };
}

/** Hands the setting to the system, when this run is the installed app. */
export function applyOpenAtLogin(app: LoginItems, packaged: boolean, on: boolean): void {
  if (packaged && app.getLoginItemSettings().openAtLogin !== on) app.setLoginItemSettings({ openAtLogin: on });
}

/** Whether the system opened this run at login: the app then keeps to the tray, with no window and no dock icon, until it is opened. */
export function openedAtLogin(app: LoginItems, packaged: boolean): boolean {
  return packaged && app.getLoginItemSettings().wasOpenedAtLogin === true;
}

/** The working count the tray and the questions show: the agents mid-turn in an agents.list answer. */
export function workingOf(list: { agents?: { state?: string }[] }): number {
  return (list.agents ?? []).filter((a) => a.state === 'running').length;
}
