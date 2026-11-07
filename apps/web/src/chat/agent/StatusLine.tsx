import { useAgentDetail, useAgents } from '@handline/app-data';
import { AccountItem } from '#src/chat/agent/AccountItem.tsx';
import { BackgroundItem } from '#src/chat/agent/BackgroundItem.tsx';
import { BranchItem } from '#src/chat/agent/BranchItem.tsx';
import { openTerminalWindow } from '#src/tools/terminalWindow.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { SquareTerminal } from 'lucide-react';

/** Under the message box, quiet: the branch the agent works on, the account it runs on, and what runs behind its prompt. Resting on
 * one shows more. At its right end, the agent's terminal in a new window. Its row is kept while the agent loads, so the box above it does not move. */
export function StatusLine({ agent }: { agent: string }) {
  const detail = useAgentDetail(agent).data;
  // From the list, which the agents feed keeps current.
  const jobs = useAgents().data?.agents.find((a) => a.name === agent)?.background ?? [];
  return (
    <div className="flex h-8 items-center gap-5 overflow-hidden pb-2 pl-2 text-xs text-muted">
      {detail?.worktree && <BranchItem agent={agent} branch={detail.worktree.branch} path={detail.worktree.path} />}
      {detail?.account && <AccountItem agent={agent} account={detail.account} fire={detail.fire ?? null} />}
      <BackgroundItem jobs={jobs} />
      {detail?.terminal && (
        <IconButton className="ml-auto" label="Open the terminal in a new window" onClick={() => openTerminalWindow(agent)}>
          <SquareTerminal size={14} />
        </IconButton>
      )}
    </div>
  );
}
