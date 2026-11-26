import { Segmented } from '#src/ui/Segmented.tsx';
import { LinesLoading } from '#src/ui/Loading.tsx';
import { useAgentDetail, useRead, useWorktrees } from '@handline/app-data';
import { ChevronDown, ChevronRight, GitBranch } from 'lucide-react';
import { useCallback, useRef } from 'react';
import { useFocused } from '#src/layout/focus.ts';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { useTabState } from '#src/persist/tabState.ts';

/** One file's part of a unified diff: its path and its lines, the file headers left out. */
export type FileDiff = { path: string; lines: string[] };

/** Splits a unified diff (`git diff`) by file. */
export function byFile(diff: string): FileDiff[] {
  const files: FileDiff[] = [];
  for (const line of diff.split('\n')) {
    const head = /^diff --git a\/.+ b\/(.+)$/.exec(line);
    if (head) files.push({ path: head[1], lines: [] });
    else if (files.length && !/^(index |--- |\+\+\+ |new file|deleted file|similarity|rename |old mode|new mode)/.test(line))
      files.at(-1)!.lines.push(line);
  }
  return files;
}

const tone = (line: string) =>
  line.startsWith('@@')
    ? 'bg-surface text-muted'
    : line.startsWith('+')
      ? 'bg-success/10 text-fg'
      : line.startsWith('-')
        ? 'bg-danger/10 text-fg'
        : 'text-fg';

/** The agent's worktree changes against its base: the branch, each changed file with its lines added and removed, and the diff
 * under it, folded per file. */
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
  const box = useRef<HTMLDivElement>(null);
  // A file asked for from the branch card comes to the top of the view, opened.
  const show = useCallback((file: string) => {
    const el = [...(box.current?.querySelectorAll('[data-file]') ?? [])].find((n) => n.getAttribute('data-file') === file);
    el?.dispatchEvent(new CustomEvent('reveal'));
    el?.scrollIntoView({ block: 'start' });
  }, []);
  useFocused('diff', agent, !!diff.data, show);
  if (!detail) return <p className="p-4 text-sm text-muted">Loading…</p>;
  if (!detail.worktree) return <p className="p-4 text-sm text-muted">This agent has no worktree.</p>;
  const files = diff.data ? byFile(diff.data.diff) : [];
  const counts = new Map(worktreeChanged.map((f) => [f.path, f]));
  return (
    <ScrollFade keep={`changes:${agent}`} className="min-h-0 flex-1">
      <div ref={box} className="flex flex-col gap-3 p-4">
        <div className="flex items-center gap-2 text-sm text-muted">
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

export function File({
  file,
  added,
  deleted,
  open,
  setOpen,
}: {
  file: FileDiff;
  added?: number;
  deleted?: number;
  open: boolean;
  setOpen: (open: boolean) => void;
}) {
  return (
    <section
      data-file={file.path}
      ref={(el) => {
        // A reveal (the file asked for from elsewhere) opens it if it was folded.
        const reveal = () => setOpen(true);
        el?.addEventListener('reveal', reveal);
        return () => el?.removeEventListener('reveal', reveal);
      }}
      className="scroll-mt-4 overflow-hidden rounded-md border border-border"
    >
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-2 bg-surface px-3 py-1.5 text-left text-sm outline-none focus-visible:ring-2 focus-visible:ring-accent"
      >
        {open ? <ChevronDown size={14} className="text-muted" /> : <ChevronRight size={14} className="text-muted" />}
        <span className="min-w-0 flex-1 truncate font-mono text-fg">{file.path}</span>
        {added != null && <span className="text-xs text-success tabular-nums">+{added}</span>}
        {deleted != null && <span className="text-xs text-danger tabular-nums">−{deleted}</span>}
      </button>
      {open && (
        <pre className="scrollbar-auto overflow-x-auto font-mono text-xs leading-5">
          {file.lines.map((l, i) => (
            <div key={i} className={`px-3 whitespace-pre ${tone(l)}`}>
              {l || ' '}
            </div>
          ))}
        </pre>
      )}
    </section>
  );
}
