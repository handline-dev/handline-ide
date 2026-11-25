// A vendor's own login on this Mac, where it is used: Claude's in one line over the Accounts list, only when something needs doing;
// Codex's on its row in Tools. A found login is used as it is; a missing one is logged in once in the browser.
import { why } from '@handline/app-data';
import { useState } from 'react';
import { useAiTools } from '#src/onboarding/data/progress.ts';
import type { AiRow } from '#src/onboarding/data/steps.ts';
import { Button } from '#src/ui/Button.tsx';
import { InstallButton, Installing } from '#src/install/install.tsx';
import { Spinner } from '#src/ui/Loading.tsx';

function useVendor(vendor: string) {
  const ai = useAiTools();
  const [login, setLogin] = useState<{ url: string; code: string | null } | null>(null);
  const [error, setError] = useState<unknown>(null);
  const run = (go: () => Promise<unknown>) => {
    setError(null);
    go().catch(setError);
  };
  const logIn = () =>
    run(async () => {
      const r = await ai.logIn(vendor);
      if (!r) return;
      setLogin({ url: r.url, code: r.deviceCode });
      window.open(r.url, '_blank', 'noopener,noreferrer');
    });
  const row: AiRow | undefined = ai.rows.find((r) => r.vendor === vendor);
  const act = () => (row?.state === 'found' && row.source && row.adoptable ? run(() => ai.adopt(vendor, row.source!)) : logIn());
  return { row, busy: ai.busy, login, error, act, logIn };
}

function Pending({ login, error }: { login: { url: string; code: string | null } | null; error: unknown }) {
  return (
    <>
      {login && (
        <span className="text-xs text-muted">
          {login.code && (
            <>
              Code <code className="text-fg">{login.code}</code>{' '}
            </>
          )}
          <a className="text-accent" href={login.url} target="_blank" rel="noreferrer">
            Open the login page
          </a>
        </span>
      )}
      {!!error && (
        <span className="text-xs text-danger" role="alert">
          {why(error)}
        </span>
      )}
    </>
  );
}

/** Over the Accounts list: one line when Claude Code is missing or a login of it found here is not used yet; else nothing. */
export function ClaudeNotice() {
  const v = useVendor('claude');
  const r = v.row;
  if (!r) return null;
  const missing = !r.installed;
  const found = r.state === 'found' && r.adoptable;
  if (!missing && !found && r.state !== 'logging_in') return null;
  return (
    <div className="flex flex-wrap items-center gap-3 text-sm text-fg">
      {missing
        ? 'Claude Code is not installed'
        : r.state === 'logging_in'
          ? 'Waiting for the login in your browser…'
          : 'Claude login found on this Mac'}
      {missing && <InstallButton tool="claude">Install</InstallButton>}
      {missing && <Installing tool="claude" />}
      {found && (
        <Button disabled={v.busy} onClick={v.act}>
          Use
        </Button>
      )}
      {r.state === 'logging_in' && <Spinner className="text-muted" />}
      <Pending login={v.login} error={v.error} />
    </div>
  );
}

/** On the codex row in Tools: its login state and the one button that fixes it. */
export function CodexLogin() {
  const v = useVendor('codex');
  const r = v.row;
  if (!r?.installed) return null;
  return (
    <>
      <span className="text-xs text-muted">
        {r.state === 'ready' && (r.email ? `Logged in as ${r.email}` : 'Logged in')}
        {r.state === 'found' && 'Login found on this Mac'}
        {r.state === 'none' && 'Not logged in'}
      </span>
      {r.state === 'logging_in' && <Spinner className="text-muted" />}
      {(r.state === 'found' || r.state === 'none') && (
        <Button size="sm" disabled={v.busy} onClick={v.act}>
          {r.state === 'found' && r.adoptable ? 'Use' : 'Log in'}
        </Button>
      )}
      {r.state === 'ready' && (
        <Button size="sm" variant="ghost" disabled={v.busy} onClick={v.logIn}>
          Log in again
        </Button>
      )}
      <Pending login={v.login} error={v.error} />
    </>
  );
}
