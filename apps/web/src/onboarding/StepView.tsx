// One onboarding step's page (wide and narrow) and the moves between steps it offers.
import { Dialog } from '@base-ui/react/dialog';
import { ArrowLeft, X } from 'lucide-react';
import { createContext, useContext, useSyncExternalStore, type ReactNode } from 'react';
import { closeOnboarding } from '#src/onboarding/data/progress.ts';
import { STEPS } from '#src/onboarding/data/steps.ts';
import { Button } from '#src/ui/Button.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';

const NARROW = '(max-width: 759px)';
const subscribeNarrow = (l: () => void) => {
  const m = matchMedia(NARROW);
  m.addEventListener('change', l);
  return () => m.removeEventListener('change', l);
};
export const useNarrow = () => useSyncExternalStore(subscribeNarrow, () => matchMedia(NARROW).matches);

/** The primary button of a step: its words, and what it does. */
export type Primary = { label: ReactNode; onClick: () => void; disabled?: boolean };

export type Nav = { index: number; back: (() => void) | null; skip: (() => void) | null; next: () => void };
export const NavContext = createContext<Nav | null>(null);
/** The step being shown, and moves between steps. */
export const useStepNav = () => {
  const nav = useContext(NavContext);
  if (!nav) throw new Error('useStepNav outside onboarding');
  return nav;
};

/** One step's page: its title and line, the body, and the footer (Back, Skip, the step's one button). */
export function StepView({ title, line, primary, children }: { title: string; line: ReactNode; primary: Primary; children?: ReactNode }) {
  const narrow = useNarrow();
  const nav = useStepNav();
  const later = (
    <button type="button" onClick={closeOnboarding} className="text-xs text-muted hover:text-fg">
      {narrow ? 'Later' : 'Finish later'}
    </button>
  );
  const main = (
    <Button variant="primary" className={narrow ? 'h-9 flex-1 justify-center' : ''} disabled={primary.disabled} onClick={primary.onClick}>
      {primary.label}
    </Button>
  );
  const skip = nav.skip && (
    <Button variant="ghost" onClick={nav.skip}>
      Skip
    </Button>
  );
  if (narrow)
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        <div className="flex items-center gap-2 border-b border-border px-3 py-2.5">
          {nav.back ? (
            <IconButton label="Back" onClick={nav.back}>
              <ArrowLeft size={16} />
            </IconButton>
          ) : (
            <IconButton label="Close" onClick={closeOnboarding}>
              <X size={16} />
            </IconButton>
          )}
          <Dialog.Title className="min-w-0 flex-1 truncate text-center text-sm font-semibold">{title}</Dialog.Title>
          {later}
        </div>
        <div className="flex gap-1 px-4 pt-3" aria-label={`Step ${nav.index + 1} of ${STEPS.length}`}>
          {STEPS.map((s, i) => (
            <span key={s} className={`h-0.5 flex-1 rounded-full ${i <= nav.index ? 'bg-accent' : 'bg-border'}`} />
          ))}
        </div>
        <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-4 py-4 text-sm">
          <p className="text-muted">{line}</p>
          {children}
        </div>
        <div className="flex items-center gap-3 border-t border-border p-4">
          {skip}
          {main}
        </div>
      </div>
    );
  return (
    <div className="flex min-w-0 flex-1 flex-col">
      <div className="flex items-center justify-between px-6 pt-4">
        <span className="text-xs text-muted">
          Step {nav.index + 1} of {STEPS.length}
        </span>
        <span className="flex items-center gap-2">
          {later}
          <IconButton label="Close" onClick={closeOnboarding}>
            <X size={14} />
          </IconButton>
        </span>
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-6 pt-2 pb-4 text-sm">
        <div className="flex flex-col gap-1">
          <Dialog.Title className="text-lg font-semibold">{title}</Dialog.Title>
          <Dialog.Description className="text-muted">{line}</Dialog.Description>
        </div>
        {children}
      </div>
      <div className="flex items-center gap-2 border-t border-border px-6 py-3">
        {nav.back && <Button onClick={nav.back}>Back</Button>}
        <span className="flex-1" />
        {skip}
        {main}
      </div>
    </div>
  );
}
