import { useAgentDetail, useAgents } from '@handline/app-data';
import { AccountItem } from '#src/chat/agent/AccountItem.tsx';
import { BackgroundItem } from '#src/chat/agent/BackgroundItem.tsx';
import { BranchItem } from '#src/chat/agent/BranchItem.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { SquareTerminal } from 'lucide-react';

/** Under the message box, quiet: the branch the agent works on, the account it runs on, and what runs behind its prompt. Resting on
 * one shows more. At its right end, the agent's terminal, opened as a tab. Its row is kept while the agent loads, so the box above it does not move. */
export function StatusLine({ agent }: { agent: string }) {
  const detail = useAgentDetail(agent).data;
  // From the list, which the agents feed keeps current.
  const jobs = useAgents().data?.agents.find((a) => a.key === agent)?.background ?? [];
  return (
    <div className="flex h-8 items-center gap-5 overflow-hidden pb-2 pl-2 text-xs text-muted">
      {detail?.worktree && <BranchItem agent={agent} branch={detail.worktree.branch} path={detail.worktree.path} />}
      {detail?.account && <AccountItem agent={agent} account={detail.account} fire={detail.fire ?? null} />}
      <BackgroundItem jobs={jobs} />
      {detail?.terminal && <TerminalButton />}
    </div>
  );
}

/** The agent's own terminal, opened as a tab the app's way (a click: in the chat's pane); one already open comes up. */
function TerminalButton() {
  return (
    <IconButton className="ml-auto" label="Open the agent's terminal" {...useOpenable('shell')}>
      <SquareTerminal size={14} />
    </IconButton>
  );
}
