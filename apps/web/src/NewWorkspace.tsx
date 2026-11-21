import { classify, sentence, useOperation, useWrite } from '@handline/app-data';
import { useEffect, useState, type ReactNode } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { Field, Input } from '#src/ui/Field.tsx';
import { ModalFrame, openModal, type Close } from '#src/ui/modal.tsx';
import { shell } from '#src/shell.ts';
import { workspaceNameOf } from '#src/onboarding/firstTask.ts';

/** The create the daemon started: its step while it runs, then closed once done, or its failure. */
function Progress({ operation, close }: { operation: string; close: Close<string> }) {
  const op = useOperation(operation).data;
  const done = op?.state === 'done';
  useEffect(() => {
    if (done) close(operation);
  }, [done, close, operation]);
  if (done) return null;
  return op?.state === 'failed' ? (
    <div className="flex flex-col gap-2 pb-4">
      <p className="text-danger">{op.message ?? 'The workspace was not made.'}</p>
      <div className="flex justify-end">
        <Button onClick={() => close()}>Close</Button>
      </div>
    </div>
  ) : (
    <p className="pb-4 text-muted">{op?.step ?? 'Starting…'}</p>
  );
}

/** A folder's workspace name, from its last part, as onboarding names it. */
const nameOf = (folder: string) =>
  workspaceNameOf(
    folder
      .replace(/[\\/]+$/, '')
      .split(/[\\/]/)
      .pop() ?? '',
  );

/** Where no folder picker is (a browser): the folder typed, its name shown as it will be. */
function Form({ close }: { close: Close<string> }) {
  const create = useWrite('workspace.create');
  const [folder, setFolder] = useState('');
  const [started, setStarted] = useState<string | null>(null);
  if (started) return <Progress operation={started} close={close} />;
  const ready = !!folder.trim();
  return (
    <form
      className="flex flex-col gap-3 pb-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (ready)
          create.mutate(
            { name: nameOf(folder.trim()), folder: folder.trim() },
            { onSuccess: (r) => setStarted((r as { operation: string }).operation) },
          );
      }}
    >
      <Field label="Folder">
        <Input autoFocus value={folder} onChange={(e) => setFolder(e.target.value)} placeholder="/path/to/folder" />
      </Field>
      {!!create.error && <p className="text-sm text-danger">{sentence(classify(create.error))}</p>}
      <div className="flex justify-end gap-2 pt-1">
        <Button onClick={() => close()}>Cancel</Button>
        <Button type="submit" variant="primary" disabled={!ready || create.isPending}>
          {create.isPending ? 'Creating…' : 'Create'}
        </Button>
      </div>
    </form>
  );
}

/** Add workspace with the folder picked: the workspace it makes, named after the folder as in onboarding, and Create. */
function Picked({ folder, close }: { folder: string; close: Close<string> }) {
  const create = useWrite('workspace.create');
  const [started, setStarted] = useState<string | null>(null);
  if (started) return <Progress operation={started} close={close} />;
  return (
    <div className="flex flex-col gap-3 pb-4">
      <p>
        <span className="font-medium">{nameOf(folder)}</span> <span className="text-muted">· {folder}</span>
      </p>
      {!!create.error && <p className="text-sm text-danger">{sentence(classify(create.error))}</p>}
      <div className="flex justify-end gap-2 pt-1">
        <Button onClick={() => close()}>Cancel</Button>
        <Button
          variant="primary"
          autoFocus
          disabled={create.isPending}
          onClick={() =>
            create.mutate({ name: nameOf(folder), folder }, { onSuccess: (r) => setStarted((r as { operation: string }).operation) })
          }
        >
          {create.isPending ? 'Creating…' : 'Create'}
        </Button>
      </div>
    </div>
  );
}

/** Add workspace: a folder is picked first, git or plain; where no picker is, it is typed. */
export function newWorkspace() {
  const choose = shell()?.pickFolder;
  const open = (body: (close: Close<string>) => ReactNode) =>
    void openModal<string>((close) => (
      <ModalFrame title="Add workspace" onClose={() => close()}>
        {body(close)}
      </ModalFrame>
    ));
  if (!choose) return open((close) => <Form close={close} />);
  void choose().then(
    (r) => r.ok && r.path && open((close) => <Picked folder={r.path!} close={close} />),
    () => undefined,
  );
}
