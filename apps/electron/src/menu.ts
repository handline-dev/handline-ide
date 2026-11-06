import os from 'node:os';
import { app, Menu, type WebContents } from 'electron';
import { appMenu } from '#src/app-menu.ts';
import { CHANNELS } from '#src/bridge.ts';
import { sendCloseTabKey } from '#src/close-key.ts';

/** The application menu: its Go entries open the window if it is closed, then send the page to their screen; Close Tab sends the page its key. */
export function setMenu(openWindow: () => Promise<void>, page: () => WebContents | undefined): void {
  const mac = os.platform() === 'darwin';
  const go = (screen: string) => void openWindow().then(() => page()?.send(CHANNELS.navigate, { screen, id: '', event: 'menu' }));
  Menu.setApplicationMenu(Menu.buildFromTemplate(appMenu({ mac, dev: !app.isPackaged, go, closeTab: () => sendCloseTabKey(page(), mac) })));
}
