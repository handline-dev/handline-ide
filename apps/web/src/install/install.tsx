// Installing a missing tool: its command runs in a window of the daemon's tmux, shown here in a modal, never in a Terminal window.
import { useRead, useStatus, useWrite } from '@handline/app-data';
import type { CallParams } from '@handline/client';
import { useEffect, useState } from 'react';
import { PaneTab } from '#src/tools/TerminalTab.tsx';
import { Button } from '#src/ui/Button.tsx';
import { ModalFrame, openModal, type Close } from '#src/ui/modal.tsx';

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

/** Opens the install of `tool` (one the daemon can install); resolves once the modal closes, by itself when the tool is there. */
export const installTool = async (tool: string) => {
  if (!canInstall(tool)) return;
  await openModal<void>((close) => (
    <ModalFrame title={`Install ${tool}`} size="xl" onClose={() => close()}>
      <Install tool={tool} close={close} />
    </ModalFrame>
  ));
};

function Install({ tool, close }: { tool: Tool; close: Close<void> }) {
  const { mutateAsync: start } = useWrite('tools.install');
  const { mutate: closeShell } = useWrite('shell.close');
  const [handle, setHandle] = useState<string | null>(null);
  const missing = useMissing(tool) !== null;
  // An install tells the daemon nothing when it lands: while the modal is open, what is missing is asked again every few seconds.
  useRead('daemon.status', {}, { refetchMs: 3000 });
  // The window goes with the modal, also when its answer comes after the modal closed. A refusal is reported by the write.
  useEffect(() => {
    if (!missing) return close();
    let gone = false,
      opened: string | null = null;
    start({ tool }).then(
      (r) => (gone ? closeShell({ terminal: r.terminal }) : setHandle((opened = r.terminal))),
      () => {},
    );
    return () => {
      gone = true;
      if (opened) closeShell({ terminal: opened });
    };
  }, [missing, close, start, closeShell, tool]);
  return (
    <div className="flex flex-col gap-3 pb-4">
      <div className="flex h-72 min-h-0 overflow-hidden rounded-md">{handle && <PaneTab handle={handle} label={`Install ${tool}`} />}</div>
      <div className="flex justify-end">
        <Button onClick={() => close()}>Close</Button>
      </div>
    </div>
  );
}

/** Where a project is picked, cloned or added: while git is missing, the place says so with Install, and shows `children` once it is there. */
export function NeedsGit({ children }: { children: React.ReactNode }) {
  if (useMissing('git') === null) return children;
  return (
    <div className="flex items-center gap-3 pb-4 text-sm">
      <p className="flex-1">git is needed for a project.</p>
      <Button variant="primary" onClick={() => void installTool('git')}>
        Install git
      </Button>
    </div>
  );
}
