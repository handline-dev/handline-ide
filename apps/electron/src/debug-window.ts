// The one window the page may open itself, on the page's own origin: the debug window (#/debug/terminals). Anything else is denied.
export const DEBUG_HASH = '#/debug/terminals';

export function isDebugWindow(url: string, origin: string): boolean {
  try {
    const u = new URL(url);
    return u.origin === origin && u.hash === DEBUG_HASH;
  } catch {
    return false;
  }
}
