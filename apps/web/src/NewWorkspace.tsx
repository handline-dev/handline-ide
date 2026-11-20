import { classify, sentence, useOperation, useWrite } from '@handline/app-data';
import { useEffect, useState } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { Field, Input } from '#src/ui/Field.tsx';
import { ModalFrame, openModal, type Close } from '#src/ui/modal.tsx';
import { shell } from '#src/shell.ts';

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

function Form({ close }: { close: Close<string> }) {
  const create = useWrite('workspace.create');
  const [name, setName] = useState('');
  const [folder, setFolder] = useState('');
  const [started, setStarted] = useState<string | null>(null);
  if (started) return <Progress operation={started} close={close} />;
  const choose = shell()?.pickFolder;
  const ready = !!name.trim() && !!folder.trim();
  return (
    <form
      className="flex flex-col gap-3 pb-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (ready)
          create.mutate(
            { name: name.trim(), folder: folder.trim() },
            { onSuccess: (r) => setStarted((r as { operation: string }).operation) },
          );
      }}
    >
      <Field label="Name">
        <Input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="shop" />
      </Field>
      <Field label="Folder">
        <div className="flex gap-2">
          <Input value={folder} onChange={(e) => setFolder(e.target.value)} placeholder="/path/to/folder" className="flex-1" />
          {choose && (
            <Button
              onClick={() =>
                void choose().then(
                  (r) => r.ok && r.path && setFolder(r.path),
                  () => undefined,
                )
              }
            >
              Choose…
            </Button>
          )}
        </div>
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

/** Add workspace: its name and first folder, any folder, git or plain. */
export function newWorkspace() {
  void openModal<string>((close) => (
    <ModalFrame title="Add workspace" onClose={() => close()}>
      <Form close={close} />
    </ModalFrame>
  ));
}
