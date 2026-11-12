// The Background service card: whether it runs, and Restart, Stop and Start.
import { CircleAlert, RotateCw, Square } from 'lucide-react';
import { useState } from 'react';
import { ago } from '#src/format/time.ts';
import { Block, Setting } from '#src/settings/Meter.tsx';
import type { useService } from '#src/settings/data/service.ts';
import { Button } from '#src/ui/Button.tsx';
import { confirm } from '#src/ui/modal.tsx';

type Service = NonNullable<ReturnType<typeof useService>>;

const agentsWorking = (n: number) => (n === 1 ? '1 agent is working' : `${n} agents are working`);

function stateText(v: Service) {
  if (v.state === 'starting') return 'Starting…';
  if (v.state !== 'running') return v.reason ? `Stopped: ${v.reason}` : 'Stopped';
  const up = v.startedAt ? ago(v.startedAt) : '';
  return `Running${up === 'now' ? ' · just started' : up ? ` · up ${up}` : ''}${v.thisBuild === false ? ' · an older build' : ''}`;
}

export function ServiceBlock({ service: v, working: busy }: { service: Service; working: number }) {
  const [refused, setRefused] = useState<string | null>(null);
  const go = (key: 'restart' | 'stop' | 'start') => {
    setRefused(null);
    v[key]().catch((e: unknown) => setRefused(typeof e === 'string' ? e : 'It did not work.'));
  };
  const state = stateText(v);
  const down = v.state === 'stopped' || v.state === 'failed';
  const off = v.pending !== null || v.state === 'starting';
  const restart = async () => {
    if (
      busy > 0 &&
      !(await confirm({
        title: 'Restart the background service?',
        message: `${agentsWorking(busy)}. They go on after it is back.`,
        confirmText: 'Restart',
      }))
    )
      return;
    go('restart');
  };
  const stop = async () => {
    const yes = await confirm({
      title: 'Stop the background service?',
      message: `${busy > 0 ? `${agentsWorking(busy)}. ` : ''}Nothing new starts while it is stopped, and this window stops updating. Agents' terminals keep running. Press Start here, or open Handline again, to bring it back.`,
      confirmText: 'Stop',
      danger: true,
    });
    if (yes) go('stop');
  };
  return (
    <Block title="Background service" note="The part of Handline that runs your agents. It starts and recovers by itself.">
      <Setting
        title={
          <span className="inline-flex items-center gap-2">
            <span
              className={`size-2 rounded-full ${v.state === 'running' ? 'bg-accent' : v.state === 'starting' ? 'bg-warning' : 'bg-danger'}`}
              aria-hidden
            />
            {state}
          </span>
        }
        note={
          down ? 'Press Start, or open Handline again, to bring it back.' : 'Restart is for when something looks stuck. Stop asks first.'
        }
      >
        <div className="flex gap-2">
          {down ? (
            <Button variant="primary" disabled={off} onClick={() => go('start')}>
              Start
            </Button>
          ) : (
            <>
              <Button disabled={off || !v.canRestart} onClick={() => void restart()}>
                <RotateCw size={13} />
                Restart
              </Button>
              <Button disabled={off} onClick={() => void stop()}>
                <Square size={13} />
                Stop…
              </Button>
            </>
          )}
        </div>
      </Setting>
      {refused && (
        <p className="flex items-center gap-1 text-xs text-danger" role="alert">
          <CircleAlert size={12} />
          {refused}
        </p>
      )}
    </Block>
  );
}
