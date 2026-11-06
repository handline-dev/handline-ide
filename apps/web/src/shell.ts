// The desktop shell's calls (window.handline, apps/electron api.ts); none in a plain browser.
import type { ShellApi } from '@handline/app-core';

export const shell = (): Partial<ShellApi> | null => {
  const s = (window as { handline?: Partial<ShellApi> }).handline;
  return s?.api === 1 ? s : null;
};
