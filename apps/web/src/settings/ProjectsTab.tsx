import { baseName } from '#src/format/path.ts';
// Settings > Projects: the git folders agents work in, each with its worktree setup, and the ones found on this Mac. A project is listed once an agent works in it, so every way to add one starts an agent.
import { Hooks, useRepos, useWrite, why } from '@handline/app-data';
import { FolderGit2, Link, Plus } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { NeedsGit, useMissing } from '#src/install/install.tsx';
import { workspaceNameOf } from '#src/onboarding/firstTask.ts';
import { Block } from '#src/settings/Meter.tsx';
import { ProjectWorktrees } from '#src/settings/ProjectWorktrees.tsx';
import { ProjectSetup, setupSummary } from '#src/settings/ProjectSetup.tsx';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { shell } from '#src/shell.ts';
import { Button } from '#src/ui/Button.tsx';
import { Field, Input } from '#src/ui/Field.tsx';
import { toast } from '#src/ui/toast.tsx';

/** Makes a workspace on `path` and starts its lead there, which is what makes it a project. */
function useStartIn() {
  const create = useWrite('workspace.create');
  return {
    pending: create.isPending,
    error: create.error,
    start: (path: string) =>
      create.mutate(
        { name: workspaceNameOf(baseName(path)), folder: path },
        { onSuccess: () => toast(`${baseName(path)} is added: an agent is starting in it.`) },
      ),
  };
}

export function ProjectsTab() {
  const repos = useRepos();
  const recent = Hooks.useReposRecent();
  const open = repos.data?.repos ?? [];
  const found = (recent.data?.folders ?? []).filter((f) => !open.some((r) => r.path === f.path));
  const here = useStartIn();
  const noGit = useMissing('git') !== null;
  const [mode, setMode] = useState<'folder' | 'clone' | 'new' | null>(null);
  const [editing, setEditing] = useState<string | null>(null);
  return (
    <SettingsPage
      keep="settings-projects"
      title="Projects"
    >
      <Block title="Your projects" note={open.length ? undefined : 'None yet.'}>
        {open.map((r) => (
          <div key={r.path} data-repo={r.path} className="flex flex-col gap-2">
            <div className="flex items-center gap-3 text-sm">
              <FolderGit2 size={14} className="shrink-0 text-muted" />
              <span className="font-medium text-fg">{r.name}</span>
              <span className="min-w-0 flex-1 truncate text-xs text-muted">{r.path}</span>
              <span className={`text-xs ${r.setup.last && !r.setup.last.ok ? 'text-danger' : 'text-muted'}`}>{setupSummary(r.setup)}</span>
              <Button
                size="sm"
                aria-label={`Setup for ${r.name}`}
                aria-expanded={editing === r.path}
                onClick={() => setEditing(editing === r.path ? null : r.path)}
              >
                Setup…
              </Button>
            </div>
            {editing === r.path && (
              <>
                <ProjectSetup repo={r} close={() => setEditing(null)} />
                <ProjectWorktrees repo={r} />
              </>
            )}
          </div>
        ))}
      </Block>
      {found.length > 0 && !noGit && (
        <Block title="Found on this Mac" note="Git folders near your home. Add one to start an agent in it.">
          {found.map((f) => (
            <div key={f.path} data-found={f.path} className="flex items-center gap-3 text-sm">
              <FolderGit2 size={14} className="shrink-0 text-muted" />
              <span className="font-medium text-fg">{f.name}</span>
              <span className="min-w-0 flex-1 truncate text-xs text-muted">{f.path}</span>
              <Button size="sm" disabled={here.pending} onClick={() => here.start(f.path)}>
                Add
              </Button>
            </div>
          ))}
        </Block>
      )}
      <Block title="Other ways to add">
        <NeedsGit>
          <div className="flex flex-wrap gap-2">
            <Button aria-expanded={mode === 'folder'} onClick={() => setMode('folder')}>
              <FolderGit2 size={14} />
              Browse folder…
            </Button>
            <Button aria-expanded={mode === 'clone'} onClick={() => setMode('clone')}>
              <Link size={14} />
              Clone from URL…
            </Button>
            <Button aria-expanded={mode === 'new'} onClick={() => setMode('new')}>
              <Plus size={14} />
              Create new…
            </Button>
          </div>
          {mode === 'folder' && <FolderForm key="folder" />}
          {mode === 'new' && <FolderForm key="new" init />}
          {mode === 'clone' && <CloneForm />}
        </NeedsGit>
      </Block>
      {!!here.error && (
        <p className="text-sm text-danger" role="alert">
          {why(here.error)}
        </p>
      )}
    </SettingsPage>
  );
}

function Form({
  children,
  onSubmit,
  busy,
  word,
  error,
}: {
  children: ReactNode;
  onSubmit: () => void;
  busy: boolean;
  word: string;
  error: unknown;
}) {
  return (
    <form
      className="flex flex-col gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        onSubmit();
      }}
    >
      {children}
      <div className="flex items-center gap-3">
        <Button type="submit" variant="primary" disabled={busy}>
          {busy ? 'Working…' : word}
        </Button>
        {!!error && (
          <span className="text-xs text-danger" role="alert">
            {why(error)}
          </span>
        )}
      </div>
    </form>
  );
}

/** A folder on this Mac, chosen or typed: used as it is, or with `init` made a git repository first (a first commit of what it holds). */
function FolderForm({ init }: { init?: boolean }) {
  const [path, setPath] = useState('');
  const make = useWrite('folder.init');
  const here = useStartIn();
  const pick = shell()?.pickFolder;
  const go = async () => {
    const p = path.trim();
    if (!p) return;
    if (init) {
      try {
        await make.mutateAsync({ path: p });
      } catch {
        return; // its refusal is shown under the button
      }
    }
    here.start(p);
  };
  return (
    <Form
      onSubmit={() => void go()}
      busy={make.isPending || here.pending || !path.trim()}
      word={init ? 'Start git and add' : 'Add'}
      error={make.error ?? here.error}
    >
      <div className="flex items-end gap-2">
        <Field
          label={init ? 'Folder to start from' : 'Folder'}
        >
          <Input value={path} placeholder="/path/to/project" onChange={(e) => setPath(e.target.value)} />
        </Field>
        {pick && (
          <Button
            onClick={() =>
              void pick().then(
                (r) => r.ok && r.path && setPath(r.path),
                () => {},
              )
            }
          >
            Choose…
          </Button>
        )}
      </div>
    </Form>
  );
}

function CloneForm() {
  const [url, setUrl] = useState('');
  const [path, setPath] = useState('');
  const clone = useWrite('folder.clone');
  const here = useStartIn();
  const submit = () =>
    clone.mutate(
      { url: url.trim(), ...(path.trim() ? { path: path.trim() } : {}) },
      { onSuccess: (r) => here.start((r as { path: string }).path) },
    );
  return (
    <Form onSubmit={submit} busy={clone.isPending || here.pending || !url.trim()} word="Clone and add" error={clone.error ?? here.error}>
      <Field label="Repository address">
        <Input value={url} placeholder="https://github.com/you/project.git" onChange={(e) => setUrl(e.target.value)} />
      </Field>
      <Field label="Into folder (optional)">
        <Input value={path} placeholder="/path/to/new/folder" onChange={(e) => setPath(e.target.value)} />
      </Field>
    </Form>
  );
}
