// A command an agent asked the founder to run, drawn like a comment where it was asked: the agent, the whole command, Run and Deny
// under it; once answered, one line saying how it ended. The task page and the agent's chat draw it the same.
import { commandSteps, runCommand, runOutcome, type RunOutcome } from '@handline/app-core';
import type { Proposal } from '@handline/client';
import { useRead } from '@handline/app-data';
import { useStore } from '@handline/app-react';
import { Ban, CircleCheck, CircleX, LoaderCircle, Minus } from 'lucide-react';
import { CopyButton } from '#src/chat/MessageActions.tsx';
import { clock } from '#src/format/time.ts';
import { ProposalAnswers } from '#src/requests/TurnQuick.tsx';
import { Ago } from '#src/ui/Ago.tsx';
import { Face, Name } from '#src/ui/Face.tsx';

const MARK: Record<RunOutcome['mark'], typeof Ban> = {
  ok: CircleCheck,
  fail: CircleX,
  denied: Ban,
  running: LoaderCircle,
  other: Minus,
};
const TONE: Record<RunOutcome['mark'], string> = {
  ok: 'text-success',
  fail: 'text-danger',
  denied: 'text-muted',
  running: 'text-muted animate-spin motion-reduce:animate-none',
  other: 'text-muted',
};

export function RunAsk({ p, who, at }: { p: Proposal; who: string; at: string }) {
  const command = runCommand(p);
  const end = runOutcome(p);
  return (
    <div data-proposal={p.id} className="flex gap-2.5 pb-3">
      <Face who={who} />
      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        <span className="flex flex-wrap items-baseline gap-x-2 text-sm">
          <Name who={who} className="font-medium" />
          <span className="text-xs text-muted">
            {end ? 'Asked you to run' : 'Asks you to run'} ·{' '}
            <time dateTime={at} title={clock(at)}>
              <Ago at={at} />
            </time>
          </span>
        </span>
        {end ? <Ended end={end} command={command} /> : <Waiting p={p} command={command} />}
      </div>
    </div>
  );
}

function Ended({ end, command }: { end: RunOutcome; command: string }) {
  const Mark = MARK[end.mark];
  return (
    <div className="flex min-w-0 flex-col gap-1 text-sm">
      <span className="flex items-center gap-2">
        <Mark size={14} className={`shrink-0 ${TONE[end.mark]}`} />
        <span className="text-fg">{end.word}</span>
      </span>
      <pre className="min-w-0 pl-5.5 font-mono text-xs leading-5 break-words whitespace-pre-wrap text-muted">{commandSteps(command)}</pre>
    </div>
  );
}

/** The machine a run starts on, the daemon's own, named only when this app is not on it. */
function useRunsOn(): string | null {
  const t = useStore().client.hello?.transport;
  const remote = !!t && t !== 'local'; // a paired device comes in over the network
  const mdns = useRead('connection.addresses', {}).data?.addresses.find((a) => a.kind === 'mdns')?.host;
  return remote && mdns ? mdns.replace(/\.local$/, '') : null;
}

function Waiting({ p, command }: { p: Proposal; command: string }) {
  const on = useRunsOn();
  return (
    <>
      <div className="relative min-w-0 rounded-md border border-border bg-surface">
        <pre className="py-2 pr-10 pl-3 font-mono text-xs leading-5 [overflow-wrap:anywhere] whitespace-pre-wrap text-fg">
          {commandSteps(command)}
        </pre>
        <span className="absolute top-1 right-1">
          <CopyButton text={command} />
        </span>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <ProposalAnswers id={p.id} runFirst />
        {p.run?.cwd && (
          <span className="min-w-0 font-mono text-xs break-all text-muted">{on ? `on ${on} · ${p.run.cwd}` : `in ${p.run.cwd}`}</span>
        )}
      </div>
    </>
  );
}
