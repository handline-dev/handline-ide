// The pieces of an opened account card: its Login row and Remove.
import type { Account } from '@handline/client';
import { classify, sentence, useWrite } from '@handline/app-data';
import { useEffect, useState, type ReactNode } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { Input } from '#src/ui/Field.tsx';
import { ModalFrame, openModal, type Close } from '#src/ui/modal.tsx';
import { Refs } from '#src/ui/refs.tsx';

const why = (e: unknown) => (e ? sentence(classify(e)) : null);

export function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-xs font-medium text-muted">{label}</span>
      <div className="flex min-h-8 flex-wrap items-center gap-2 text-sm">{children}</div>
    </div>
  );
}

/** Remove, in a modal: a dry run says what would happen and who sits on it; the name typed confirms. Its directory is moved aside, not deleted. */
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
          Type <b className="text-fg">{a.name}</b> to confirm. Its directory is moved aside, not deleted.
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
        {!a.agents.length && !!plan.data?.plan?.length && <p className="text-xs text-muted">{plan.data.plan.join(' · ')}</p>}
        {!!(remove.error || (plan.error && !a.agents.length)) && (
          <p className="text-danger">
            <Refs text={why(remove.error ?? plan.error)} />
          </p>
        )}
      </div>
    </ModalFrame>
  );
}
