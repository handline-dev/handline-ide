// The update in the bell's list, from the desktop shell: Restart to update once it is built, the step while it builds, Try again after a failure.
import type { ShellUpdate } from '@handline/app-core';
import { RotateCw } from 'lucide-react';
import { useEffect, useState } from 'react';
import { shell } from '#src/shell.ts';

/** The shell's update, null in a plain browser or while there is none. */
export function useShellUpdate(): ShellUpdate | null {
  const [u, setU] = useState<ShellUpdate | null>(null);
  useEffect(() => shell()?.onUpdate?.(setU), []);
  return u && u.phase !== 'idle' ? u : null;
}

export const updateReady = (u: ShellUpdate | null) => u?.phase === 'ready' || u?.phase === 'later';

/** The update's row at the top of the bell's list. */
export function UpdateRow({ u }: { u: ShellUpdate }) {
  const act = (what: 'restart' | 'retry') =>
    void shell()
      ?.updateAct?.(what)
      .catch(() => {});
  const ready = updateReady(u);
  const line = ready
    ? 'A Handline update is ready.'
    : u.phase === 'failed'
      ? `The update did not finish. ${u.reason}`
      : u.step || 'Preparing an update…';
  return (
    <div className="flex items-center gap-2 px-2">
      <RotateCw size={14} className={ready ? 'shrink-0 text-accent' : 'shrink-0 text-muted'} />
      <span className="min-w-0 flex-1 truncate">{line}</span>
      {(ready || u.phase === 'failed') && (
        <button
          type="button"
          onClick={() => act(ready ? 'restart' : 'retry')}
          className="shrink-0 rounded-md bg-accent px-2 py-0.5 text-xs text-accent-fg hover:opacity-90"
        >
          {ready ? 'Restart now' : 'Try again'}
        </button>
      )}
    </div>
  );
}
