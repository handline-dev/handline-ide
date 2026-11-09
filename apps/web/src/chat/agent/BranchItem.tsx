import { useWorktrees } from '@handline/app-data';
import { Check, Copy, FileDiff, GitBranch } from 'lucide-react';
import { CardAction, chip, MenuHeading, MenuRule } from '#src/chat/agent/menuParts.tsx';
import { focusIn } from '#src/layout/focus.ts';
import { useOpenable } from '#src/layout/openable.tsx';
import type { ReactNode } from 'react';
import { HoverCard } from '#src/ui/HoverCard.tsx';
import { useCopy } from '#src/ui/copy.ts';

/** The branch: resting on it shows its changed files and copies its name; a click opens the Changes tab with the diff. */
export function BranchItem({ agent, branch, path }: { agent: string; branch: string; path: string }) {
  const changed = useWorktrees({ owner: agent }).data?.worktrees.find((w) => w.path === path)?.changed ?? [];
  const diff = useOpenable('diff');
  // The copy button says it took, in place, for a moment: the card stays open under the pointer, so it is seen there.
  const { copied, copy } = useCopy();
  const added = changed.reduce((n, f) => n + f.added, 0);
  const deleted = changed.reduce((n, f) => n + f.deleted, 0);
  const biggest = [...changed].sort((a, b) => b.added + b.deleted - (a.added + a.deleted)).slice(0, 5);
  const rest = changed.find((f) => !biggest.includes(f));
  return (
    <HoverCard
      trigger={
        <button type="button" {...diff} className={chip}>
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
            <DiffAt key={f.path} agent={agent} path={f.path}>
              <span className="truncate text-fg">{f.path}</span>
              <span className="shrink-0 tabular-nums">
                <span className="text-success">+{f.added}</span> <span className="text-danger">−{f.deleted}</span>
              </span>
            </DiffAt>
          ))}
          {rest && (
            // The rest: the Changes tab, at the first file not listed here.
            <DiffAt agent={agent} path={rest.path} more>
              {`and ${changed.length - biggest.length} more`}
            </DiffAt>
          )}
          <MenuRule />
        </>
      )}
      {/* The card opens above the branch, so its actions sit at its foot, nearest the pointer. */}
      <div className="flex flex-col">
        <CardAction icon={<FileDiff size={14} />} {...diff}>
          Open changes
        </CardAction>
        <CardAction
          icon={copied ? <Check size={14} className="text-success" /> : <Copy size={14} />}
          onClick={() => copy(branch, 'the branch name')}
        >
          {copied ? 'Copied' : 'Copy branch name'}
        </CardAction>
      </div>
    </HoverCard>
  );
}

/** A line of the card that opens the Changes tab at one file. */
function DiffAt({ agent, path, more, children }: { agent: string; path: string; more?: boolean; children: ReactNode }) {
  return (
    <button
      type="button"
      {...useOpenable('diff', { focus: () => focusIn('diff', agent, path) })}
      className={`flex h-6 w-full items-center gap-6 rounded-sm px-2 text-xs outline-none hover:bg-hover focus-visible:bg-hover ${
        more ? 'text-muted hover:text-fg' : 'justify-between'
      }`}
    >
      {children}
    </button>
  );
}
