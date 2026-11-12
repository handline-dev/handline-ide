// The five steps' bodies. Each has one button that does the step; what is true on the machine already shows as done.
import { classify, hooks, sentence, useRead, useRepos, useStatus } from '@handline/app-data';
import { Bell, Check, Copy, Folder, FolderGit2, GitBranch, LoaderCircle, Terminal } from 'lucide-react';
import { useEffect, useState, type ReactNode } from 'react';
import { shell } from '#src/shell.ts';
import { useAiTools, useOnboarding } from '#src/onboarding/data/progress.ts';
import { BASE_TOOLS, type AiRow } from '#src/onboarding/data/steps.ts';
import { setPicked, usePicked } from '#src/onboarding/data/picked.ts';
import { StepView, useStepNav } from '#src/onboarding/StepView.tsx';
import { ago } from '#src/format/time.ts';
import { copy } from '#src/ui/copy.ts';

const VENDOR: Record<string, { name: string; mark: string; tone: string }> = {
  claude: { name: 'Claude Code', mark: 'CC', tone: 'bg-warning text-bg' },
  codex: { name: 'Codex', mark: 'CX', tone: 'bg-fg text-bg' },
};
export const vendorOf = (v: string) => VENDOR[v] ?? { name: v, mark: v.slice(0, 2).toUpperCase(), tone: 'bg-fg text-bg' };

export function Mark({ vendor, small }: { vendor: string; small?: boolean }) {
  const v = vendorOf(vendor);
  return (
    <span
      className={`flex shrink-0 items-center justify-center rounded-sm font-semibold ${v.tone} ${small ? 'size-4 text-xs' : 'size-7 text-xs'}`}
    >
      {v.mark}
    </span>
  );
}

/** A bordered row: an icon, a name with its line, and something at the right. */
function Line({ icon, name, note, end, picked }: { icon: ReactNode; name: ReactNode; note: ReactNode; end?: ReactNode; picked?: boolean }) {
  return (
    <div className={`flex items-center gap-3 rounded-md border p-3 ${picked ? 'border-accent' : 'border-border'}`}>
      {icon}
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="font-medium text-fg">{name}</span>
        <span className="truncate text-xs text-muted">{note}</span>
      </span>
      {end}
    </div>
  );
}

const Pill = ({ tone, children }: { tone: 'success' | 'warning' | 'accent'; children: ReactNode }) => (
  <span
    className={`shrink-0 rounded-full border px-2 text-xs ${
      tone === 'success'
        ? 'border-success/40 text-success'
        : tone === 'warning'
          ? 'border-warning/40 text-warning'
          : 'border-accent/40 text-accent'
    }`}
  >
    {children}
  </span>
);

const openUrl = (url: string) => window.open(url, '_blank', 'noopener,noreferrer');

export function AiStep() {
  const nav = useStepNav();
  const ai = useAiTools();
  const [code, setCode] = useState<string | null>(null);
  const [error, setError] = useState('');
  const ready = ai.rows.filter((r) => r.state === 'ready');
  const found = ai.rows.filter((r) => r.state === 'found');
  const logIn = (vendor: string) =>
    ai.logIn(vendor).then((s) => {
      if (!s) return;
      if (s.deviceCode) setCode(s.deviceCode);
      openUrl(s.url);
    });
  const use = (r: AiRow) => (r.adoptable && r.source ? ai.adopt(r.vendor, r.source) : logIn(r.vendor));
  const run = (p: Promise<unknown>) => {
    setError('');
    p.catch((e: unknown) => setError(sentence(classify(e))));
  };
  const primary = ready.length
    ? { label: 'Continue', onClick: nav.next }
    : found.length
      ? { label: 'Use these', onClick: () => run(Promise.all(found.map(use))), disabled: ai.busy }
      : {
          label: `Log in to ${vendorOf(ai.rows[0]?.vendor ?? 'claude').name}`,
          onClick: () => run(logIn(ai.rows[0]?.vendor ?? 'claude')),
          disabled: ai.busy || !ai.rows.length,
        };
  const line = ready.length
    ? `${ready.map((r) => vendorOf(r.vendor).name).join(' and ')} ${ready.length > 1 ? 'are' : 'is'} ready.${ai.rows.length > ready.length ? ' Add the other too if you use it.' : ''}`
    : found.length
      ? 'We found these logins on this Mac. Agents will use them.'
      : 'Log in to the AI tool your agents run on.';
  return (
    <StepView title="Your AI coding tools" line={line} primary={primary}>
      {ai.loading && <LoaderCircle size={16} className="animate-spin text-muted" />}
      {ai.rows.map((r) => (
        <Line
          key={r.vendor}
          picked={r.state === 'ready' || r.state === 'found'}
          icon={<Mark vendor={r.vendor} />}
          name={vendorOf(r.vendor).name}
          note={
            r.state === 'ready'
              ? `Ready${r.email ? ` · ${r.email}` : ''}`
              : r.state === 'found'
                ? 'Found on this Mac'
                : r.state === 'logging_in'
                  ? 'Finish logging in in your browser'
                  : !r.installed
                    ? `Not installed: it installs when you log in (${r.fix})`
                    : 'Not logged in'
          }
          end={
            r.state === 'ready' || r.state === 'found' ? (
              <Check size={14} className="text-accent" />
            ) : r.state === 'logging_in' ? (
              <Pill tone="accent">Waiting for your browser</Pill>
            ) : (
              <button
                type="button"
                disabled={ai.busy}
                onClick={() => run(logIn(r.vendor))}
                className="text-xs text-accent hover:underline disabled:opacity-50"
              >
                Log in
              </button>
            )
          }
        />
      ))}
      {code && (
        <p className="text-xs text-muted">
          Type this code in the browser: <code className="font-mono text-fg">{code}</code>
        </p>
      )}
      {error && <p className="text-xs text-danger">{error}</p>}
      <p className="text-xs text-muted">
        {ready.length
          ? 'One tool is enough. You can continue now.'
          : 'Handline uses the logins claude and codex already have in Terminal. Nothing to type.'}{' '}
        Other accounts and API keys are in Settings › Accounts.
      </p>
    </StepView>
  );
}

const TOOL_NOTE: Record<string, { note: string; icon: ReactNode }> = {
  git: { note: 'Every agent works on its own branch.', icon: <GitBranch size={14} /> },
  tmux: { note: 'Keeps agents running when you close the window.', icon: <Terminal size={14} /> },
};

export function ToolsStep() {
  const nav = useStepNav();
  const status = useStatus();
  const missing = (status.data?.missing_tools ?? []).filter((m) => BASE_TOOLS.includes(m.tool));
  const install = shell()?.installTools;
  const [running, setRunning] = useState(false);
  const [error, setError] = useState('');
  // Terminal installing a tool tells the daemon nothing: while it runs, the list is asked again every few seconds.
  useRead('daemon.status', {}, { refetchMs: running ? 3000 : undefined });
  const names = missing.map((m) => m.tool).join(' and ');
  // Once what was installing has landed, the step moves on by itself.
  useEffect(() => {
    if (running && status.isSuccess && !missing.length) nav.next();
  }, [running, status.isSuccess, missing.length, nav]);
  const line = !status.isSuccess
    ? 'Checking this Mac…'
    : missing.length
      ? `${BASE_TOOLS.filter((t) => !missing.some((m) => m.tool === t)).join(' and ') || 'Nothing'} is here. ${names} ${missing.length > 1 ? 'are' : 'is'} missing.`
      : 'Both are here.';
  const primary =
    missing.length && install
      ? {
          label: running ? `Installing ${names}…` : `Install ${names} and continue`,
          disabled: running,
          onClick: () => {
            setError('');
            void install().then(
              (r) => (r.ok ? setRunning(true) : setError(r.message)),
              (e: unknown) => setError(sentence(classify(e))),
            );
          },
        }
      : { label: 'Continue', onClick: nav.next, disabled: !status.isSuccess };
  return (
    <StepView title="Two tools every agent needs" line={line} primary={primary}>
      <div className="flex flex-col divide-y divide-border rounded-md border border-border">
        {BASE_TOOLS.map((t) => {
          const m = missing.find((x) => x.tool === t);
          return (
            <div key={t} className="flex items-center gap-3 p-3">
              <span className="flex size-7 shrink-0 items-center justify-center rounded-md border border-border text-muted">
                {TOOL_NOTE[t]?.icon}
              </span>
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="font-medium text-fg">{t}</span>
                <span className="truncate text-xs text-muted">{TOOL_NOTE[t]?.note}</span>
                {m && !install && (
                  <button
                    type="button"
                    onClick={() => void copy(m.fix, 'command')}
                    className="mt-1 flex w-fit items-center gap-1 text-xs text-fg"
                  >
                    <code className="font-mono">{m.fix}</code>
                    <Copy size={11} className="text-muted" />
                  </button>
                )}
              </span>
              {status.isSuccess && (m ? <Pill tone="warning">Missing</Pill> : <Pill tone="success">Installed</Pill>)}
            </div>
          );
        })}
      </div>
      {error && <p className="text-xs text-danger">{error}</p>}
      <p className="text-xs text-muted">
        {install ? 'Installs with Homebrew in Terminal, where you can watch it.' : 'Run the command in Terminal, then come back.'} Settings
        › Tools has more.
      </p>
    </StepView>
  );
}

export function NotifyStep() {
  const nav = useStepNav();
  const ob = useOnboarding();
  const on = ob.notify === 'granted' || ob.notify === 'unsupported';
  const primary = on
    ? { label: 'Continue', onClick: nav.next }
    : ob.notify === 'denied'
      ? { label: 'Continue', onClick: () => (ob.skipNotify(), nav.next()) }
      : {
          label: (
            <>
              <Bell size={14} />
              Allow notifications
            </>
          ),
          onClick: () => void ob.askNotify().finally(nav.next),
        };
  return (
    <StepView title="Hear back from your agents" line="Even when the Handline window is closed." primary={primary}>
      <div className="flex items-center justify-center rounded-md bg-accent/5 px-4 py-8">
        <div className="flex w-72 items-start gap-2.5 rounded-lg border border-border bg-bg p-3 shadow-popover">
          <span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-accent text-bg">
            <Bell size={14} />
          </span>
          <span className="flex min-w-0 flex-col text-xs">
            <span className="font-medium text-fg">Handline</span>
            <span className="text-fg">api-server needs you</span>
            <span className="truncate text-muted">"May I run the database migrations?"</span>
          </span>
        </div>
      </div>
      <p className="text-xs text-muted">
        {on
          ? 'Notifications are on.'
          : ob.notify === 'denied'
            ? 'They are blocked for Handline; turn them on in System Settings › Notifications.'
            : 'When an agent finishes or asks you something.'}{' '}
        Pick which ones, and sounds, in Settings › Notifications.
      </p>
    </StepView>
  );
}

const folderName = (p: string) => p.replace(/\/+$/, '').split('/').pop() ?? p;

export function ProjectStep() {
  const nav = useStepNav();
  const recent = hooks.useReposRecent();
  const repos = useRepos().data?.repos ?? [];
  const chosen = usePicked();
  const folders = [
    ...repos.map((r) => ({ path: r.path, name: r.name, touched_at: null as string | null })),
    ...(recent.data?.folders ?? []),
  ];
  const sel = chosen ?? folders[0] ?? null;
  const pick = shell()?.pickFolder;
  const [error, setError] = useState('');
  return (
    <StepView
      title="What should agents work on?"
      line="Git repositories found on this Mac. Pick one; add more any time."
      primary={{
        label: sel ? `Add ${sel.name}` : 'Pick a folder',
        disabled: !sel,
        onClick: () => (setPicked(sel), nav.next()),
      }}
    >
      {recent.isPending && <LoaderCircle size={16} className="animate-spin text-muted" />}
      {folders.length > 0 && (
        <div role="radiogroup" className="flex flex-col divide-y divide-border rounded-md border border-border">
          {folders.map((f) => {
            const on = sel?.path === f.path;
            return (
              <button
                key={f.path}
                type="button"
                role="radio"
                aria-checked={on}
                onClick={() => setPicked({ path: f.path, name: f.name })}
                className={`flex items-center gap-3 px-3 py-2 text-left ${on ? 'bg-selected' : 'hover:bg-hover/30'}`}
              >
                {on ? (
                  <Check size={14} className="shrink-0 text-accent" />
                ) : (
                  <span className="size-3.5 shrink-0 rounded-full border border-border" />
                )}
                <FolderGit2 size={14} className="shrink-0 text-muted" />
                <span className="w-36 shrink-0 truncate font-medium text-fg">{f.name}</span>
                <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted">{f.path.replace(/^\/Users\/[^/]+/, '~')}</span>
                {f.touched_at && <span className="shrink-0 text-xs text-muted">{ago(f.touched_at)}</span>}
              </button>
            );
          })}
        </div>
      )}
      {!recent.isPending && !folders.length && <p className="text-xs text-muted">No git folders found nearby.</p>}
      {error && <p className="text-xs text-danger">{error}</p>}
      {pick && (
        <button
          type="button"
          onClick={() =>
            void pick().then(
              (r) => (r.ok ? r.path && setPicked({ path: r.path, name: folderName(r.path) }) : setError(r.message)),
              (e: unknown) => setError(sentence(classify(e))),
            )
          }
          className="flex w-fit items-center gap-1.5 rounded-md border border-border px-2.5 py-1 text-xs text-fg hover:bg-hover/30"
        >
          <Folder size={12} />
          Other folder…
        </button>
      )}
    </StepView>
  );
}
