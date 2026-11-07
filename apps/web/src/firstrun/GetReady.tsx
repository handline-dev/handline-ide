// First run, step one: the tools agents need (an install command for each one missing), the Claude login (one button; the account is
// named for you), and notifications. Continue turns on once logged in.
import { loginRunning } from '@handline/app-core';
import { classify, sentence, useAccounts, useStatus, useWrite } from '@handline/app-data';
import { Bell, Check, TriangleAlert } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { copy } from '#src/ui/copy.ts';
import { Login } from '#src/usage/AccountsTab.tsx';

const ACCOUNT = 'claude';

export function GetReady({ onContinue }: { onContinue: () => void }) {
  const missing = useStatus().data?.missing_tools ?? [];
  const accounts = useAccounts();
  const list = accounts.data?.accounts ?? [];
  const account = list[0];
  const add = useWrite('account.add', ACCOUNT);
  const made = useRef(false);
  // The account is made once the list has loaded empty; the login button then belongs to it.
  useEffect(() => {
    if (accounts.isSuccess && !list.length && !made.current) {
      made.current = true;
      add.mutate({ account: ACCOUNT });
    }
  }, [accounts.isSuccess, list.length, add]);
  const loggedIn = account?.login_state === 'ok';
  // A machine that is already set up passes straight through; one that needed something waits for Continue.
  const ready = loggedIn && !missing.length;
  const waited = useRef(false);
  useEffect(() => {
    if (accounts.isSuccess && !ready) waited.current = true;
    if (accounts.isSuccess && ready && !waited.current) onContinue();
  }, [accounts.isSuccess, ready, onContinue]);
  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-xl font-medium text-fg">Get ready</h1>
      <Row done={!missing.length} title="Tools">
        {missing.length ? (
          missing.map((m) => (
            <div key={m.tool} className="flex items-center gap-2 text-sm">
              <TriangleAlert size={14} className="text-warning" />
              <span className="text-fg">{m.tool} is missing</span>
              <code className="rounded bg-surface px-1.5 py-0.5 text-xs text-muted">{m.fix}</code>
              <Button variant="ghost" onClick={() => void copy(m.fix, 'command')}>
                Copy
              </Button>
            </div>
          ))
        ) : (
          <p className="text-sm text-muted">claude, git and tmux are installed.</p>
        )}
      </Row>
      <Row done={loggedIn} title="Claude login">
        {add.isError && <p className="text-sm text-danger">{sentence(classify(add.error))}</p>}
        {loggedIn ? (
          <p className="text-sm text-muted">Logged in.</p>
        ) : account ? (
          <Login account={account} />
        ) : (
          <p className="text-sm text-muted">Setting up…</p>
        )}
        {!loggedIn && !!account && !loginRunning(account) && <p className="text-xs text-muted">New agents use Sonnet at medium effort.</p>}
      </Row>
      <Row done={false} title="Notifications" optional>
        <Notifications />
      </Row>
      <Button variant="primary" className="w-fit" disabled={!loggedIn || missing.length > 0} onClick={onContinue}>
        Continue
      </Button>
    </div>
  );
}

function Row({ title, done, optional, children }: { title: string; done: boolean; optional?: boolean; children: React.ReactNode }) {
  return (
    <section className="flex flex-col gap-2">
      <h2 className="flex items-center gap-2 text-sm font-medium text-fg">
        {done && <Check size={14} className="text-success" />}
        {title}
        {optional && <span className="text-xs font-normal text-muted">recommended</span>}
      </h2>
      {children}
    </section>
  );
}

function Notifications() {
  const supported = typeof Notification !== 'undefined';
  const [state, setState] = useState(supported ? Notification.permission : 'denied');
  if (!supported) return <p className="text-sm text-muted">Not available here.</p>;
  if (state === 'granted') return <p className="text-sm text-muted">On.</p>;
  if (state === 'denied') return <p className="text-sm text-muted">Blocked; change it in the system settings.</p>;
  return (
    <Button className="w-fit" onClick={() => void Notification.requestPermission().then(setState)}>
      <Bell size={14} />
      Allow notifications
    </Button>
  );
}
