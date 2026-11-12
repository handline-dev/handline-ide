// Settings > Projects: the git folders agents work in and the ones found on this Mac. A project is listed once an agent works in it, so every way to add one starts an agent.
import { classify, hooks, sentence, useRepos, useWrite } from '@handline/app-data';
import { FolderGit2, Link, Plus } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { agentNameOf } from '#src/onboarding/firstTask.ts';
import { Block } from '#src/settings/Meter.tsx';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { shell } from '#src/shell.ts';
import { Button } from '#src/ui/Button.tsx';
import { Field, Input } from '#src/ui/Field.tsx';
import { toast } from '#src/ui/toast.tsx';

const nameOf = (p: string) => p.replace(/\/+$/, '').split('/').pop() ?? p;

/** Starts an agent in `path`, which is what makes it a project. */
function useStartIn() {
  const create = useWrite('agent.create');
  return {
    pending: create.isPending,
    error: create.error,
    start: (path: string) =>
      create.mutate(
        { name: agentNameOf(nameOf(path)), role: 'developer', repo: path },
        { onSuccess: () => toast(`${nameOf(path)} is added: an agent is starting in it.`) },
      ),
  };
}

export function ProjectsTab() {
  const repos = useRepos();
  const recent = hooks.useReposRecent();
  const open = repos.data?.repos ?? [];
  const found = (recent.data?.folders ?? []).filter((f) => !open.some((r) => r.path === f.path));
  const here = useStartIn();
  const [mode, setMode] = useState<'folder' | 'clone' | 'new' | null>(null);
  return (
    <SettingsPage
      keep="settings-projects"
      title="Projects"
      note="What your agents work on. Each task gets its own worktree, so your checkout stays as you left it."
    >
      <Block title="Your projects" note={open.length ? undefined : 'None yet.'}>
        {open.map((r) => (
          <div key={r.path} data-repo={r.path} className="flex items-center gap-3 text-sm">
            <FolderGit2 size={14} className="shrink-0 text-muted" />
            <span className="font-medium text-fg">{r.name}</span>
            <span className="min-w-0 flex-1 truncate text-xs text-muted">{r.path}</span>
            <span className="text-xs text-muted">{r.default_branch}</span>
          </div>
        ))}
      </Block>
      {found.length > 0 && (
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
      </Block>
      {!!here.error && (
        <p className="text-sm text-danger" role="alert">
          {sentence(classify(here.error))}
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
            {sentence(classify(error))}
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
          hint={init ? 'An empty or existing folder that is not a git repository yet.' : undefined}
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
      <Field label="Repository address" hint="https, ssh or git. A repository that asks for a password here is refused.">
        <Input value={url} placeholder="https://github.com/you/project.git" onChange={(e) => setUrl(e.target.value)} />
      </Field>
      <Field label="Into folder (optional)" hint="Left empty, it goes under your Handline folder.">
        <Input value={path} placeholder="/path/to/new/folder" onChange={(e) => setPath(e.target.value)} />
      </Field>
    </Form>
  );
}
