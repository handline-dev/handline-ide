import { classify, sentence, useNewAgentForm, useOperation } from '@handline/app-data';
import { useEffect, useState } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { Field, Input, Select } from '#src/ui/Field.tsx';
import { ModalFrame, openModal, type Close } from '#src/ui/modal.tsx';

/** Watches the create the daemon started: its step while it runs, then done (closing with the new name) or failed. */
function Progress({ operation, name, close }: { operation: string; name: string; close: Close<string> }) {
  const op = useOperation(operation).data;
  if (op?.state === 'done') {
    queueMicrotask(() => close(name));
    return null;
  }
  return (
    <div className="flex flex-col gap-2 pb-4">
      {op?.state === 'failed' ? (
        <p className="text-danger">{op.message ?? 'The create failed.'}</p>
      ) : (
        <p className="text-muted">
          {op?.step ?? 'Starting…'}
          {op?.steps_total ? ` (${op.steps_done}/${op.steps_total})` : ''}
        </p>
      )}
      {op?.state === 'failed' && (
        <div className="flex justify-end">
          <Button onClick={() => close()}>Close</Button>
        </div>
      )}
    </div>
  );
}

function Form({ parent, close }: { parent?: string; close: Close<string> }) {
  const [started, setStarted] = useState<{ operation: string; name: string } | null>(null);
  const form = useNewAgentForm((operation, f) => setStarted({ operation, name: (f.parent ? `${f.parent}.` : '') + f.name.trim() }));
  const { f, set, list, prefix, ready, send, create, agents } = form;
  // A sub-agent's dialog starts with its parent picked.
  useEffect(() => {
    if (parent) set('parent', parent);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- once, on open
  }, []);
  if (started) return <Progress {...started} close={close} />;
  const error = create.error ? sentence(classify(create.error)) : null;
  return (
    <form
      className="flex flex-col gap-3 pb-4"
      onSubmit={(e) => {
        e.preventDefault();
        send();
      }}
    >
      <Field label="Name" hint={prefix ? `Becomes ${prefix}${f.name || '…'}` : undefined}>
        <Input autoFocus value={f.name} onChange={(e) => set('name', e.target.value)} placeholder="reviewer" />
      </Field>
      <Field label="Role">
        <Input value={f.role} onChange={(e) => set('role', e.target.value)} placeholder="What it does" />
      </Field>
      <Field label="Project">
        <Select value={f.repo} onChange={(e) => set('repo', e.target.value)}>
          <option value="">Pick a repository</option>
          {list.map((r) => (
            <option key={r.path} value={r.path}>
              {r.path.split('/').pop()}
            </option>
          ))}
        </Select>
      </Field>
      <Field label="Base branch" hint="Empty: the repository's default branch">
        <Input value={f.base} onChange={(e) => set('base', e.target.value)} placeholder={form.repo?.default_branch ?? 'main'} />
      </Field>
      <Field label="Reports to">
        <Select value={f.parent} onChange={(e) => set('parent', e.target.value)}>
          <option value="">Nobody (a new top-level agent)</option>
          {agents.map((a) => (
            <option key={a.name} value={a.name}>
              {a.name}
            </option>
          ))}
        </Select>
      </Field>
      {error && <p className="text-sm text-danger">{error}</p>}
      <div className="flex justify-end gap-2 pt-1">
        <Button onClick={() => close()}>Cancel</Button>
        <Button type="submit" variant="primary" disabled={!ready || create.isPending}>
          {create.isPending ? 'Creating…' : 'Create'}
        </Button>
      </div>
    </form>
  );
}

/** The new-agent dialog; resolves the new agent's name once it is created, or undefined. */
export function newAgent(parent?: string) {
  return openModal<string>((close) => (
    <ModalFrame title={parent ? `New agent under ${parent}` : 'New agent'} onClose={() => close()}>
      <Form parent={parent} close={close} />
    </ModalFrame>
  ));
}
