// This Mac's page: what Handline does about the agents' heavy runs once the Mac is slow. The default only tells; the others are the
// person's to turn on.
import { RUN_POLICIES, RUN_POLICY_LINE, RUN_POLICY_WORD } from '@handline/app-core';
import { Timer } from 'lucide-react';
import type { ReactNode } from 'react';
import { Block, Setting } from '#src/settings/Meter.tsx';
import type { useMachine } from '#src/settings/data/machine.ts';
import { NumberStepper } from '#src/ui/NumberStepper.tsx';
import { Segmented } from '#src/ui/Segmented.tsx';

export function HeavyRuns({
  m,
  send,
  why,
}: {
  m: ReturnType<typeof useMachine>;
  send: (key: string, go: () => Promise<unknown>) => Promise<unknown>;
  why: (key: string) => ReactNode;
}) {
  return (
    <Block
      title="Heavy runs"
      icon={<Timer size={16} />}
      note="A heavy run is a test or build command that usually takes a minute or more. The Mac is slow once such runs take much longer than they usually do; until then Handline leaves them alone."
    >
      <Setting title="When the Mac gets slow" note={RUN_POLICY_LINE[m.runPolicy]}>
        <div className={m.pending === 'heavy_runs_policy' ? 'opacity-50' : ''}>
          <Segmented
            label="When the Mac gets slow"
            value={m.runPolicy}
            options={RUN_POLICIES}
            word={(p) => RUN_POLICY_WORD[p]}
            onChange={(p) => m.pending === null && void send('heavy_runs_policy', () => m.setRunPolicy(p)).catch(() => {})}
          />
        </div>
      </Setting>
      {why('heavy_runs_policy')}
      {m.runPolicy === 'fixed' && (
        <Setting title="Heavy runs at once" note="Auto is half of this Mac's cores.">
          <NumberStepper
            value={m.runsAtOnce}
            max={64}
            label="Heavy runs at once"
            zeroWord="Auto"
            disabled={m.pending !== null}
            onCommit={(n) => send('heavy_runs_at_once', () => m.setRunsAtOnce(n))}
          />
        </Setting>
      )}
      {why('heavy_runs_at_once')}
    </Block>
  );
}
