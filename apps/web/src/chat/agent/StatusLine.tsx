import { useAgentDetail } from '@handline/app-data';
import { AccountItem } from '#src/chat/agent/AccountItem.tsx';
import { BranchItem } from '#src/chat/agent/BranchItem.tsx';

/** Under the message box, quiet: the branch the agent works on and the account it runs on. Resting on either shows more, a click
 * opens its tab (Changes, Usage). Its row is kept while the agent loads, so the box above it does not move. */
export function StatusLine({ agent }: { agent: string }) {
  const detail = useAgentDetail(agent).data;
  return (
    <div className="flex h-8 items-center gap-5 overflow-hidden pb-2 pl-2 text-xs text-muted">
      {detail?.worktree && <BranchItem agent={agent} branch={detail.worktree.branch} path={detail.worktree.path} />}
      {detail?.account && <AccountItem agent={agent} account={detail.account} fire={detail.fire ?? null} />}
    </div>
  );
}
