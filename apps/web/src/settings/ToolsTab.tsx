// Settings > Tools: what agents run on, installed or missing, and the command that installs what is missing.
import { classify, sentence, useRead, useStatus } from '@handline/app-data';
import { Check, Copy, TriangleAlert } from 'lucide-react';
import { useState } from 'react';
import { shell } from '#src/shell.ts';
import { copy } from '#src/ui/copy.ts';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { AI_VENDORS, BASE_TOOLS } from '#src/onboarding/data/steps.ts';
import { Button } from '#src/ui/Button.tsx';

const FOR: Record<string, string> = {
  git: 'Version control. Every agent works on its own branch.',
  tmux: 'Keeps agents running when you close the window.',
  claude: 'Claude Code, which runs the agents.',
  codex: 'Codex, which can run agents too.',
};

export function ToolsTab() {
  const status = useStatus();
  const missing = status.data?.missing_tools ?? [];
  const vendors = status.data?.vendors ?? [];
  const rows = [
    ...BASE_TOOLS.map((tool) => ({
      tool,
      installed: !missing.some((m) => m.tool === tool),
      fix: missing.find((m) => m.tool === tool)?.fix ?? '',
    })),
    ...AI_VENDORS.flatMap((vendor) => {
      const v = vendors.find((x) => x.vendor === vendor);
      return v ? [{ tool: v.binary, installed: v.installed, fix: v.fix }] : [];
    }),
  ];
  const gone = rows.filter((r) => !r.installed && r.fix);
  const ok = status.isSuccess;
  return (
    <SettingsPage
      keep="settings-tools"
      title="Tools"
      note="What agents run on, and whether it is installed."
      action={
        <Button disabled={status.isFetching} onClick={() => void status.refetch()}>
          Check again
        </Button>
      }
    >
      <div className="flex flex-col rounded-lg border border-border px-4">
        {status.isPending && <p className="py-3 text-sm text-muted">Looking…</p>}
        {status.isError && <p className="py-3 text-sm text-danger">The tools did not load.</p>}
        {ok &&
          rows.map((r) => (
            <div key={r.tool} data-tool={r.tool} className="flex items-center gap-3 border-t border-border py-3 first:border-t-0">
              <div className="min-w-0 flex-1">
                <div className="text-sm text-fg">{r.tool}</div>
                <p className="text-xs text-muted">{FOR[r.tool]}</p>
              </div>
              <span className={`inline-flex items-center gap-1 text-xs ${r.installed ? 'text-success' : 'text-warning'}`}>
                {r.installed ? <Check size={12} /> : <TriangleAlert size={12} />}
                {r.installed ? 'Installed' : 'Missing'}
              </span>
            </div>
          ))}
      </div>
      {ok && gone.length > 0 && <Install missing={gone} />}
    </SettingsPage>
  );
}

/** What is missing, each with its command to copy, and where the desktop app can, one button that runs them in Terminal. */
function Install({ missing }: { missing: { tool: string; fix: string }[] }) {
  const install = shell()?.installTools;
  const [running, setRunning] = useState(false);
  const [error, setError] = useState('');
  // A tool landing tells the daemon nothing: while Terminal installs, the list is asked again every few seconds.
  useRead('daemon.status', {}, { refetchMs: running ? 3000 : undefined });
  return (
    <div className="flex flex-col gap-2 rounded-lg border border-border p-4">
      {missing.map((m) => (
        <button
          key={m.tool}
          type="button"
          onClick={() => void copy(m.fix, 'command')}
          className="flex w-fit items-center gap-2 text-xs text-fg"
        >
          <code className="font-mono">{m.fix}</code>
          <Copy size={11} className="text-muted" aria-label={`Copy ${m.fix}`} />
        </button>
      ))}
      {install && (
        <Button
          variant="primary"
          className="w-fit"
          disabled={running}
          onClick={() => {
            setError('');
            void install().then(
              (r) => (r.ok ? setRunning(true) : setError(r.message)),
              (e: unknown) => setError(sentence(classify(e))),
            );
          }}
        >
          {running ? 'Installing in Terminal…' : `Install ${missing.map((m) => m.tool).join(' and ')}`}
        </Button>
      )}
      {error && <p className="text-xs text-danger">{error}</p>}
    </div>
  );
}
