// Joining another computer's cluster from this app, in one modal: the link (from that computer's Settings › Devices › Connect a device),
// then how this computer joins (Control from here: on Off; Run agents here: Run at once), then what came of it. A link the app was opened
// with (the link page's Open in the Handline app) starts at the second step.
import { why } from '@handline/app-data';
import { CircleCheck, Cpu, Monitor, TriangleAlert } from 'lucide-react';
import { useEffect, useState } from 'react';
import { shell } from '#src/shell.ts';
import { useAction } from '#src/ui/Button.tsx';
import { ChoiceCards } from '#src/ui/ChoiceCards.tsx';
import { Input } from '#src/ui/Field.tsx';
import { openModal } from '#src/ui/modal.tsx';
import { StepModal } from '#src/ui/StepModal.tsx';

type How = 'control' | 'run';
type Done = { name: string; warning?: string };

/** Join with a link, from its first step; `link` given (a link the app was opened with) starts at the choice, where nothing is picked
 * for the person: the link came from a page, so it joins only once they pick and press Join. */
export function joinWithLink(link?: string): Promise<void> {
  return openModal<boolean>((close) => <JoinFlow link={link ?? ''} close={() => close(true)} />).then(() => undefined);
}

function JoinFlow({ link: given, close }: { link: string; close: () => void }) {
  const [step, setStep] = useState(given ? 1 : 0);
  const [link, setLink] = useState(given);
  const [how, setHow] = useState<How | null>(null);
  const [done, setDone] = useState<Done | null>(null);
  const call = useAction();
  const join = () =>
    void call.go(async () => {
      const r = await shell()?.joinCluster?.({ link: link.trim(), ...(how === 'run' ? { lending: 'run' as const } : {}) });
      if (!r) throw new Error('only the desktop app joins with a link');
      if (!r.ok && r.code !== 'cannot_run') throw new Error(r.message);
      setDone(r.ok ? { name: r.name } : { name: '', warning: r.message });
      setStep(2);
    });
  const next = () => void (link.trim() && (call.reset(), setStep(1)));
  return (
    <StepModal
      title="Join with a link"
      description={step === 0 ? 'On the other computer, copy the link from Settings › Devices › Connect a device.' : undefined}
      size="md"
      onClose={close}
      step={step}
      steps={[
        {
          body: (
            <form onSubmit={(e) => (e.preventDefault(), next())} className="pb-1">
              <Input autoFocus value={link} onChange={(e) => setLink(e.target.value)} aria-label="Link" placeholder="Link" />
            </form>
          ),
          next: { label: 'Next', onClick: next, disabled: !link.trim() },
        },
        {
          body: (
            <div className="flex flex-col gap-3 pb-1">
              <ChoiceCards
                label="How this computer joins"
                value={how}
                onPick={setHow}
                cards={[
                  { value: 'control', icon: Monitor, label: 'Control from here' },
                  { value: 'run', icon: Cpu, label: 'Run agents here' },
                ]}
              />
              {!!call.error && (
                <p className="text-sm text-danger" role="alert">
                  {why(call.error)}
                </p>
              )}
            </div>
          ),
          back: { label: 'Back', onClick: () => (call.reset(), setStep(0)), disabled: call.busy },
          next: { label: 'Join', onClick: join, disabled: !how, busy: call.busy },
        },
        {
          finish: { icon: done?.warning ? TriangleAlert : CircleCheck, tone: done?.warning ? 'warning' : 'success' },
          body: done && (done.warning ?? `Joined ${done.name}`),
        },
      ]}
    />
  );
}

/** A link the app was opened with comes as a navigate to `join`. */
export function JoinLinks() {
  useEffect(() => shell()?.onNavigate?.((to) => void (to.screen === 'join' && to.id && joinWithLink(to.id))), []);
  return null;
}
