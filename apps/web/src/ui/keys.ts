/** Whether the keyboard is a Mac's: Cmd where others press Ctrl. */
export const MAC = /Mac|iPhone|iPad/.test(globalThis.navigator?.platform ?? '');

/** The platform's command key alone, of Cmd and Ctrl: Cmd on a Mac, Ctrl elsewhere. */
export const primaryMod = (e: { metaKey: boolean; ctrlKey: boolean }, mac = MAC) =>
  mac ? e.metaKey && !e.ctrlKey : e.ctrlKey && !e.metaKey;
