// First run: with no agent yet the app opens on this full-screen flow: Get ready, the folder to work in, what to do first. The agent is
// made from the folder (named for it, no role or "Reports to" asked) and its first task goes with it.
import { classify, hooks, sentence, useAgents, useCreateProgress, useWrite } from '@handline/app-data';
import { FolderGit2, LoaderCircle } from 'lucide-react';
import { useState } from 'react';
import { GetReady } from '#src/firstrun/GetReady.tsx';
import { Button } from '#src/ui/Button.tsx';
import { Field, Input } from '#src/ui/Field.tsx';

/** The first-run flow replaces the app until an agent exists. */
export function FirstRunGate({ children }: { children: React.ReactNode }) {
  const agents = useAgents();
  // Once the flow opens it stays until the first task is sent: the new agent shows in the list before that.
  const [active, setActive] = useState(false);
  const empty = agents.isSuccess && !agents.data.agents.length;
  if (empty && !active) setActive(true);
  if (active) return <FirstRun onStarted={() => setActive(false)} />;
  return <>{children}</>;
}

type Folder = { path: string; name: string };
/** The agent's name from its folder's: lower case, anything else a dash. */
export const agentNameOf = (folder: string): string =>
  folder
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '') || 'agent';
const LOOK_AROUND = 'Look around this project and propose 3 tasks worth doing next. Read only: change nothing.';

function FirstRun({ onStarted }: { onStarted: () => void }) {
  const [step, setStep] = useState<'ready' | 'repo' | 'first'>('ready');
  const [folder, setFolder] = useState<Folder | null>(null);
  return (
    <div className="fixed inset-0 z-50 overflow-y-auto bg-bg">
      <div className="mx-auto flex max-w-xl flex-col gap-6 px-6 py-16">
        {step === 'ready' && <GetReady onContinue={() => setStep('repo')} />}
        {step === 'repo' && (
          <ChooseFolder
            onPick={(f) => {
              setFolder(f);
              setStep('first');
            }}
          />
        )}
        {step === 'first' && folder && <WhatFirst folder={folder} onBack={() => setStep('repo')} onStarted={onStarted} />}
      </div>
    </div>
  );
}

function ChooseFolder({ onPick }: { onPick: (f: Folder) => void }) {
  const recent = hooks.useReposRecent();
  const folders = recent.data?.folders ?? [];
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-xl font-medium text-fg">Where should the agent work?</h1>
      {recent.isPending && <LoaderCircle size={16} className="animate-spin text-muted" />}
      <div className="flex flex-col">
        {folders.map((f) => (
          <button
            key={f.path}
            type="button"
            onClick={() => onPick(f)}
            className="flex items-center gap-3 border-b border-border py-2.5 text-left hover:bg-hover/30"
          >
            <FolderGit2 size={16} className="shrink-0 text-muted" />
            <span className="text-sm font-medium text-fg">{f.name}</span>
            <span className="min-w-0 flex-1 truncate text-xs text-muted">{f.path}</span>
          </button>
        ))}
      </div>
      {!recent.isPending && !folders.length && <p className="text-sm text-muted">No git folders found nearby.</p>}
      <ChooseAnother onPick={onPick} />
    </div>
  );
}

/** A folder path typed in. */
function ChooseAnother({ onPick }: { onPick: (f: Folder) => void }) {
  const [path, setPath] = useState('');
  const name = (p: string) => p.replace(/\/+$/, '').split('/').pop() ?? p;
  return (
    <form
      className="flex items-end gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        if (path.trim()) onPick({ path: path.trim(), name: name(path.trim()) });
      }}
    >
      <Field label="Another folder">
        <Input value={path} placeholder="/path/to/project" onChange={(e) => setPath(e.target.value)} />
      </Field>
      <Button type="submit" disabled={!path.trim()}>
        Use it
      </Button>
    </form>
  );
}

function WhatFirst({ folder, onBack, onStarted }: { folder: Folder; onBack: () => void; onStarted: () => void }) {
  const create = useWrite('agent.create');
  const [own, setOwn] = useState('');
  const [task, setTask] = useState(LOOK_AROUND);
  const [operation, setOperation] = useState<string | null>(null);
  const start = (text: string) => {
    setTask(text);
    create.mutate(
      {
        name: agentNameOf(folder.name),
        role: 'developer',
        repo: folder.path,
      },
      { onSuccess: (r) => setOperation((r as { operation: string }).operation) },
    );
  };
  if (operation) return <Starting operation={operation} task={task} onStarted={onStarted} />;
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-xl font-medium text-fg">What first, in {folder.name}?</h1>
      <button
        type="button"
        disabled={create.isPending}
        onClick={() => start(LOOK_AROUND)}
        className="rounded-md border border-border p-3 text-left hover:bg-hover/30 disabled:opacity-60"
      >
        <div className="text-sm font-medium text-fg">Look around and propose 3 tasks</div>
        <div className="text-xs text-muted">Reads only; nothing is changed.</div>
      </button>
      <form
        className="flex items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (own.trim()) start(own.trim());
        }}
      >
        <Field label="Or write it">
          <Input value={own} onChange={(e) => setOwn(e.target.value)} />
        </Field>
        <Button type="submit" variant="primary" disabled={!own.trim() || create.isPending}>
          Start
        </Button>
      </form>
      {!!create.error && <p className="text-sm text-danger">{sentence(classify(create.error))}</p>}
      <Button variant="ghost" className="w-fit" onClick={onBack}>
        Back
      </Button>
    </div>
  );
}

function Starting({ operation, task, onStarted }: { operation: string; task: string; onStarted: () => void }) {
  const p = useCreateProgress(operation, { task, box: '2h' }, onStarted);
  return (
    <p className="flex items-center gap-2 text-sm text-muted">
      <LoaderCircle size={14} className="animate-spin" />
      {p.op?.state === 'failed' ? 'The agent did not start.' : 'Starting the agent…'}
      {!!p.ask.error && <span className="text-danger">The first task was not sent: {sentence(classify(p.ask.error))}</span>}
    </p>
  );
}
