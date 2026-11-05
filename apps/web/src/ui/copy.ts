// Copying to the clipboard, the one way: a failure is always said (a toast), and a copy with nothing else to show it took says so too.
import { useState } from 'react';
import { toast } from '#src/ui/toast.tsx';

/** Copies `what` (text, or a picture's blob). `name` says what it is in the toast; `say` also toasts when it took, for a menu item that closes. */
export async function copy(what: string | Blob, name: string, { say = false } = {}): Promise<boolean> {
  try {
    if (typeof what === 'string') await navigator.clipboard.writeText(what);
    else await navigator.clipboard.write([new ClipboardItem({ [what.type]: what })]);
    if (say) toast(`Copied ${name}.`);
    return true;
  } catch {
    toast(`Could not copy ${name}.`, { tone: 'warning' });
    return false;
  }
}

/** For a button that shows it took: `copied` is true for a moment after a copy that worked. */
export function useCopy(): { copied: boolean; copy: (what: string | Blob, name: string) => void } {
  const [copied, setCopied] = useState(false);
  return {
    copied,
    copy: (what, name) =>
      void copy(what, name).then((ok) => {
        if (!ok) return;
        setCopied(true);
        setTimeout(() => setCopied(false), 1500);
      }),
  };
}
