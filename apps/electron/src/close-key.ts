import type { WebContents } from 'electron';

/** The menu's Close Tab click: the page owns the Cmd+W key (it also reaches a focused terminal), so the click sends that key to it. */
export function sendCloseTabKey(contents: WebContents | undefined, mac: boolean): void {
  for (const type of ['keyDown', 'keyUp'] as const) contents?.sendInputEvent({ type, keyCode: 'W', modifiers: [mac ? 'meta' : 'control'] });
}
