import type { WebContents } from 'electron';

/** The menu's Close Tab click: the page owns the Cmd+W key (it also reaches a focused terminal), so the click sends that key to it. */
export function sendCloseTabKey(contents: WebContents | undefined, mac: boolean): void {
  sendKey(contents, 'W', [mac ? 'meta' : 'control']);
}

/** Reopen Closed Tab is Cmd+Shift+T (Ctrl+Shift+T elsewhere), which the page owns as it owns Close Tab. */
export function sendReopenTabKey(contents: WebContents | undefined, mac: boolean): void {
  sendKey(contents, 'T', [mac ? 'meta' : 'control', 'shift']);
}

function sendKey(contents: WebContents | undefined, keyCode: string, modifiers: ('meta' | 'control' | 'shift')[]): void {
  for (const type of ['keyDown', 'keyUp'] as const) contents?.sendInputEvent({ type, keyCode, modifiers });
}
