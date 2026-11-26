import { Segmented } from '#src/ui/Segmented.tsx';
import { LinesLoading } from '#src/ui/Loading.tsx';
import { useAgentDetail, useRead, useWorktrees } from '@handline/app-data';
import { diffFiles } from '@handline/app-core';
import { GitBranch } from 'lucide-react';
import { useCallback, useRef } from 'react';
import { useFocused } from '#src/layout/focus.ts';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { useTabState } from '#src/persist/tabState.ts';
import { ChatCommentBar } from '#src/changes/CommentBar.tsx';
import { Elsewhere, File } from '#src/changes/DiffFile.tsx';
import { agentPlace, useLineDrafts, useMovedMarked } from '#src/changes/lineDrafts.ts';

/** The agent's worktree changes against its base: the branch, each changed file with its lines added and removed, and the diff
 * under it, folded per file. A line or a file takes a comment; the comments wait as drafts and go to the agent's chat as one message. */
const AGAINST = ['head', 'base'] as const;

export function Changes({ agent }: { agent: string }) {
  const detail = useAgentDetail(agent).data;
  const path = detail?.worktree?.path ?? '';
  const row = useWorktrees({ owner: agent }).data?.worktrees.find((w) => w.path === path);
  const worktreeChanged = row?.changed ?? [];
  // The branch the worktree merges into, as its row names it: main in most repos, master or another where the repo says so.
  const base = row?.base || 'main';
  const [against, setAgainst] = useTabState<'head' | 'base'>('against', 'base');
  // The files folded shut; the rest are open. A fold drops the paths no longer in the diff, so the list never outgrows it.
  const [shut, setShut] = useTabState<string[]>('shut', []);
  const diff = useRead('worktree.diff', { path, against }, { enabled: path !== '' });
  const drafts = useLineDrafts(agentPlace(agent));
  const box = useRef<HTMLDivElement>(null);
  // A file asked for from the branch card comes to the top of the view, opened.
  const show = useCallback((file: string) => {
    const el = [...(box.current?.querySelectorAll('[data-file]') ?? [])].find((n) => n.getAttribute('data-file') === file);
    el?.dispatchEvent(new CustomEvent('reveal'));
    el?.scrollIntoView({ block: 'start' });
  }, []);
  useFocused('diff', agent, !!diff.data, show);
  const files = diff.data ? diffFiles(diff.data.diff, diff.data.truncated) : [];
  // The diff served says what it was taken against: with no base to measure from it is the uncommitted work.
  const commenting = { drafts, view: diff.data?.against ?? against };
  // The worktree changes under the drafts: one whose line is not there as written any more is marked, and sent on its file.
  useMovedMarked(drafts, commenting.view, diff.data ? files : null);
  if (!detail) return <p className="p-4 text-sm text-muted">Loading…</p>;
  if (!detail.worktree) return <p className="p-4 text-sm text-muted">This agent has no worktree.</p>;
  const counts = new Map(worktreeChanged.map((f) => [f.path, f]));
  return (
    <ScrollFade keep={`changes:${agent}`} fadeTop={!drafts.list.length} className="min-h-0 flex-1">
      <div ref={box} className="flex flex-col gap-3 p-4">
        <ChatCommentBar agent={agent} />
        {diff.data && <Elsewhere commenting={commenting} paths={files.map((f) => f.path)} />}
        <div className="flex flex-wrap items-center gap-2 text-sm text-muted">
          <GitBranch size={14} />
          <span className="text-fg">{detail.worktree.branch}</span>
          <span>{`${worktreeChanged.length} files changed`}</span>
          <span className="flex-1" />
          <Segmented
            label="Compare"
            value={against}
            onChange={setAgainst}
            options={AGAINST}
            word={(v) => (v === 'head' ? 'Uncommitted' : `vs ${diff.data?.against === 'base' ? (diff.data.base_ref ?? base) : base}`)}
          />
        </div>
        {diff.isPending && <LinesLoading label="Loading the diff" />}
        {diff.isError && <p className="text-sm text-danger">The diff did not load.</p>}
        {diff.data && !files.length && <p className="text-sm text-muted">No changes.</p>}
        {files.map((f) => (
          <File
            key={f.path}
            file={f}
            added={counts.get(f.path)?.added}
            deleted={counts.get(f.path)?.deleted}
            commenting={commenting}
            open={!shut.includes(f.path)}
            setOpen={(open) =>
              setShut((s) => [...s.filter((p) => p !== f.path && files.some((x) => x.path === p)), ...(open ? [] : [f.path])])
            }
          />
        ))}
        {diff.data?.truncated && <p className="text-xs text-warning">The diff is longer than this; the rest is cut off.</p>}
      </div>
    </ScrollFade>
  );
}
