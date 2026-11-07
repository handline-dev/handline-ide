// The windows the page may open itself, on the page's own origin: the debug window (#/debug/terminals) and an agent's terminal
// (#terminal=<agent>). Anything else is denied.
export const DEBUG_HASH = '#/debug/terminals';

export function isDebugWindow(url: string, origin: string): boolean {
  try {
    const u = new URL(url);
    return u.origin === origin && u.hash === DEBUG_HASH;
  } catch {
    return false;
  }
}

export const TERMINAL_HASH = '#terminal=';

export function isTerminalWindow(url: string, origin: string): boolean {
  try {
    const u = new URL(url);
    return u.origin === origin && u.hash.startsWith(TERMINAL_HASH);
  } catch {
    return false;
  }
}
