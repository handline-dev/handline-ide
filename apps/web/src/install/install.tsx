// Installing a missing tool: its command runs in a window of the daemon's tmux, shown as one line (Working), never in a Terminal.
import { useRead, useStatus, useWrite } from '@handline/app-data';
import type { CallParams } from '@handline/client';
import { useState, type ComponentProps, type ReactNode } from 'react';
import { Working } from '#src/tools/Working.tsx';
import { Button } from '#src/ui/Button.tsx';

type Tool = CallParams<'tools.install'>['tool'];
const TOOLS: readonly string[] = ['claude', 'codex', 'git'] satisfies Tool[];
/** Whether the daemon can install `tool` (tools.install). */
export const canInstall = (tool: string): tool is Tool => TOOLS.includes(tool);

type Status = ReturnType<typeof useStatus>['data'];
/** Every tool daemon.status lists missing (missing_tools, or its vendor not installed), with its install run if it has one. */
const missingRows = (s: Status) => [
  ...(s?.missing_tools ?? []),
  ...(s?.vendors ?? []).filter((v) => !v.installed).map((v) => ({ ...v, tool: v.binary })),
];
const useRow = (tool: string) => missingRows(useStatus().data).find((r) => r.tool === tool) ?? null;

/** The tool's install command while it is missing, else null. */
export const useMissing = (tool: string): string | null => useRow(tool)?.fix ?? null;

/** The install of `tool`, as daemon.status tells it: whether it runs, whether the last one failed, its window, and start. */
export function useInstall(tool: string) {
  const run = useRow(tool)?.install;
  const { refetch } = useStatus();
  const { mutateAsync } = useWrite('tools.install');
  // From the press until daemon.status, read again, carries the run (no event says it started).
  const [starting, setStarting] = useState(false);
  const known = canInstall(tool);
  const running = known && (starting || (!!run && !run.ended));
  const start = () => {
    if (!known || running) return;
    setStarting(true);
    // A refusal is reported by the write.
    mutateAsync({ tool })
      .then(() => refetch())
      .catch(() => {})
      .finally(() => setStarting(false));
  };
  // An ended run whose tool is still missing did not install it; its window's last line says why.
  return { running, failed: !running && !!run?.ended, terminal: run?.terminal ?? null, start };
}

/** Mounted once: while an install runs, daemon.status is read again every few seconds, as nothing tells it when the tool lands. */
export function InstallPoll() {
  const running = missingRows(useStatus().data).some((r) => r.install && !r.install.ended);
  useRead('daemon.status', {}, { refetchMs: running ? 3000 : undefined });
  return null;
}

/** "Installing <tool>…" while its install runs, or that it did not install after one failed, its terminal a click away. */
export function Installing({ tool }: { tool: string }) {
  const { running, failed, terminal } = useInstall(tool);
  if (running) return <Working label={`Installing ${tool}…`} terminal={terminal} />;
  return failed ? <Working failed label={`${tool} did not install.`} terminal={terminal} /> : null;
}

/** Install for `tool`, gone while its install runs. */
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
