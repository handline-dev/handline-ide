// Settings > Onboarding: the five setup items, done or not by what is true on this machine, each leading to its own Settings page, and Continue,
// which opens the setup where it was left.
import { hooks, useAgents, useRepos, useStatus } from '@handline/app-data';
import { ArrowRight, Check } from 'lucide-react';
import { BASE_TOOLS, STEPS, type StepId } from '#src/onboarding/data/steps.ts';
import { openOnboarding, useAiTools, useOnboarding } from '#src/onboarding/data/progress.ts';
import { Block } from '#src/settings/Meter.tsx';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { closeSettings, type Section } from '#src/settings/open.ts';
import { Button } from '#src/ui/Button.tsx';

const TITLE: Record<StepId, string> = {
  ai: 'AI tools',
  tools: 'Git & tmux',
  notify: 'Notifications',
  project: 'Project',
  agent: 'First agent',
};
const PAGE: Partial<Record<StepId, { section: Section; word: string }>> = {
  ai: { section: 'accounts', word: 'Accounts' },
  tools: { section: 'tools', word: 'Tools' },
  notify: { section: 'notifications', word: 'Notifications' },
  project: { section: 'projects', word: 'Projects' },
};

export function OnboardingTab({ go }: { go: (s: Section) => void }) {
  const o = useOnboarding();
  const ai = useAiTools();
  const missing = (useStatus().data?.missing_tools ?? []).filter((m) => BASE_TOOLS.includes(m.tool));
  const repos = useRepos().data?.repos.length ?? 0;
  const found = hooks.useReposRecent().data?.folders.length ?? 0;
  const agents = useAgents().data?.agents.length ?? 0;
  const ready = ai.rows.filter((r) => r.state === 'ready');
  const loaded = o.loaded && !ai.loading;
  const detail: Record<StepId, string> = {
    ai: ready.length
      ? ready.map((r) => `${r.vendor === 'codex' ? 'Codex' : 'Claude Code'}${r.email ? ` · ${r.email}` : ''}`).join(', ')
      : 'No AI tool is logged in yet.',
    tools: missing.length ? `${missing.map((m) => m.tool).join(' and ')} missing` : 'git and tmux are installed.',
    notify: o.done.notify ? (o.notify === 'granted' ? 'On' : 'Skipped') : 'Not set',
    project: repos ? `${repos} ${repos === 1 ? 'project' : 'projects'}` : `None yet${found ? ` · ${found} found on this Mac` : ''}`,
    agent: agents ? `${agents} ${agents === 1 ? 'agent' : 'agents'}` : 'Not started',
  };
  return (
    <SettingsPage
      keep="settings-onboarding"
      title="Onboarding"
      note="The quick setup that opened the first time. Each item has its own Settings page with every option: Open takes you there."
    >
      <Block title={o.allDone ? 'All set' : `${o.doneCount} of ${o.total} done`}>
        <div className="flex gap-1.5" aria-hidden>
          {STEPS.map((id) => (
            <div key={id} className={`h-1.5 flex-1 rounded-full ${o.done[id] ? 'bg-accent' : 'bg-surface'}`} />
          ))}
        </div>
        <ol className="flex flex-col">
          {STEPS.map((id, i) => (
            <li key={id} data-step={id} className="flex items-center gap-3 border-t border-border py-2.5 first:border-t-0">
              <span
                className={`flex size-5 shrink-0 items-center justify-center rounded-full text-xs ${o.done[id] ? 'bg-success text-bg' : 'border border-border text-muted'}`}
              >
                {o.done[id] ? (
                  <Check size={12} role="img" aria-label="Done" />
                ) : (
                  <>
                    <span className="sr-only">Not done: </span>
                    {i + 1}
                  </>
                )}
              </span>
              <div className="min-w-0 flex-1">
                <div className="text-sm text-fg">{TITLE[id]}</div>
                <div className="truncate text-xs text-muted">{loaded ? detail[id] : '…'}</div>
              </div>
              {PAGE[id] && (
                <button type="button" className="shrink-0 text-xs text-accent hover:underline" onClick={() => go(PAGE[id]!.section)}>
                  Open {PAGE[id]!.word}
                  <ArrowRight size={12} className="ml-1 inline" />
                </button>
              )}
            </li>
          ))}
        </ol>
      </Block>
      {!o.allDone && (
        <div className="flex flex-wrap items-center gap-3">
          <Button
            variant="primary"
            onClick={() => {
              closeSettings();
              openOnboarding(o.resumeStep);
            }}
          >
            Continue onboarding
            <ArrowRight size={14} />
          </Button>
          <span className="text-xs text-muted">Opens where you left off: {TITLE[STEPS[o.resumeStep]!]}.</span>
        </div>
      )}
    </SettingsPage>
  );
}
