// The pieces of an opened account card: its name, its agents at once, its subscription end, and Remove.
import type { Account } from '@handline/client';
import { classify, sentence, useWrite } from '@handline/app-data';
import { Minus, Pencil, Plus, Trash2 } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { Input } from '#src/ui/Field.tsx';
import { capSet, MAX_CAP } from '#src/usage/AccountFacts.tsx';

const why = (e: unknown) => (e ? sentence(classify(e)) : null);

export function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-xs font-medium text-muted">{label}</span>
      <div className="flex min-h-8 flex-wrap items-center gap-2 text-sm">{children}</div>
    </div>
  );
}

/** The name as text with Rename beside it; Rename turns it into a box with Save and Cancel (Enter saves, Escape cancels). */
export function Rename({ account: a }: { account: Account }) {
  const save = useWrite('account.set', a.name);
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(a.label);
  const done = () => {
    setEditing(false);
    save.reset();
  };
  const commit = () => {
    const next = name.trim();
    if (!next || next === a.label) return done();
    save.mutate({ account: a.name, label: next }, { onSuccess: done });
  };
  return (
    <Row label="Name">
      {editing ? (
        <>
          <div className="w-36">
            <Input
              autoFocus
              value={name}
              disabled={save.isPending}
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') commit();
                if (e.key === 'Escape') {
                  e.preventDefault();
                  done();
                }
              }}
            />
          </div>
          <Button variant="primary" disabled={save.isPending} onClick={commit}>
            Save
          </Button>
          <Button variant="ghost" onClick={done}>
            Cancel
          </Button>
          {!!save.error && <span className="text-xs text-danger">{why(save.error)}</span>}
        </>
      ) : (
        <>
          <span className="text-fg">{a.label}</span>
          <Button
            onClick={() => {
              setName(a.label);
              setEditing(true);
            }}
          >
            <Pencil size={13} />
            Rename
          </Button>
        </>
      )}
    </Row>
  );
}

/** The most agents at once, 1 to 8, one step at a time; "now N" and a line say when the daemon has lowered it for a while. */
export function Stepper({ account: a }: { account: Account }) {
  const save = useWrite('account.set', a.name);
  const set = capSet(a);
  const go = (n: number) => save.mutate({ account: a.name, cap: n });
  const lowered = !a.paused && a.cap_effective < set;
  return (
    <>
      <div className="flex h-8 items-center rounded-md border border-border bg-bg">
        <button
          type="button"
          aria-label="Fewer agents"
          disabled={set <= 1 || save.isPending}
          onClick={() => go(set - 1)}
          className="grid h-full w-8 place-items-center text-muted hover:text-fg disabled:opacity-40"
        >
          <Minus size={14} />
        </button>
        <span className="w-9 border-x border-border text-center leading-8 tabular-nums" aria-live="polite">
          {set}
        </span>
        <button
          type="button"
          aria-label="More agents"
          disabled={set >= MAX_CAP || save.isPending}
          onClick={() => go(set + 1)}
          className="grid h-full w-8 place-items-center text-muted hover:text-fg disabled:opacity-40"
        >
          <Plus size={14} />
        </button>
      </div>
      {lowered && (
        <>
          <span className="rounded-full bg-surface px-2 text-xs leading-5 text-fg">{`↓ now ${a.cap_effective}`}</span>
          <span className="text-xs text-muted">Lowered for a while to save this week's usage.</span>
        </>
      )}
      {!!save.error && <span className="text-xs text-danger">{why(save.error)}</span>}
    </>
  );
}

/** Off: the account ends when its login drops. On: a date box; unchecking clears the date and the cycle. */
export function SubscriptionEnd({ account: a }: { account: Account }) {
  const save = useWrite('account.set', a.name);
  const [want, setWant] = useState(a.cycle.on);
  const [seen, setSeen] = useState(a.cycle.on);
  if (seen !== a.cycle.on) {
    setSeen(a.cycle.on);
    setWant(a.cycle.on);
  }
  return (
    <>
      <label className="flex items-center gap-2">
        <input
          type="checkbox"
          checked={want}
          onChange={(e) => {
            setWant(e.target.checked);
            if (!e.target.checked && (a.cycle.on || a.cycle.end)) save.mutate({ account: a.name, cycle: false, cycle_end: null });
          }}
        />
        Set end date
      </label>
      {want && (
        <div className="w-40">
          <Input
            type="date"
            aria-label="Last day"
            value={a.cycle.on ? (a.cycle.end ?? '') : ''}
            onChange={(e) => e.target.value && save.mutate({ account: a.name, cycle_end: e.target.value })}
          />
        </div>
      )}
      {!!save.error && <span className="text-xs text-danger">{why(save.error)}</span>}
    </>
  );
}

/** Remove: a dry run says what would happen and who sits on it; the name typed confirms. Its directory is moved aside, not deleted. */
export function Remove({
  account: a,
  plan,
  done,
}: {
  account: Account;
  plan: ReturnType<typeof useWrite<'account.remove'>>;
  done: () => void;
}) {
  const remove = useWrite('account.remove', a.name);
  const [typed, setTyped] = useState('');
  const would = plan.data?.would_remove === true;
  return (
    <div className="flex flex-col gap-2 rounded-lg border border-danger/30 bg-danger/5 p-3 text-sm">
      <div className="flex flex-wrap items-center gap-3">
        <Trash2 size={16} className="text-danger" />
        <div className="flex min-w-0 flex-1 flex-col">
          <span className="font-medium text-fg">{`Remove ${a.label}?`}</span>
          <span className="text-xs text-muted">
            Type <b className="text-fg">{a.name}</b> to confirm. Its directory is moved aside, not deleted.
          </span>
        </div>
        <div className="w-44">
          <Input
            autoFocus
            autoComplete="off"
            aria-label="Type the account name to confirm"
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
          />
        </div>
        <Button variant="ghost" onClick={done}>
          Cancel
        </Button>
        <Button
          variant="danger"
          disabled={!would || typed !== a.name || remove.isPending}
          onClick={() => remove.mutate({ account: a.name, confirm: typed })}
        >
          {remove.isPending ? 'Removing…' : 'Remove'}
        </Button>
      </div>
      {plan.isPending && <p className="text-muted">Checking what removing it would do…</p>}
      {a.agents.length > 0 && <p className="text-warning">On it now: {a.agents.join(', ')}. Move them off first.</p>}
      {!a.agents.length && !!plan.data?.plan?.length && <p className="text-xs text-muted">{plan.data.plan.join(' · ')}</p>}
      {!!(remove.error || (plan.error && !a.agents.length)) && <p className="text-danger">{why(remove.error ?? plan.error)}</p>}
    </div>
  );
}
