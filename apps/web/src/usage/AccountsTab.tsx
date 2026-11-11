// Settings > Accounts: one card per account, its row Usage's, opened in place; several stay open and which is kept.
import { AccountFacts } from '#src/usage/AccountFacts.tsx';
import { Remove, Rename, Row, Stepper, SubscriptionEnd } from '#src/usage/AccountParts.tsx';
import { AccountEmail } from '#src/usage/AccountEmail.tsx';
import type { Account } from '@handline/client';
import { loginRunning } from '@handline/app-core';
import { classify, sentence, useAccounts, useAccountUsage, useWrite } from '@handline/app-data';
import { ChevronRight, LoaderCircle, Pause, Play, Plus, Trash2 } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { Field, Input } from '#src/ui/Field.tsx';
import { Page } from '#src/ui/Page.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { usePersisted } from '#src/persist/kv.ts';
import { RowsLoading } from '#src/usage/UsageViews.tsx';

const why = (e: unknown) => (e ? sentence(classify(e)) : null);

/** `focus`: an account whose card opens and scrolls into view. */
export function AccountsTab({ focus }: { focus?: string }) {
  const q = useAccounts();
  const accounts = q.data?.accounts ?? [];
  const plans = new Map((useAccountUsage().data?.accounts ?? []).map((u) => [u.name, u.plan]));
  const [open, setOpen] = usePersisted<string[]>('settings:accounts:open', []);
  // An account just added: its login starts on its own.
  const [fresh, setFresh] = useState<string | null>(null);
  const found = focus !== undefined && accounts.some((a) => a.name === focus);
  useEffect(() => {
    if (!found) return;
    setOpen((o) => (o.includes(focus) ? o : [...o, focus]));
    document.querySelector(`[data-account="${CSS.escape(focus)}"]`)?.scrollIntoView({ block: 'center' });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- once per link
  }, [found, focus]);
  const toggle = (name: string) => setOpen((o) => (o.includes(name) ? o.filter((x) => x !== name) : [...o, name]));
  return (
    <ScrollFade keep="accounts" className="min-h-0 flex-1">
      <Page>
        <div className="flex flex-col gap-4 py-6">
          <div className="flex items-end justify-between gap-4">
            <div>
              <h1 className="text-xl font-semibold text-fg">Accounts</h1>
              <p className="text-sm text-muted">Agents are spread across these Claude accounts.</p>
            </div>
            <AddAccount
              names={accounts.map((a) => a.name)}
              onAdded={(n) => {
                setOpen((o) => (o.includes(n) ? o : [...o, n]));
                setFresh(n);
              }}
            />
          </div>
          {q.isPending && <RowsLoading rows={6} />}
          {q.isError && <p className="text-sm text-danger">Accounts did not load.</p>}
          {accounts.map((a) => (
            <Card
              key={a.name}
              account={a}
              plan={plans.get(a.name)}
              open={open.includes(a.name)}
              toggle={() => toggle(a.name)}
              autoLogin={fresh === a.name}
              onStarted={() => setFresh(null)}
            />
          ))}
        </div>
      </Page>
    </ScrollFade>
  );
}

/** One click adds an account under a free name and starts its login; Rename changes the name after. */
function AddAccount({ names, onAdded }: { names: string[]; onAdded: (name: string) => void }) {
  const add = useWrite('account.add', 'new');
  const free = () => {
    let n = names.length + 1;
    while (names.includes(`account-${n}`)) n++;
    return `account-${n}`;
  };
  return (
    <div className="flex flex-col items-end gap-1">
      <Button
        variant="primary"
        disabled={add.isPending}
        onClick={() => {
          const name = free();
          add.mutate({ account: name }, { onSuccess: () => onAdded(name) });
        }}
      >
        <Plus size={14} />
        Add account
      </Button>
      {!!add.error && <span className="text-xs text-danger">{why(add.error)}</span>}
    </div>
  );
}

/** One account: the row (a button that opens it, or Log in when it is logged out), and opened, its two columns and the bottom row. */
function Card({
  account: a,
  plan,
  open,
  toggle,
  autoLogin,
  onStarted,
}: {
  account: Account;
  plan?: string | null;
  open: boolean;
  toggle: () => void;
  autoLogin: boolean;
  onStarted: () => void;
}) {
  const out = a.login_state === 'needs_login';
  return (
    <section data-account={a.name} className={`rounded-xl border border-border ${a.paused ? 'bg-surface/60' : 'bg-bg'}`}>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 pr-4">
        <button
          type="button"
          aria-expanded={open}
          aria-label={`${open ? 'Close' : 'Open'} ${a.label}`}
          onClick={toggle}
          className="@container flex min-w-0 flex-1 items-center gap-3 rounded-xl p-4 text-left hover:bg-hover/20"
        >
          <ChevronRight size={16} className={`shrink-0 text-muted transition-transform ${open ? 'rotate-90' : ''}`} />
          <AccountFacts account={a} plan={plan} bars={!out} />
        </button>
        {out && <Login account={a} autoStart={autoLogin} onStarted={onStarted} />}
      </div>
      {open && (
        <div className="flex flex-col gap-5 border-t border-border bg-surface/40 p-4 pl-12">
          <div className="grid gap-x-12 gap-y-5 sm:grid-cols-2">
            <div className="flex flex-col gap-5">
              <Rename account={a} />
              <Row label="Login">{out ? <AccountEmail account={a} className="text-sm text-fg" /> : <Login account={a} emailBefore />}</Row>
            </div>
            <div className="flex flex-col gap-5">
              <Row label="Agents at once">
                <Stepper account={a} />
              </Row>
              <Row label="Subscription end">
                <SubscriptionEnd account={a} />
              </Row>
            </div>
          </div>
          <Bottom account={a} />
        </div>
      )}
    </section>
  );
}

/** Pause or Resume with what it does, and Remove at the right, which asks for the name typed in a red band in its place. */
function Bottom({ account: a }: { account: Account }) {
  const save = useWrite('account.set', a.name);
  const plan = useWrite('account.remove', a.name);
  const [removing, setRemoving] = useState(false);
  if (removing) return <Remove account={a} plan={plan} done={() => setRemoving(false)} />;
  return (
    <div className="flex flex-col gap-2 border-t border-border pt-4">
      <div className="flex flex-wrap items-center gap-3">
        <Button disabled={save.isPending} onClick={() => save.mutate({ account: a.name, paused: !a.paused })}>
          {a.paused ? <Play size={13} /> : <Pause size={13} />}
          {a.paused ? 'Resume' : 'Pause'}
        </Button>
        <span className="min-w-0 flex-1 text-xs text-muted">
          {a.paused
            ? a.agents.length
              ? 'No new agents. The running one moves at its next turn.'
              : 'No new agents go here.'
            : 'Stops new agents here. Running ones move at their next turn.'}
        </span>
        <Button
          variant="ghost"
          className="text-danger"
          onClick={() => {
            setRemoving(true);
            plan.mutate({ account: a.name, dry_run: true });
          }}
        >
          <Trash2 size={13} />
          Remove
        </Button>
      </div>
      {!!save.error && <p className="text-xs text-danger">{why(save.error)}</p>}
    </div>
  );
}

/** Log in by URL and code: account.login starts the vendor's login and gives its URL; the code goes back with account.login_code. The code
 * is never kept. A login another window started can be cancelled here. */
export function Login({
  account: a,
  autoStart,
  onStarted,
  emailBefore,
}: {
  account: Account;
  autoStart?: boolean;
  onStarted?: () => void;
  emailBefore?: boolean;
}) {
  const login = useWrite('account.login', a.name);
  const sendCode = useWrite('account.login_code', a.name);
  const cancel = useWrite('account.login_cancel', a.name);
  const [code, setCode] = useState('');
  const started = useRef(false);
  useEffect(() => {
    if (!autoStart || started.current) return;
    started.current = true;
    login.mutate({ account: a.name });
    onStarted?.();
  }, [autoStart, onStarted, login, a.name]);
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
    <div className="flex flex-wrap items-center gap-2 text-sm">
      {emailBefore && <AccountEmail account={a} className="text-sm text-fg" />}
      <Button disabled={login.isPending} onClick={() => login.mutate({ account: a.name })}>
        {login.isPending ? 'Starting…' : a.login_state === 'ok' ? 'Log in again' : 'Log in'}
      </Button>
      {!emailBefore && <AccountEmail account={a} as />}
      {a.login?.state === 'failed' && <span className="text-danger">Login failed: {a.login.reason}</span>}
      {!!login.error && <span className="text-danger">{why(login.error)}</span>}
    </div>
  );
}
