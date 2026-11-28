import { baseName } from '#src/format/path.ts';
import { useRead, useWrite, why } from '@handline/app-data';
import { Code, FolderOpen, GitBranch, Megaphone, MessageCircle, PenLine, Plus, Search } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { OperationProgress } from '#src/ui/OperationProgress.tsx';
import { Field, Input } from '#src/ui/Field.tsx';
import { ModalFrame, openModal, type Close } from '#src/ui/modal.tsx';
import { shell } from '#src/shell.ts';
import { workspaceNameOf } from '#src/onboarding/firstTask.ts';
import { NeedsGit } from '#src/install/install.tsx';
import { toast } from '#src/ui/toast.tsx';
import { createParams, freeName, STARTERS, takenNames, type Word } from '#src/newTeam.ts';

/** A folder's workspace name, from its last part, as onboarding names it. */
export const nameOf = (folder: string) => workspaceNameOf(baseName(folder));

/** Where no folder picker is (a browser): the folder typed. */
function Typed({ go, close }: { go: (folder: string) => void; close: Close<string> }) {
  const [folder, setFolder] = useState('');
  const ready = !!folder.trim();
  return (
    <form
      className="flex flex-col gap-3 pb-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (ready) go(folder.trim());
      }}
    >
      <Field label="Folder">
        <Input autoFocus value={folder} onChange={(e) => setFolder(e.target.value)} placeholder="/path/to/folder" />
      </Field>
      <div className="flex justify-end gap-2 pt-1">
        <Button onClick={() => close()}>Cancel</Button>
        <Button type="submit" variant="primary" disabled={!ready}>
          Create
        </Button>
      </div>
    </form>
  );
}

/** Clone: the repository's address; the daemon clones it under the Handline folder, and the team is made on it. */
function Clone({ go, close }: { go: (folder: string) => void; close: Close<string> }) {
  const clone = useWrite('folder.clone');
  const [url, setUrl] = useState('');
  const ready = !!url.trim() && !clone.isPending;
  return (
    <form
      className="flex flex-col gap-3 pb-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (ready) clone.mutate({ url: url.trim() }, { onSuccess: (r) => go((r as { path: string }).path) });
      }}
    >
      <Field label="Repository address">
        <Input autoFocus value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://github.com/you/project.git" />
      </Field>
      {!!clone.error && <p className="text-sm text-danger">{why(clone.error)}</p>}
      <div className="flex justify-end gap-2 pt-1">
        <Button onClick={() => close()}>Cancel</Button>
        <Button type="submit" variant="primary" disabled={!ready}>
          {clone.isPending ? 'Cloning…' : 'Clone'}
        </Button>
      </div>
    </form>
  );
}

const ICONS: Record<Word, ReactNode> = {
  Empty: <Plus size={26} />,
  Coding: <Code size={26} />,
  Writing: <PenLine size={26} />,
  Research: <Search size={26} />,
  Support: <MessageCircle size={26} />,
  Marketing: <Megaphone size={26} />,
};

/** A way to bring in a project, as a row: its icon, then its words. */
function Way({ icon, word, onClick }: { icon: ReactNode; word: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex h-10 items-center justify-center gap-2 rounded-lg border border-border text-sm hover:bg-hover/30"
    >
      <span className="text-muted">{icon}</span>
      {word}
    </button>
  );
}

type Step = 'choose' | 'type' | 'clone' | { name: string; operation: string };

/** A new team: a tile makes one at once, named after it; or a project brought in, named after its folder. Nothing is asked. */
function Start({ close }: { close: Close<string> }) {
  const [step, setStep] = useState<Step>('choose');
  const workspaces = useRead('workspaces.list', {}).data?.workspaces;
  const agents = useRead('agents.list', {}).data?.agents;
  const people = useRead('people.list', {}).data?.people;
  const create = useWrite('workspace.create');
  // A name is picked once every name it must not take is known.
  const ready = !!(workspaces && agents && people) && !create.isPending;
  /** Makes the team `base` names (on `folder`, else in a new folder the daemon makes, its lead given `role`) at once. */
  const make = (base: string, how: { folder?: string; role?: string } = {}) => {
    if (!workspaces || !agents || !people || create.isPending) return;
    const name = freeName(base, takenNames(workspaces, agents, people));
    create.mutate(createParams(name, how), {
      onSuccess: (r) => setStep({ name, operation: (r as { operation: string }).operation }),
      onError: (e) => toast(why(e), { tone: 'warning' }),
    });
  };
  const choose = shell()?.pickFolder;
  const fromFolder = (folder: string) => make(nameOf(folder), { folder });
  if (step === 'type') return <Typed go={fromFolder} close={close} />;
  if (step === 'clone')
    return (
      <NeedsGit>
        <Clone go={fromFolder} close={close} />
      </NeedsGit>
    );
  if (typeof step === 'object')
    return (
      <OperationProgress operation={step.operation} failed="The team was not made." onDone={() => close(step.name)} close={() => close()} />
    );
  const open = () =>
    choose
      ? void choose().then(
          (r) => r.ok && r.path && fromFolder(r.path),
          () => undefined,
        )
      : setStep('type');
  return (
    <div className="flex flex-col gap-3 pb-4">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
        {STARTERS.map((s) => (
          <button
            key={s.word}
            type="button"
            disabled={!ready}
            onClick={() => make(s.name, { role: s.role })}
            className="flex h-24 flex-col items-center justify-center gap-2 rounded-lg border border-border text-sm hover:bg-hover/30 disabled:opacity-50"
          >
            <span className="text-muted">{ICONS[s.word]}</span>
            {s.word}
          </button>
        ))}
      </div>
      <div className="flex items-center gap-3 py-1 text-xs text-muted">
        <span className="h-px flex-1 bg-border" />
        or
        <span className="h-px flex-1 bg-border" />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <Way icon={<FolderOpen size={16} />} word="Open a folder" onClick={open} />
        <Way icon={<GitBranch size={16} />} word="Clone a repository" onClick={() => setStep('clone')} />
      </div>
    </div>
  );
}

/** A new team, from the sidebar or the town (`title` says it in the words of each); resolves with its name once it is made. */
export function newWorkspace(title = 'Add workspace'): Promise<string | undefined> {
  return openModal<string>((close) => (
    <ModalFrame title={title} onClose={() => close()}>
      <Start close={close} />
    </ModalFrame>
  ));
}
