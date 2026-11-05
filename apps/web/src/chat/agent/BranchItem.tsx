import { useWorktrees } from '@handline/app-data';
import { Check, Copy, FileDiff, GitBranch } from 'lucide-react';
import { useState } from 'react';
import { CardAction, chip, MenuHeading, MenuRule } from '#src/chat/agent/menuParts.tsx';
import { focusIn } from '#src/layout/focus.ts';
import { howOf, openProps, useOpenTab } from '#src/layout/openTab.tsx';
import { HoverCard } from '#src/ui/HoverCard.tsx';
import { toast } from '#src/ui/toast.tsx';

/** The branch: resting on it shows its changed files and copies its name; a click opens the Changes tab with the diff. */
export function BranchItem({ agent, branch, path }: { agent: string; branch: string; path: string }) {
  const changed = useWorktrees({ owner: agent }).data?.worktrees.find((w) => w.path === path)?.changed ?? [];
  const openTab = useOpenTab();
  // The copy button says it took, in place, for a moment: the card stays open under the pointer, so it is seen there.
  const [copied, setCopied] = useState(false);
  const added = changed.reduce((n, f) => n + f.added, 0);
  const deleted = changed.reduce((n, f) => n + f.deleted, 0);
  const biggest = [...changed].sort((a, b) => b.added + b.deleted - (a.added + a.deleted)).slice(0, 5);
  return (
    <HoverCard
      trigger={
        <button type="button" {...openProps((how) => openTab('diff', how))} className={chip}>
          <GitBranch size={13} className="shrink-0" />
          <span className="truncate">{branch}</span>
          {changed.length > 0 && (
            <span className="shrink-0 tabular-nums">
              <span className="text-success">+{added}</span> <span className="text-danger">−{deleted}</span>
            </span>
          )}
        </button>
      }
    >
      {changed.length > 0 && (
        <>
          <MenuHeading>{`${changed.length} files changed`}</MenuHeading>
          {biggest.map((f) => (
            <button
              key={f.path}
              type="button"
              {...openProps((how) => {
                focusIn('diff', agent, f.path);
                openTab('diff', how);
              })}
              className="flex h-6 w-full items-center justify-between gap-6 rounded-sm px-2 text-xs outline-none hover:bg-hover focus-visible:bg-hover"
            >
              <span className="truncate text-fg">{f.path}</span>
              <span className="shrink-0 tabular-nums">
                <span className="text-success">+{f.added}</span> <span className="text-danger">−{f.deleted}</span>
              </span>
            </button>
          ))}
          {changed.length > biggest.length && (
            // The rest: the Changes tab, at the first file not listed here.
            <button
              type="button"
              {...openProps((how) => {
                const first = changed.find((f) => !biggest.includes(f));
                if (first) focusIn('diff', agent, first.path);
                openTab('diff', how);
              })}
              className="flex h-6 w-full items-center rounded-sm px-2 text-xs text-muted outline-none hover:bg-hover hover:text-fg focus-visible:bg-hover"
            >
              {`and ${changed.length - biggest.length} more`}
            </button>
          )}
          <MenuRule />
        </>
      )}
      {/* The card opens above the branch, so its actions sit at its foot, nearest the pointer. */}
      <div className="flex flex-col">
        <CardAction icon={<FileDiff size={14} />} onClick={(e) => openTab('diff', howOf(e))}>
          Open changes
        </CardAction>
        <CardAction
          icon={copied ? <Check size={14} className="text-success" /> : <Copy size={14} />}
          onClick={() =>
            void navigator.clipboard.writeText(branch).then(
              () => {
                setCopied(true);
                setTimeout(() => setCopied(false), 1500);
              },
              () => toast('Copy did not work.', { tone: 'warning' }),
            )
          }
        >
          {copied ? 'Copied' : 'Copy branch name'}
        </CardAction>
      </div>
    </HoverCard>
  );
}
