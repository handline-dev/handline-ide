// First run: with no agent yet the app opens on this full-screen flow: Get ready, the folder to work in, what to do first. The agent is
// made from the folder (named for it, no role or "Reports to" asked) and its first task goes with it.
import { classify, hooks, sentence, useAgents, useCreateProgress, useWrite } from '@handline/app-data';
import { RpcError } from '@handline/client';
import { FolderGit2, LoaderCircle } from 'lucide-react';
import { useState } from 'react';
import { LayerDragStrip, useWindowLayer } from '#src/layout/windowLayer.tsx';
import { shell } from '#src/shell.ts';
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
type Card = { title: string; note: string; task: string };
/** What the repo itself offers to do first: its tests, its newest open issues, its TODOs, its uncommitted changes. */
export function factCards(
  f: { test_command: string | null; issues: { number: number; title: string }[] | null; todos: number; uncommitted: number } | undefined,
): Card[] {
  if (!f) return [];
  return [
    ...(f.test_command
      ? [{ title: 'Run the tests and fix what fails', note: f.test_command, task: `Run ${f.test_command} and fix what fails.` }]
      : []),
    ...(f.issues ?? []).map((i) => ({
      title: `#${i.number} ${i.title}`,
      note: 'Open issue',
      task: `Work on issue #${i.number}: ${i.title}`,
    })),
    ...(f.todos
      ? [
          {
            title: `Go through ${f.todos} TODOs`,
            note: 'TODO and FIXME comments',
            task: 'Go through the TODO and FIXME comments, and do the ones worth doing.',
          },
        ]
      : []),
    ...(f.uncommitted
      ? [
          {
            title: `Tidy ${f.uncommitted} uncommitted changes`,
            note: 'Review and commit them',
            task: 'Review the uncommitted changes and commit them in sensible pieces.',
          },
        ]
      : []),
  ];
}
const LOOK_AROUND = 'Look around this project and propose 3 tasks worth doing next. Read only: change nothing.';

function FirstRun({ onStarted }: { onStarted: () => void }) {
  const [step, setStep] = useState<'ready' | 'repo' | 'first'>('ready');
  const [folder, setFolder] = useState<Folder | null>(null);
  useWindowLayer(true, { drag: true });
  return (
    <div className="fixed inset-0 z-50 overflow-y-auto bg-bg">
      <div className="sticky top-0 z-10 bg-bg">
        <LayerDragStrip />
      </div>
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

/** The desktop app's folder chooser where it has one; a path typed in otherwise. */
function ChooseAnother({ onPick }: { onPick: (f: Folder) => void }) {
  const [path, setPath] = useState('');
  const name = (p: string) => p.replace(/\/+$/, '').split('/').pop() ?? p;
  const api = shell();
  const pick = api?.pickFolder ? () => api.pickFolder!() : null;
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
      {pick && (
        <Button
          onClick={() =>
            void pick().then((r) => {
              if (r.ok && r.path) onPick({ path: r.path, name: name(r.path) });
            })
          }
        >
          Choose a folder…
        </Button>
      )}
    </form>
  );
}

/** repo.facts on a folder that is not a git repository (or not there; folder.init then says which). */
export const notGit = (e: unknown) => e instanceof RpcError && e.code === 'not_found' && e.data?.what === 'path';

/** A folder that is not a git repository yet: folder.init makes it one (git init and a first commit), and its facts are read again.
 * `reading`: that read is on its way, so the folder may be one already. */
function StartGit({ path, reading }: { path: string; reading: boolean }) {
  const init = useWrite('folder.init');
  return (
    <div className="flex flex-col gap-2 rounded-md border border-border p-3 text-sm">
      <p className="text-fg">This folder is not a git repository yet. Agents work on git branches, so it needs one.</p>
      <p className="text-xs text-muted">
        Start git here makes one, with a first commit of what the folder holds. Secret files like .env stay out.
      </p>
      <Button variant="primary" className="w-fit" disabled={init.isPending || reading} onClick={() => init.mutate({ path })}>
        {init.isPending ? 'Starting git…' : 'Start git here'}
      </Button>
      {!!init.error && <p className="text-danger">{sentence(classify(init.error))}</p>}
    </div>
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
  const facts = hooks.useRepoFacts({ path: folder.path });
  if (operation) return <Starting operation={operation} task={task} onStarted={onStarted} />;
  const cards = factCards(facts.data);
  const bare = notGit(facts.error);
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-xl font-medium text-fg">What first, in {folder.name}?</h1>
      {bare ? (
        <StartGit path={folder.path} reading={facts.isFetching} />
      ) : (
        facts.isError && <p className="text-sm text-danger">{sentence(classify(facts.error))}</p>
      )}
      {!bare &&
        [{ title: 'Look around and propose 3 tasks', note: 'Reads only; nothing is changed.', task: LOOK_AROUND }, ...cards].map((c) => (
          <button
            key={c.title}
            type="button"
            disabled={create.isPending || facts.isError}
            onClick={() => start(c.task)}
            className="rounded-md border border-border p-3 text-left hover:bg-hover/30 disabled:opacity-60"
          >
            <div className="text-sm font-medium text-fg">{c.title}</div>
            <div className="text-xs text-muted">{c.note}</div>
          </button>
        ))}
      {!bare && (
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
          <Button type="submit" variant="primary" disabled={!own.trim() || create.isPending || facts.isError}>
            Start
          </Button>
        </form>
      )}
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
