// Settings > Tools: what agents run on, installed or missing, and Install on what is missing.
import { useStatus } from '@handline/app-data';
import { Check, TriangleAlert } from 'lucide-react';
import { CodexLogin } from '#src/settings/AiLogin.tsx';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { AI_VENDORS, BASE_TOOLS } from '#src/onboarding/data/steps.ts';
import { Button } from '#src/ui/Button.tsx';
import { canInstall, InstallButton, Installing } from '#src/install/install.tsx';

const FOR: Record<string, string> = {
  git: 'Version control. Every agent works on its own branch.',
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
            <div key={r.tool} data-tool={r.tool} className="flex flex-wrap items-center gap-3 border-t border-border py-3 first:border-t-0">
              <div className="min-w-0 flex-1">
                <div className="text-sm text-fg">{r.tool}</div>
                <p className="text-xs text-muted">{FOR[r.tool]}</p>
              </div>
              <span className={`inline-flex items-center gap-1 text-xs ${r.installed ? 'text-success' : 'text-warning'}`}>
                {r.installed ? <Check size={12} /> : <TriangleAlert size={12} />}
                {r.installed ? 'Installed' : 'Missing'}
              </span>
              {!r.installed && canInstall(r.tool) && (
                <InstallButton tool={r.tool} size="sm">
                  Install
                </InstallButton>
              )}
              <Installing tool={r.tool} />
              {r.tool === 'codex' && r.installed && <CodexLogin />}
            </div>
          ))}
      </div>
    </SettingsPage>
  );
}
