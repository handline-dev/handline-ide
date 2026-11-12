// First run, step one: the tools agents need (the missing ones, installed by one button), the Claude login (one button; the account is
// named for you), and notifications. Continue turns on once logged in.
import { loginRunning } from '@handline/app-core';
import { classify, sentence, useAccounts, useRead, useStatus, useWrite } from '@handline/app-data';
import { Bell, Check, Copy, Download, Play, TriangleAlert } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { copy } from '#src/ui/copy.ts';
import { shell } from '#src/shell.ts';
import { Login } from '#src/usage/AccountsTab.tsx';

const ACCOUNT = 'claude';

/** True when the desktop app says its login shell did not answer at the daemon's start: the missing tools it names may be installed. */
function usePathTimedOut(): boolean {
  const [timedOut, setTimedOut] = useState(false);
  useEffect(() => {
    void shell()
      ?.daemon?.()
      .then((d) => setTimedOut(d.ok && d.pathTimedOut === true));
  }, []);
  return timedOut;
}

export function GetReady({ onContinue }: { onContinue: () => void }) {
  const missing = useStatus().data?.missing_tools ?? [];
  const slowShell = usePathTimedOut();
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
        {slowShell && (
          <p className="text-sm text-warning">
            Your login shell did not answer in time, so the tools below may be installed already. A restart asks it again.
          </p>
        )}
        {missing.length ? <Missing missing={missing} /> : <p className="text-sm text-muted">claude, git and tmux are installed.</p>}
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
      <Button variant="primary" className="self-end" disabled={!loggedIn || missing.length > 0} onClick={onContinue}>
        Continue
      </Button>
    </div>
  );
}

/** What each tool is for, in the line that says it is missing. */
const FOR: Record<string, string> = {
  claude: 'runs the agents',
  git: "Apple's command line tools",
  tmux: 'runs each agent in its own terminal',
};

/** The missing tools, a line each, and one button that installs them all in Terminal. Without the desktop app, each one's command to copy. */
function Missing({ missing }: { missing: { tool: string; fix: string }[] }) {
  const install = shell()?.installTools;
  const [state, setState] = useState<{ running: boolean; error: string }>({ running: false, error: '' });
  // A tool landing tells the daemon nothing: while Terminal installs, the list is asked again every few seconds.
  useRead('daemon.status', {}, { refetchMs: state.running ? 3000 : undefined });
  const run = (tool?: string) =>
    void install?.(tool ? { tool } : undefined).then(
      (r) => setState(r.ok ? { running: true, error: '' } : { running: false, error: r.message }),
      (e: unknown) => setState({ running: false, error: e instanceof Error ? e.message : String(e) }),
    );
  return (
    <div className="flex flex-col gap-3 rounded-lg border border-border p-3">
      {missing.map((m) => (
        <div key={m.tool} className="flex flex-col gap-1 text-sm">
          <div className="flex items-center gap-2">
            <TriangleAlert size={14} className="text-warning" />
            <span className="text-fg">{m.tool} is missing</span>
            <span className="text-muted">{FOR[m.tool]}</span>
          </div>
          <div className="flex items-center gap-2 pl-5">
            <div className="flex h-8 min-w-0 flex-1 items-center rounded-md border border-border bg-surface pl-2.5">
              <code className="min-w-0 flex-1 truncate text-xs text-fg">{m.fix}</code>
              <button
                type="button"
                aria-label={`Copy ${m.fix}`}
                title="Copy"
                className="flex h-full items-center px-2 text-muted hover:text-fg"
                onClick={() => void copy(m.fix, 'command')}
              >
                <Copy size={13} />
              </button>
            </div>
            {install && (
              <Button className="h-8" onClick={() => run(m.tool)}>
                <Play size={12} />
                Run
              </Button>
            )}
          </div>
        </div>
      ))}
      {install && (
        <>
          {missing.length > 1 && (
            <Button variant="primary" className="h-8 w-full justify-center" disabled={state.running} onClick={() => run()}>
              <Download size={14} />
              Install {missing.map((m) => m.tool).join(' and ')}
            </Button>
          )}
          <p className="text-xs text-muted">Run opens Terminal, where you can watch it install.</p>
          {missing.some((m) => m.tool === 'git') && (
            <p className="text-xs text-muted">For git, Apple's installer opens its own window: click Install there, then come back here.</p>
          )}
          {state.running && <p className="text-xs text-muted">Installing in Terminal. This list updates as each one lands.</p>}
          {state.error && <p className="text-xs text-danger">{state.error}</p>}
        </>
      )}
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
