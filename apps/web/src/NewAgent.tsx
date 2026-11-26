import { Hooks, useAgentOptions, useDirectory, useNewAgentForm, useOperation, why } from '@handline/app-data';
import type { Workspace } from '@handline/client';
import { Check, Eye } from 'lucide-react';
import { useState } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { Field, Input } from '#src/ui/Field.tsx';
import { ModalFrame, openModal, type Close } from '#src/ui/modal.tsx';
import { Refs } from '#src/ui/refs.tsx';
import { OperationProgress } from '#src/ui/OperationProgress.tsx';
import { AddFolder, FolderName } from '#src/WorkspaceFolders.tsx';

/** Watches the create the daemon started, closing with the new agent's key once the list holds the agent of that name in the workspace. */
function Progress({ operation, name, workspace, close }: { operation: string; name: string; workspace: string; close: Close<string> }) {
  const op = useOperation(operation).data;
  // The daemon's result names the new agent; its name in the workspace before the list has it.
  const made = (op?.result as { agent?: string } | null)?.agent;
  const key = useDirectory().agents.find((a) => a.key === made || (!made && a.name === name && a.workspace === workspace))?.key;
  return (
    <OperationProgress operation={operation} failed="The create failed." ready={!!key} onDone={() => close(key)} close={() => close()} />
  );
}

/** The workspace's folders to work in, one row each with a git or plain-folder mark; the picked one checked; Add folder last. */
function Folders({ wsKey, picked, pick }: { wsKey: string; picked: string; pick: (path: string) => void }) {
  // Read live, so a folder added here shows at once.
  const ws = Hooks.useWorkspacesList().data?.workspaces.find((w) => w.key === wsKey);
  if (!ws) return null;
  return (
    <div role="radiogroup" aria-label="Folder" className="flex flex-col overflow-hidden rounded-md border border-border">
      {ws.folders.map((d) => {
        const on = picked ? picked === d.path : d.default;
        return (
          <button
            key={d.path}
            type="button"
            role="radio"
            aria-checked={on}
            title={d.path}
            onClick={() => pick(d.default ? '' : d.path)}
            className={`flex h-9 items-center gap-2 border-t border-border px-3 text-left text-sm outline-none first:border-t-0 focus-visible:bg-hover ${on ? 'bg-selected' : 'hover:bg-hover'}`}
          >
            <FolderName folder={d} />
            <span className="flex-1" />
            {/* Works in the checked one, reads the others. */}
            {on ? (
              <Check size={14} className="shrink-0 text-accent" aria-label="works here" />
            ) : (
              <Eye size={14} className="shrink-0 text-muted" aria-label="reads" />
            )}
          </button>
        );
      })}
      <div className="border-t border-border">
        <AddFolder wsKey={wsKey} onAdded={pick} />
      </div>
    </div>
  );
}

function Form({ ws, close }: { ws: Workspace; close: Close<string> }) {
  const [started, setStarted] = useState<{ operation: string; name: string } | null>(null);
  const { f, set, ready, send, create } = useNewAgentForm(ws.key, (operation, name) => setStarted({ operation, name }));
  const defaultName = useAgentOptions().data?.default_names?.[ws.key] ?? '';
  if (started) return <Progress {...started} workspace={ws.key} close={close} />;
  const error = create.error ? why(create.error) : null;
  return (
    <form
      className="flex flex-col gap-3 pb-4"
      onSubmit={(e) => {
        e.preventDefault();
        send();
      }}
    >
      {/* Each empty field gets the daemon's pick, shown as its placeholder. */}
      <Field label="Name">
        <Input autoFocus value={f.name} onChange={(e) => set('name', e.target.value)} placeholder={defaultName} />
      </Field>
      <Field label="Role">
        <Input value={f.role} onChange={(e) => set('role', e.target.value)} placeholder="Helps with anything in this workspace." />
      </Field>
      <Field label="Folder">
        <Folders wsKey={ws.key} picked={f.folder} pick={(p) => set('folder', p)} />
      </Field>
      {error && (
        <p className="text-sm text-danger">
          <Refs text={error} />
        </p>
      )}
      <div className="flex justify-end gap-2 pt-1">
        <Button onClick={() => close()}>Cancel</Button>
        <Button type="submit" variant="primary" disabled={!ready || create.isPending}>
          {create.isPending ? 'Creating…' : 'Create'}
        </Button>
      </div>
    </form>
  );
}

/** The new-agent dialog for a workspace; resolves the new agent's key once it is created, or undefined. */
export function newAgent(ws: Workspace) {
  return openModal<string>((close) => (
    <ModalFrame title={`New agent in ${ws.name}`} onClose={() => close()}>
      <Form ws={ws} close={close} />
    </ModalFrame>
  ));
}
