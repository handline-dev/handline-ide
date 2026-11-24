import { count } from '#src/format/count.ts';
// A task's code: its commits with All changes first, and the files of what is picked, each opening its diff in the look of the agent's
// Changes. In a tab of its own (code:<id>, CodeTab) and in a story sub-task's Code tab; the task's change size opens the tab (ChangeSize).
import { Hooks } from '@handline/app-data';
import { useState } from 'react';
import { byFile, File } from '#src/changes/Changes.tsx';
import { codeOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { Ago } from '#src/ui/Ago.tsx';
import { Name } from '#src/ui/Face.tsx';
import { LinesLoading } from '#src/ui/Loading.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';

/** Files listed before the rest fold into "Show N more files". */
const FIRST = 20;

type Changes = NonNullable<ReturnType<typeof Hooks.useTaskChanges>['data']>;

const sum = (rows: { added: number; deleted: number }[]) => ({
  added: rows.reduce((n, r) => n + r.added, 0),
  deleted: rows.reduce((n, r) => n + r.deleted, 0),
});

function Lines({ added, deleted }: { added: number; deleted: number }) {
  return (
    <span className="shrink-0 text-xs tabular-nums">
      <span className="text-success">+{added}</span> <span className="text-danger">−{deleted}</span>
    </span>
  );
}

/** The task's change size, "+280 −72 · 26 files", which opens its Code tab by the app's one scheme; nothing while it has no commits. */
export function ChangeSize({ task }: { task: number }) {
  const all = Hooks.useTaskChanges({ task }).data;
  const open = useOpenable(codeOf(task));
  if (!all?.commits.length) return null;
  const { added, deleted } = sum(all.files);
  return (
    <button
      type="button"
      {...open}
      title="Open its code"
      className="self-start rounded-sm text-sm text-muted hover:bg-hover/30 hover:text-fg"
    >
      <span className="text-success">+{added}</span> <span className="text-danger">−{deleted}</span> · {count(all.files.length, 'file')}
    </button>
  );
}

/** The Code tab of its own, "T<n> Code". */
export function CodeTab({ id }: { id: number }) {
  return (
    <ScrollFade keep={`code:${id}`} className="min-h-0 flex-1">
      <TaskCode task={id} />
    </ScrollFade>
  );
}

/** Commits on top, the picked one's files under them. */
export function TaskCode({ task }: { task: number }) {
  const [commit, pick] = useState<string | null>(null);
  const all = Hooks.useTaskChanges({ task });
  const one = Hooks.useTaskChanges({ task, commit: commit ?? undefined }, { enabled: !!commit });
  if (all.isPending) return <LinesLoading label="Loading the changes" />;
  if (all.isError || !all.data) return <p className="p-4 text-sm text-danger">The changes did not load.</p>;
  const commits = all.data.commits;
  if (!commits.length) return <p className="p-4 text-sm text-muted">No commits on this task yet.</p>;
  const shown = commit ? one.data : all.data;
  const whole = sum(all.data.files);
  const row = (on: boolean) => `flex w-full items-center gap-3 px-2 py-1.5 text-left text-sm ${on ? 'bg-selected' : 'hover:bg-hover/30'}`;
  return (
    <div className="flex flex-col gap-4 p-4">
      <section className="flex flex-col">
        <h3 className="px-2 pb-1 text-xs font-medium text-muted">Commits</h3>
        <button type="button" aria-pressed={!commit} onClick={() => pick(null)} className={`${row(!commit)} rounded-md`}>
          <span className="font-medium text-fg">All changes</span>
          <span className="min-w-0 flex-1 truncate text-muted">
            {count(commits.length, 'commit')} · {count(all.data.files.length, 'file')}
          </span>
          <Lines {...whole} />
        </button>
        {commits.map((c) => (
          <button
            key={c.sha}
            type="button"
            aria-pressed={commit === c.sha}
            onClick={() => pick(c.sha)}
            className={`${row(commit === c.sha)} border-t border-border`}
          >
            <span className="shrink-0 font-mono text-xs text-muted">{c.sha.slice(0, 7)}</span>
            <span className="min-w-0 flex-1 truncate text-fg">{c.subject}</span>
            {c.agent && <Name who={c.agent} className="shrink-0 text-muted" />}
            <span className="shrink-0 text-xs text-muted">
              <Ago at={c.at} />
            </span>
            <span className={`shrink-0 text-xs ${c.on_main ? 'text-muted' : 'text-warning'}`}>
              {c.on_main ? 'on main' : 'not on main yet'}
            </span>
            <Lines added={c.added} deleted={c.deleted} />
          </button>
        ))}
      </section>
      {shown ? <Files key={commit ?? 'all'} changes={shown} /> : <LinesLoading label="Loading the commit" />}
    </div>
  );
}

/** The files of what is picked, each folded until clicked; past the first ones, "Show N more files". */
function Files({ changes }: { changes: Changes }) {
  const [opened, setOpened] = useState<string[]>([]);
  const [every, setEvery] = useState(false);
  const diffs = new Map(byFile(changes.diff).map((f) => [f.path, f]));
  const files = every ? changes.files : changes.files.slice(0, FIRST);
  return (
    <section className="flex flex-col gap-2">
      <h3 className="px-2 text-xs font-medium text-muted">Files</h3>
      {files.map((f) => (
        <File
          key={`${f.repo}/${f.path}`}
          file={diffs.get(f.path) ?? { path: f.path, lines: [f.binary ? 'A binary file.' : 'Left out: the diff is too long to show.'] }}
          added={f.added}
          deleted={f.deleted}
          open={opened.includes(f.path)}
          setOpen={(o) => setOpened((s) => (o ? [...s, f.path] : s.filter((p) => p !== f.path)))}
        />
      ))}
      {changes.files.length > FIRST && !every && (
        <button type="button" onClick={() => setEvery(true)} className="self-start px-2 text-xs text-muted hover:text-fg">
          Show {count(changes.files.length - FIRST, 'more file')}
        </button>
      )}
    </section>
  );
}
