// Under the sidebar's agents while setup is closed and not finished: how far it got, Continue (opens it where it stopped), and X to hide it.
import { ArrowRight, Settings2, X } from 'lucide-react';
import { openOnboarding, useOnboarding, useOnboardingOpen } from '#src/onboarding/data/progress.ts';
import { Button } from '#src/ui/Button.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';

export function SetupCard() {
  const ob = useOnboarding();
  const open = useOnboardingOpen();
  if (!ob.cardVisible || open) return null;
  return (
    <div className="flex flex-col gap-2 rounded-md border border-border bg-bg p-2.5">
      <div className="flex items-center gap-1.5">
        <Settings2 size={13} className="text-accent" />
        <span className="flex-1 text-xs font-medium text-fg">Finish setting up</span>
        <IconButton label="Hide" onClick={ob.hideCard}>
          <X size={12} />
        </IconButton>
      </div>
      <div className="flex items-center gap-2">
        <div className="h-1 flex-1 overflow-hidden rounded-full bg-border">
          <div className="h-full rounded-full bg-accent" style={{ width: `${(ob.doneCount / ob.total) * 100}%` }} />
        </div>
        <span className="text-xs text-muted tabular-nums">
          {ob.doneCount} of {ob.total}
        </span>
      </div>
      <Button variant="primary" className="h-7 justify-center" onClick={() => openOnboarding(ob.resumeStep)}>
        Continue
        <ArrowRight size={12} />
      </Button>
    </div>
  );
}
