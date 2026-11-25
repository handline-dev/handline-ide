import { Spinner } from '#src/ui/Loading.tsx';
import { Segmented } from '#src/ui/Segmented.tsx';
// Settings > Accounts: one card per account, its row Usage's, opened in place; several stay open and which is kept.
import { ClaudeNotice } from '#src/settings/AiLogin.tsx';
import { AccountFacts } from '#src/usage/AccountFacts.tsx';
import { removeAccount, renameAccount } from '#src/usage/AccountParts.tsx';
import { AccountEmail } from '#src/usage/AccountEmail.tsx';
import type { Account } from '@handline/client';
import { loginRunning } from '@handline/app-core';
import { useAccounts, useAccountUsage, useSettings, useWrite, why } from '@handline/app-data';
import { ChevronRight, Pause, Pencil, Play, Plus, Trash2 } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { Field, Input } from '#src/ui/Field.tsx';
import { ModalFrame, openModal, type Close } from '#src/ui/modal.tsx';
import { Page } from '#src/ui/Page.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { usePersisted } from '#src/persist/kv.ts';
import { RowsLoading } from '#src/usage/UsageViews.tsx';
import { Refs } from '#src/ui/refs.tsx';

/** Whether the daemon places and moves agents across the accounts (auto, the default) or leaves them where the person put them, waiting
 * at a wall for its reset (manual): the daemon's account_split setting. */
function SplitSetting() {
  const settings = useSettings();
  const save = useWrite('settings.set');
  const manual = (settings.data?.values as Record<string, unknown> | undefined)?.account_split === 'manual';
  return (
    <AutoManual
      name="Split work"
      manual={manual}
      ready={settings.isSuccess}
      save={save}
      pick={(v) => save.mutate({ changes: { account_split: v } })}
    />
  );
}

/** A setting of two choices, Auto and Manual: its name and the two. */
function AutoManual({
  name,
  manual,
  ready,
  save,
  pick,
}: {
  name: string;
  manual: boolean;
  ready: boolean;
  save: { isPending: boolean; error: unknown };
  pick: (v: 'auto' | 'manual') => void;
}) {
  return (
    <section className="flex items-center justify-between gap-6 px-4 py-3">
      <div className="min-w-0">
        <h3 className="text-sm text-fg">{name}</h3>
        {!!save.error && (
          <p className="text-xs text-danger" role="alert">
            {why(save.error)}
          </p>
        )}
      </div>
      <Segmented
        label={name}
        value={manual ? 'manual' : 'auto'}
        options={MODES}
        word={(v) => (v === 'auto' ? 'Auto' : 'Manual')}
        disabled={!ready || save.isPending}
        onChange={pick}
      />
    </section>
  );
}

const MODES = ['auto', 'manual'] as const;

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
        <div className="flex flex-col gap-4 pb-8 pt-3">
          <div className="flex items-center justify-between gap-4">
            <ClaudeNotice />
            <span className="flex-1" />
            <AddAccount
              names={accounts.map((a) => a.name)}
              onAdded={(n) => {
                setOpen((o) => (o.includes(n) ? o : [...o, n]));
                setFresh(n);
              }}
            />
          </div>
          <div className="flex flex-col border-t border-border">
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
            <div className="border-b border-border">
              <SplitSetting />
            </div>
          </div>
        </div>
      </Page>
    </ScrollFade>
  );
}

/** One click adds an account under a free name and starts its login. */
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
      {!!add.error && (
        <span className="text-xs text-danger">
          <Refs text={why(add.error)} />
        </span>
      )}
    </div>
  );
}

/** One account: the row (a button that opens it, or Log in when it is logged out), and opened, its Login row and the bottom row. */
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
    <section data-account={a.name} className="border-b border-border">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 pr-4 hover:bg-hover/30">
        <button
          type="button"
          aria-expanded={open}
          aria-label={`${open ? 'Close' : 'Open'} ${a.label}`}
          onClick={toggle}
          className={`flex min-w-0 items-center gap-3 p-4 text-left ${out ? 'pr-1' : '@container flex-1'}`}
        >
          <ChevronRight size={16} className={`shrink-0 text-muted transition-transform ${open ? 'rotate-90' : ''}`} />
          <AccountFacts account={a} plan={plan} bars={!out} />
        </button>
        {out && <Login account={a} autoStart={autoLogin} onStarted={onStarted} />}
      </div>
      {open && (
        <div className="flex flex-col gap-2 px-4 pb-4 pl-11">
          <AccountActions account={a} out={out} />
        </div>
      )}
    </section>
  );
}

/** One account's own settings, the same in Settings and over Usage (accountModal): its login, Rename, Pause or Resume, and Remove at the right, which asks for the name typed in a modal. */
export function AccountActions({ account: a, out }: { account: Account; out: boolean }) {
  const save = useWrite('account.set', a.name);
  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-3">
        {/* Rename first, under the name it changes; the changes that stop or remove the account together at the right. */}
        <Button variant="ghost" onClick={() => void renameAccount(a)}>
          <Pencil size={13} />
          Rename
        </Button>
        {out ? <AccountEmail account={a} className="text-sm text-fg" /> : <Login account={a} emailBefore />}
        <span className="flex-1" />
        <Button
          disabled={save.isPending}
          title={a.paused ? 'Let new agents start here again' : 'Start no new agents here'}
          onClick={() => save.mutate({ account: a.name, paused: !a.paused })}
        >
          {a.paused ? <Play size={13} /> : <Pause size={13} />}
          {a.paused ? 'Resume' : 'Pause'}
        </Button>
        <Button variant="ghost" className="text-danger" onClick={() => void removeAccount(a)}>
          <Trash2 size={13} />
          Remove
        </Button>
      </div>
      {!!save.error && (
        <p className="text-xs text-danger">
          <Refs text={why(save.error)} />
        </p>
      )}
    </div>
  );
}

/** Log in by URL and code: account.login starts the vendor's login and gives its URL, which opens in the browser with a modal for the code
 * (account.login_code). The code is never kept. A login another window started can be cancelled here. */
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
  const cancel = useWrite('account.login_cancel', a.name);
  const startedRef = useRef(false);
  useEffect(() => {
    if (!autoStart || startedRef.current) return;
    startedRef.current = true;
    login.mutate({ account: a.name });
    onStarted?.();
  }, [autoStart, onStarted, login, a.name]);
  // Each login this window starts: its page in the browser, the code in a modal.
  const started = login.data;
  useEffect(() => {
    if (!started?.url) return;
    const { url, login_id } = started;
    window.open(url, '_blank', 'noopener,noreferrer');
    void openModal((close) => <CodeModal name={a.name} loginId={login_id} url={url} close={close} />, {
      dismissible: false,
    });
  }, [started, a.name]);
  const run = loginRunning(a);
  const id = started?.login_id ?? a.login?.login_id ?? '';
  if (run)
    return (
      <p className="flex items-center gap-2 text-sm text-muted">
        <Spinner />
        Logging in
        <Button variant="ghost" disabled={cancel.isPending} onClick={() => cancel.mutate({ login_id: id })}>
          Cancel
        </Button>
      </p>
    );
  return (
    <div className="flex flex-wrap items-center gap-2 text-sm">
      {emailBefore && <AccountEmail account={a} before={a.login_state === 'ok' ? 'Logged in as' : ''} className="text-sm text-fg" />}
      {a.login_state === 'ok' ? (
        // A login that works needs no button's weight: logging in again is a link beside it.
        <button
          type="button"
          disabled={login.isPending}
          onClick={() => login.mutate({ account: a.name })}
          className="text-sm text-accent hover:underline disabled:opacity-50"
        >
          {login.isPending ? 'Starting…' : 'Log in again'}
        </button>
      ) : (
        <Button disabled={login.isPending} onClick={() => login.mutate({ account: a.name })}>
          {login.isPending ? 'Starting…' : 'Log in'}
        </Button>
      )}
      {!emailBefore && <AccountEmail account={a} before="as" />}
      {a.login?.state === 'failed' && (
        <span className="text-danger">
          Login failed: <Refs text={a.login.reason} />
        </span>
      )}
      {!!login.error && (
        <span className="text-danger">
          <Refs text={why(login.error)} />
        </span>
      )}
    </div>
  );
}

/** The code step of a login: Submit sends the code the login page shows, Cancel ends the login. It closes on its own once the login ends. */
function CodeModal({ name, loginId, url, close }: { name: string; loginId: string; url: string; close: Close<void> }) {
  const sendCode = useWrite('account.login_code', name);
  const cancel = useWrite('account.login_cancel', name);
  const [code, setCode] = useState('');
  const a = useAccounts().data?.accounts.find((x) => x.name === name);
  const ended = a?.login?.login_id === loginId && !loginRunning(a);
  useEffect(() => {
    if (ended) close();
  }, [ended, close]);
  const stop = () => cancel.mutate({ login_id: loginId }, { onSettled: () => close() });
  const submit = () => {
    if (!code.trim()) return;
    sendCode.mutate({ login_id: loginId, code: code.trim() }, { onSuccess: () => close() });
  };
  return (
    <ModalFrame
      title={`Log in to ${a?.label ?? name}`}
      description={
        <>
          Finish in the browser tab that opened{a?.email ? ` as ${a.email}` : ''}, then paste the code it shows here.{' '}
          <a className="text-accent" href={url} target="_blank" rel="noreferrer">
            Open the login page again
          </a>
        </>
      }
      size="sm"
      onClose={stop}
      footer={
        <>
          <Button disabled={cancel.isPending} onClick={stop}>
            Cancel
          </Button>
          <Button variant="primary" disabled={!code.trim() || sendCode.isPending} onClick={submit}>
            {sendCode.isPending ? 'Sending…' : 'Submit'}
          </Button>
        </>
      }
    >
      <Field label="Code" error={why(sendCode.error)}>
        <Input
          autoFocus
          autoComplete="off"
          value={code}
          disabled={sendCode.isPending}
          onChange={(e) => setCode(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && submit()}
        />
      </Field>
    </ModalFrame>
  );
}
