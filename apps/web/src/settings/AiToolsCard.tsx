// Settings > Accounts, at the top: Claude Code and Codex, each logged in, found on this Mac, or not yet; a found login is used as it is (Codex)
// or logged in once in the browser (Claude), and a missing login starts one.
import { classify, sentence } from '@handline/app-data';
import { Check, LoaderCircle, Search } from 'lucide-react';
import { useState } from 'react';
import { useAiTools } from '#src/onboarding/data/progress.ts';
import type { AiRow } from '#src/onboarding/data/steps.ts';
import { Block } from '#src/settings/Meter.tsx';
import { Button } from '#src/ui/Button.tsx';
import { copy } from '#src/ui/copy.ts';

const NAME: Record<string, string> = { claude: 'Claude Code', codex: 'Codex' };

export function AiToolsCard() {
  const ai = useAiTools();
  const [login, setLogin] = useState<{ vendor: string; url: string; code: string | null } | null>(null);
  const [error, setError] = useState<unknown>(null);
  const run = (go: () => Promise<unknown>) => {
    setError(null);
    go().catch(setError);
  };
  const logIn = (vendor: string) =>
    run(async () => {
      const r = await ai.logIn(vendor);
      if (!r) return;
      setLogin({ vendor, url: r.url, code: r.deviceCode });
      window.open(r.url, '_blank', 'noopener,noreferrer');
    });
  const act = (r: AiRow) => (r.state === 'found' && r.source && r.adoptable ? run(() => ai.adopt(r.vendor, r.source!)) : logIn(r.vendor));
  return (
    <Block title="AI coding tools" note="Agents use the logins claude and codex already have on this Mac. Nothing to type.">
      {ai.loading && <p className="text-sm text-muted">Looking…</p>}
      {ai.rows.map((r) => (
        <div key={r.vendor} data-vendor={r.vendor} className="flex flex-wrap items-center gap-3 text-sm">
          <div className="min-w-0 flex-1">
            <div className="text-fg">{NAME[r.vendor] ?? r.vendor}</div>
            <div className="truncate text-xs text-muted">
              {r.state === 'ready' && (r.email ? `Logged in as ${r.email}` : 'Logged in')}
              {r.state === 'logging_in' && 'Waiting for the login in your browser…'}
              {r.state === 'found' && 'A login found on this Mac'}
              {r.state === 'none' && (r.installed ? 'Not logged in' : 'Not installed: see Tools')}
            </div>
            {login?.vendor === r.vendor && (
              <div className="text-xs text-muted">
                {login.code && (
                  <>
                    Code <code className="text-fg">{login.code}</code>{' '}
                  </>
                )}
                <a className="text-accent" href={login.url} target="_blank" rel="noreferrer">
                  Open the login page
                </a>
              </div>
            )}
          </div>
          {r.state === 'ready' && (
            <span className="inline-flex items-center gap-1 text-xs text-success">
              <Check size={12} />
              Ready
            </span>
          )}
          {r.state === 'logging_in' && <LoaderCircle size={14} className="animate-spin text-muted" />}
          {(r.state === 'found' || r.state === 'none') && (
            <Button disabled={ai.busy || !r.installed} onClick={() => act(r)}>
              {r.state === 'found' && r.adoptable ? 'Use' : 'Log in'}
            </Button>
          )}
          {r.state === 'ready' && (
            <Button variant="ghost" disabled={ai.busy} onClick={() => logIn(r.vendor)}>
              Log in again
            </Button>
          )}
          {!r.installed && r.fix && (
            <Button variant="ghost" onClick={() => void copy(r.fix, 'command')}>
              Copy install command
            </Button>
          )}
        </div>
      ))}
      {!!error && (
        <p className="text-xs text-danger" role="alert">
          {sentence(classify(error))}
        </p>
      )}
      <div>
        <Button disabled={ai.looking} onClick={ai.look}>
          <Search size={13} />
          Look for logins again
        </Button>
      </div>
    </Block>
  );
}
