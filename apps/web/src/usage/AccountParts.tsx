// The modals of an opened account row: Rename and Remove.
import type { Account } from '@handline/client';
import { useWrite, why } from '@handline/app-data';
import { useEffect, useState } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { Input } from '#src/ui/Field.tsx';
import { ModalFrame, openModal, type Close } from '#src/ui/modal.tsx';
import { Refs } from '#src/ui/refs.tsx';

/** Remove, in a modal: a dry run says what would happen and who sits on it; the name typed confirms. */
export function removeAccount(a: Account) {
  return openModal<boolean>((close) => <RemoveModal account={a} close={close} />);
}

function RemoveModal({ account: a, close }: { account: Account; close: Close<boolean> }) {
  const plan = useWrite('account.remove', a.name);
  const remove = useWrite('account.remove', a.name);
  const [typed, setTyped] = useState('');
  const { mutate: dryRun } = plan;
  useEffect(() => dryRun({ account: a.name, dry_run: true }), [dryRun, a.name]);
  const would = plan.data?.would_remove === true;
  return (
    <ModalFrame
      title={`Remove ${a.label}?`}
      description={
        <>
          Type <b className="text-fg">{a.name}</b> to confirm.
        </>
      }
      size="sm"
      footer={
        <>
          <Button onClick={() => close(false)}>Cancel</Button>
          <Button
            variant="danger"
            disabled={!would || typed !== a.name || remove.isPending}
            onClick={() => remove.mutate({ account: a.name, confirm: typed }, { onSuccess: () => close(true) })}
          >
            {remove.isPending ? 'Removing…' : 'Remove'}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-2">
        <Input
          autoFocus
          autoComplete="off"
          aria-label="Type the account name to confirm"
          value={typed}
          onChange={(e) => setTyped(e.target.value)}
        />
        {plan.isPending && <p className="text-muted">Checking what removing it would do…</p>}
        {a.agents.length > 0 && <p className="text-warning">On it now: {a.agents.join(', ')}. Move them off first.</p>}
        {!!(remove.error || (plan.error && !a.agents.length)) && (
          <p className="text-danger">
            <Refs text={why(remove.error ?? plan.error)} />
          </p>
        )}
      </div>
    </ModalFrame>
  );
}

/** Rename, in a modal: the name screens show (the account's alias); empty goes back to its own name. */
export function renameAccount(a: Account) {
  return openModal<void>((close) => <RenameModal account={a} close={close} />);
}

function RenameModal({ account: a, close }: { account: Account; close: Close<void> }) {
  const save = useWrite('account.set', a.name);
  const [label, setLabel] = useState(a.label);
  const submit = () => {
    if (save.isPending) return;
    if (label.trim() === a.label) return close();
    save.mutate({ account: a.name, label: label.trim() }, { onSuccess: () => close() });
  };
  return (
    <ModalFrame
      title="Rename account"
      size="sm"
      footer={
        <>
          <Button onClick={() => close()}>Cancel</Button>
          <Button variant="primary" disabled={save.isPending} onClick={submit}>
            {save.isPending ? 'Saving…' : 'Save'}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-2">
        <Input
          autoFocus
          autoComplete="off"
          aria-label="Account name"
          maxLength={40}
          placeholder={a.name}
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && submit()}
        />
        {!!save.error && (
          <p className="text-xs text-danger">
            <Refs text={why(save.error)} />
          </p>
        )}
      </div>
    </ModalFrame>
  );
}
