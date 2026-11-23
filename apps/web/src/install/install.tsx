// Installing a missing tool: its command runs in a window of the daemon's tmux, shown where the tool is as one line that says it is
// going (Working), never in a Terminal window.
import { useRead, useStatus, useWrite } from '@handline/app-data';
import type { CallParams } from '@handline/client';
import { useEffect, useSyncExternalStore, type ComponentProps, type ReactNode } from 'react';
import { Working } from '#src/tools/Working.tsx';
import { Button } from '#src/ui/Button.tsx';

type Tool = CallParams<'tools.install'>['tool'];
const TOOLS: readonly string[] = ['claude', 'codex', 'git'] satisfies Tool[];
/** Whether the daemon can install `tool` (tools.install). */
export const canInstall = (tool: string): tool is Tool => TOOLS.includes(tool);

/** The tool's install command when daemon.status lists it missing or its vendor not installed, else null. */
export function useMissing(tool: string): string | null {
  const s = useStatus().data;
  const m = s?.missing_tools?.find((x) => x.tool === tool) ?? s?.vendors?.find((v) => v.binary === tool && !v.installed);
  return m ? m.fix : null;
}

// The installs going, by tool: their window's handle, null until the daemon answers. Kept here so every place showing the tool sees it.
let going: Partial<Record<Tool, string | null>> = {};
const listeners = new Set<() => void>();
const put = (tool: Tool, handle: string | null | undefined) => {
  going = { ...going };
  if (handle === undefined) delete going[tool];
  else going[tool] = handle;
  listeners.forEach((l) => l());
};
const subscribe = (l: () => void) => (listeners.add(l), () => void listeners.delete(l));

/** The install of `tool`: whether it is going, its window, and start. Its window closes once the tool is there. */
export function useInstall(tool: string) {
  const all = useSyncExternalStore(subscribe, () => going);
  const { mutateAsync } = useWrite('tools.install');
  const { mutate: closeShell } = useWrite('shell.close');
  const known = canInstall(tool);
  const running = known && tool in all;
  const terminal = known ? (all[tool] ?? null) : null;
  const missing = useMissing(tool) !== null;
  // An install tells the daemon nothing when it lands: while one is going, what is missing is asked again every few seconds.
  useRead('daemon.status', {}, { refetchMs: running ? 3000 : undefined });
  useEffect(() => {
    if (!known || !running || missing) return;
    put(tool, undefined);
    if (terminal) closeShell({ terminal });
  }, [known, running, missing, tool, terminal, closeShell]);
  const start = () => {
    if (!known || running) return;
    put(tool, null);
    // A refusal is reported by the write.
    mutateAsync({ tool }).then(
      (r) => put(tool, r.terminal),
      () => put(tool, undefined),
    );
  };
  return { running, terminal, start };
}

/** "Installing <tool>…" while its install goes, with its terminal a click away; nothing otherwise. */
export function Installing({ tool }: { tool: string }) {
  const { running, terminal } = useInstall(tool);
  return running ? <Working label={`Installing ${tool}…`} terminal={terminal} /> : null;
}

/** Install for `tool`, gone while its install goes; Installing says so where there is room. */
export function InstallButton({ tool, ...props }: { tool: string } & ComponentProps<typeof Button>) {
  const { running, start } = useInstall(tool);
  return running ? null : <Button {...props} onClick={start} />;
}

/** Where a project is picked, cloned or added: while git is missing, the place says so with Install, and shows `children` once it is there. */
export function NeedsGit({ children }: { children: ReactNode }) {
  if (useMissing('git') === null) return children;
  return (
    <div className="flex flex-col gap-2 pb-4 text-sm">
      <div className="flex items-center gap-3">
        <p className="flex-1">git is needed for a project.</p>
        <InstallButton tool="git" variant="primary">
          Install git
        </InstallButton>
      </div>
      <Installing tool="git" />
    </div>
  );
}
