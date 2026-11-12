// The setup modal: five steps, each one button; it opens by itself on first launch and Continue on the sidebar card reopens it where it stopped.
import { Dialog } from '@base-ui/react/dialog';
import { Check } from 'lucide-react';
import { useWindowLayer } from '#src/layout/windowLayer.tsx';
import { closeOnboarding, useOnboarding, useOnboardingOpen } from '#src/onboarding/data/progress.ts';
import { STEPS, type StepId } from '#src/onboarding/data/steps.ts';
import { AgentStep } from '#src/onboarding/AgentStep.tsx';
import { AiStep, NotifyStep, ProjectStep, ToolsStep } from '#src/onboarding/Steps.tsx';
import { NavContext, type Nav } from '#src/onboarding/StepView.tsx';
import { useBackCloses } from '#src/ui/backCloses.ts';
import { useNarrow } from '#src/ui/narrow.ts';

export const STEP_NAME: Record<StepId, { title: string; note: string }> = {
  ai: { title: 'AI tools', note: 'Claude Code, Codex' },
  tools: { title: 'Git & tmux', note: 'git and tmux' },
  notify: { title: 'Notifications', note: 'When agents need you' },
  project: { title: 'Project', note: 'What agents work on' },
  agent: { title: 'First agent', note: 'Give it a task' },
};

/** Mounted once at the root: it opens the modal on first launch, so it runs whether the modal is open or not. */
export function Onboarding() {
  const ob = useOnboarding();
  const open = useOnboardingOpen();
  const narrow = useNarrow();
  return (
    <Dialog.Root open={open} onOpenChange={(o) => !o && closeOnboarding()}>
      <Dialog.Portal>
        <Layer open={open} step={ob.step} goTo={ob.goTo} />
        <Dialog.Backdrop className="fixed inset-0 z-50 bg-fg/40 transition-opacity duration-150 data-[ending-style]:opacity-0 data-[starting-style]:opacity-0" />
        <Dialog.Viewport className={`fixed inset-0 z-50 flex items-center justify-center ${narrow ? '' : 'p-4'}`}>
          <Body ob={ob} />
        </Dialog.Viewport>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

/** Back goes one step back, and closes it on the first step. */
function Layer({ open, step, goTo }: { open: boolean; step: number; goTo: (step: number) => void }) {
  useWindowLayer(open);
  useBackCloses(open, () => (step > 0 ? goTo(step - 1) : void closeOnboarding()));
  return null;
}

function Body({ ob }: { ob: ReturnType<typeof useOnboarding> }) {
  const narrow = useNarrow();
  const index = ob.step;
  const id = STEPS[index]!;
  const last = index === STEPS.length - 1;
  const nav: Nav = {
    index,
    back: index > 0 ? () => ob.goTo(index - 1) : null,
    skip: last ? null : () => (id === 'notify' && ob.skipNotify(), ob.goTo(index + 1)),
    next: () => (last ? closeOnboarding() : ob.goTo(index + 1)),
  };
  return (
    <Dialog.Popup
      style={narrow ? undefined : { width: 940, height: 600 }}
      className={`flex bg-bg text-fg outline-none transition duration-150 data-[ending-style]:opacity-0 data-[starting-style]:opacity-0 ${
        narrow ? 'size-full flex-col' : 'max-h-full max-w-full overflow-hidden rounded-lg border border-border shadow-popover'
      }`}
    >
      {!narrow && <Rail ob={ob} />}
      <NavContext.Provider value={nav}>
        {id === 'ai' && <AiStep />}
        {id === 'tools' && <ToolsStep />}
        {id === 'notify' && <NotifyStep />}
        {id === 'project' && <ProjectStep />}
        {id === 'agent' && <AgentStep />}
      </NavContext.Provider>
    </Dialog.Popup>
  );
}

/** The steps on the left: done ones ticked, the current one marked; any can be clicked. */
function Rail({ ob }: { ob: ReturnType<typeof useOnboarding> }) {
  return (
    <div className="flex w-56 shrink-0 flex-col gap-4 border-r border-border bg-surface p-4">
      <div className="flex flex-col gap-0.5">
        <span className="text-sm font-medium text-fg">Set up Handline</span>
        <span className="text-xs text-muted">About 2 minutes. Stop any time.</span>
      </div>
      <ol className="flex flex-col gap-1">
        {ob.steps.map((s, i) => (
          <li key={s.id}>
            <button
              type="button"
              aria-current={i === ob.step ? 'step' : undefined}
              onClick={() => ob.goTo(i)}
              className={`flex w-full items-start gap-2.5 rounded-md px-2 py-1.5 text-left ${i === ob.step ? 'bg-selected' : 'hover:bg-hover/40'}`}
            >
              {s.done ? (
                <span className="mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full bg-success text-bg">
                  <Check size={10} strokeWidth={3} />
                </span>
              ) : (
                <span className="mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border border-border text-xs text-muted">
                  {i + 1}
                </span>
              )}
              <span className="flex min-w-0 flex-col">
                <span className="text-xs font-medium text-fg">{STEP_NAME[s.id].title}</span>
                <span className="truncate text-xs text-muted">{STEP_NAME[s.id].note}</span>
              </span>
            </button>
          </li>
        ))}
      </ol>
      <p className="mt-auto text-xs text-muted">Everything here is also in Settings, with more options.</p>
    </div>
  );
}
