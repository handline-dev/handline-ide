// Managing the accounts, as legacy's Settings > Accounts did: add one by name; each opens in place to log in (the vendor's URL, then the
// code back), set its alias, cap, recovery and month end, or be removed (a dry run first, then its name typed). Every write shows its
// pending state in place; a refusal says why under it.
import { RowsLoading } from '#src/usage/UsageViews.tsx';
import type { Account } from '@handline/client';
import { loginRunning } from '@handline/app-core';
import { classify, sentence, useAccounts, useWrite } from '@handline/app-data';
import { ChevronRight, LoaderCircle, Plus, TriangleAlert } from 'lucide-react';
import { useState } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { Field, Input } from '#src/ui/Field.tsx';
import { Page } from '#src/ui/Page.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';

const why = (e: unknown) => (e ? sentence(classify(e)) : null);

export function AccountsTab() {
  const q = useAccounts();
  const accounts = q.data?.accounts ?? [];
  const [open, setOpen] = useState<string | null>(null);
  return (
    <ScrollFade keep="accounts" className="min-h-0 flex-1">
      <Page>
        <div className="flex flex-col gap-3 py-4">
          <AddAccount />
          {q.isPending && <RowsLoading rows={6} />}
          {q.isError && <p className="text-sm text-danger">Accounts did not load.</p>}
          <div className="flex flex-col">
            {accounts.map((a) => (
              <div key={a.name} className="flex flex-col border-b border-border">
                <button
                  type="button"
                  aria-expanded={open === a.name}
                  onClick={() => setOpen(open === a.name ? null : a.name)}
                  className="flex h-12 items-center gap-2 text-left hover:bg-hover/30"
                >
                  <ChevronRight size={14} className={`shrink-0 text-muted transition-transform ${open === a.name ? 'rotate-90' : ''}`} />
                  <span className="text-sm font-medium text-fg">{a.label}</span>
                  {a.label !== a.name && <span className="text-xs text-muted">{a.name}</span>}
                  <span className="flex-1" />
                  {a.login_state === 'needs_login' ? (
                    <span className="flex items-center gap-1 text-xs text-danger">
                      <TriangleAlert size={12} />
                      Needs login
                    </span>
                  ) : (
                    <span className="text-xs text-muted">{a.agents.length ? `${a.agents.length} agents` : 'No agents'}</span>
                  )}
                </button>
                {open === a.name && <One account={a} />}
              </div>
            ))}
          </div>
        </div>
      </Page>
    </ScrollFade>
  );
}

/** A name registers an account; it then needs a login, and its row comes in with the accounts topic. */
function AddAccount() {
  const [asking, setAsking] = useState(false);
  const [name, setName] = useState('');
  const add = useWrite('account.add', name);
  const close = () => {
    setAsking(false);
    setName('');
    add.reset();
  };
  if (!asking)
    return (
      <Button className="w-fit" onClick={() => setAsking(true)}>
        <Plus size={14} />
        Add an account
      </Button>
    );
  return (
    <form
      className="flex flex-col gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        if (name.trim()) add.mutate({ account: name.trim() }, { onSuccess: close });
      }}
    >
      <Field label="Name" error={why(add.error)}>
        <Input autoFocus autoComplete="off" value={name} disabled={add.isPending} onChange={(e) => setName(e.target.value)} />
      </Field>
      <div className="flex gap-2">
        <Button type="submit" variant="primary" disabled={!name.trim() || add.isPending}>
          {add.isPending ? 'Adding…' : 'Add account'}
        </Button>
        <Button variant="ghost" onClick={close}>
          Cancel
        </Button>
      </div>
    </form>
  );
}

/** One account opened: its login, its settings (each saved when the field is left), and Remove. */
function One({ account: a }: { account: Account }) {
  const save = useWrite('account.set', a.name);
  const [alias, setAlias] = useState(a.label === a.name ? '' : a.label);
  const [cap, setCap] = useState(a.cap === null ? '' : String(a.cap));
  const [end, setEnd] = useState(a.cycle.end_is_default ? '' : (a.cycle.end ?? ''));
  // Another client's change comes in with the topic and replaces what the boxes show.
  const remote = [a.label, a.cap, a.cycle.end, a.cycle.end_is_default].join('|');
  const [seen, setSeen] = useState(remote);
  if (seen !== remote) {
    setSeen(remote);
    setAlias(a.label === a.name ? '' : a.label);
    setCap(a.cap === null ? '' : String(a.cap));
    setEnd(a.cycle.end_is_default ? '' : (a.cycle.end ?? ''));
  }
  return (
    <div className={`flex flex-col gap-4 pb-4 pl-6 transition-opacity ${save.isPending ? 'opacity-60' : ''}`} aria-busy={save.isPending}>
      <Login account={a} />
      <div className="grid gap-3 sm:grid-cols-3">
        <Field label="Alias">
          <Input
            value={alias}
            placeholder={a.name}
            onChange={(e) => setAlias(e.target.value)}
            onBlur={() => alias.trim() !== (a.label === a.name ? '' : a.label) && save.mutate({ account: a.name, label: alias.trim() })}
          />
        </Field>
        <Field label="Cap (agents at once)" hint={`now ${a.cap_effective}`}>
          <Input
            type="number"
            min={1}
            value={cap}
            placeholder={String(a.cap_effective)}
            onChange={(e) => setCap(e.target.value)}
            onBlur={() => {
              const n = Number(cap);
              if (cap !== '' && Number.isInteger(n) && n >= 1 && n !== a.cap) save.mutate({ account: a.name, cap: n });
            }}
          />
        </Field>
        <Field label="Last day" hint="Blank: when its login drops">
          <Input
            type="date"
            value={end}
            disabled={!a.cycle.on}
            onChange={(e) => setEnd(e.target.value)}
            onBlur={() =>
              end !== (a.cycle.end_is_default ? '' : (a.cycle.end ?? '')) && save.mutate({ account: a.name, cycle_end: end || null })
            }
          />
        </Field>
      </div>
      <div className="flex flex-wrap gap-4 text-sm">
        <label className="flex items-center gap-2">
          <input type="checkbox" checked={a.cycle.on} onChange={(e) => save.mutate({ account: a.name, cycle: e.target.checked })} />
          Ends this month
        </label>
        <label className="flex items-center gap-2" title="A dead usage read may spend one real turn to fix itself">
          <input type="checkbox" checked={a.recover} onChange={(e) => save.mutate({ account: a.name, recover: e.target.checked })} />
          Usage recovery
        </label>
      </div>
      {!!save.error && <p className="text-xs text-danger">{why(save.error)}</p>}
      <Remove account={a} />
    </div>
  );
}

/** Log in by URL and code: account.login starts the vendor's login and gives its URL; the code goes back with account.login_code. The code
 * is never kept. A login another window started can be cancelled here. */
export function Login({ account: a }: { account: Account }) {
  const login = useWrite('account.login', a.name);
  const sendCode = useWrite('account.login_code', a.name);
  const cancel = useWrite('account.login_cancel', a.name);
  const [code, setCode] = useState('');
  const run = loginRunning(a);
  const url = login.data?.url;
  const id = login.data?.login_id ?? a.login?.login_id ?? '';
  const submit = () => {
    sendCode.mutate({ login_id: id, code: code.trim() });
    setCode('');
  };
  if (run && url)
    return (
      <div className="flex flex-col gap-2 text-sm">
        <p className="text-muted">
          Open{' '}
          <a className="break-all text-accent" href={url} target="_blank" rel="noreferrer">
            {url}
          </a>{' '}
          and bring the code back.
        </p>
        <div className="flex items-end gap-2">
          <Field label="Code" error={why(sendCode.error)}>
            <Input
              autoFocus
              value={code}
              disabled={sendCode.isPending}
              onChange={(e) => setCode(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && code.trim() && submit()}
            />
          </Field>
          <Button variant="primary" disabled={!code.trim() || sendCode.isPending} onClick={submit}>
            {sendCode.isPending ? 'Sending…' : 'Submit'}
          </Button>
          <Button variant="ghost" disabled={cancel.isPending} onClick={() => cancel.mutate({ login_id: id })}>
            Cancel
          </Button>
        </div>
      </div>
    );
  if (run)
    return (
      <p className="flex items-center gap-2 text-sm text-muted">
        <LoaderCircle size={14} className="animate-spin" />
        Logging in
        <Button variant="ghost" disabled={cancel.isPending} onClick={() => cancel.mutate({ login_id: id })}>
          Cancel
        </Button>
      </p>
    );
  return (
    <div className="flex items-center gap-2 text-sm">
      <Button disabled={login.isPending} onClick={() => login.mutate({ account: a.name })}>
        {login.isPending ? 'Starting…' : a.login_state === 'ok' ? 'Log in again' : 'Log in'}
      </Button>
      {a.login?.state === 'failed' && <span className="text-danger">Login failed: {a.login.reason}</span>}
      {!!login.error && <span className="text-danger">{why(login.error)}</span>}
    </div>
  );
}

/** Remove: a dry run says what would happen and who sits on it; the name typed confirms. Its directory is moved aside, not deleted. */
function Remove({ account: a }: { account: Account }) {
  const plan = useWrite('account.remove', a.name);
  const remove = useWrite('account.remove', a.name);
  const [asking, setAsking] = useState(false);
  const [typed, setTyped] = useState('');
  if (!asking)
    return (
      <Button
        variant="ghost"
        className="w-fit text-danger"
        onClick={() => {
          setAsking(true);
          plan.mutate({ account: a.name, dry_run: true });
        }}
      >
        Remove account…
      </Button>
    );
  const would = plan.data?.would_remove === true;
  return (
    <div className="flex flex-col gap-2 rounded-md border border-border p-3 text-sm">
      {plan.isPending && <p className="text-muted">Checking what removing it would do…</p>}
      {a.agents.length > 0 && <p className="text-warning">On it now: {a.agents.join(', ')}. Move them off first.</p>}
      {(plan.data?.plan ?? []).map((l) => (
        <p key={l} className="text-muted">
          {l}
        </p>
      ))}
      {!!(plan.error || remove.error) && <p className="text-danger">{why(plan.error ?? remove.error)}</p>}
      {would && (
        <Field label={`Type ${a.name} to confirm`} hint="Its directory is moved aside, not deleted.">
          <Input autoComplete="off" value={typed} onChange={(e) => setTyped(e.target.value)} />
        </Field>
      )}
      <div className="flex gap-2">
        <Button
          variant="danger"
          disabled={!would || typed !== a.name || remove.isPending}
          onClick={() => remove.mutate({ account: a.name, confirm: typed })}
        >
          {remove.isPending ? 'Removing…' : 'Remove account'}
        </Button>
        <Button variant="ghost" onClick={() => setAsking(false)}>
          Cancel
        </Button>
      </div>
    </div>
  );
}
