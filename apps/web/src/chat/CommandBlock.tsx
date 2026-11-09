// A command fence in a chat line (app-core commandOf) runs in a window of the agent's terminal host, opened as a tab beside the chat.
import { commandRisk, runLine } from '@handline/app-core';
import { useWrite } from '@handline/app-data';
import type { TranscriptItem } from '@handline/client';
import { Play, RotateCw, SquareTerminal } from 'lucide-react';
import { createContext, useState } from 'react';
import { useNow } from '#src/format/time.ts';
import { shellOf } from '#src/layout/kinds.tsx';
import { useOpenTab } from '#src/layout/openTab.tsx';
import { Button } from '#src/ui/Button.tsx';
import { confirm } from '#src/ui/modal.tsx';
import { toast } from '#src/ui/toast.tsx';

type Run = NonNullable<TranscriptItem['runs']>[number];

/** Whose command fences a Markdown draws: the agent, the message (`<id>`) or reply item (`r<id>`) they are in, and their runs.
 * None outside a chat line: a fence there is plain code. */
export type CommandsOfValue = { agent: string; owner: string; runs?: Run[] } | null;
export const CommandsOf = createContext<CommandsOfValue>(null);

/** n: the fence's 0-based place among its text's code fences, or BANG_FROM and up for an inline `! ` span; its block is `${owner}:${n}`.
 * `inline` sits beside an inline span instead of under a fence. */
export function CommandRunBar({
  of,
  command,
  n,
  inline,
}: {
  of: NonNullable<CommandsOfValue>;
  command: string;
  n: number;
  inline?: boolean;
}) {
  const block = `${of.owner}:${n}`;
  const run = of.runs?.find((r) => r.block === block);
  const open = useWrite('shell.open', of.agent);
  const openTab = useOpenTab();
  const now = useNow(run?.state === 'running' ? 1000 : null);
  // From the click until the chat shows the new run (or the call fails): a second click would start it again.
  const [sent, setSent] = useState<string | null>(null);
  const waiting = sent !== null && (run?.started_at ?? '') === sent;
  const start = async () => {
    const risk = commandRisk(command);
    if (risk && !(await confirm({ title: 'Run this command?', message: risk, confirmText: 'Run it', danger: true }))) return;
    setSent(run?.started_at ?? '');
    let terminal: string;
    try {
      terminal = (await open.mutateAsync({ agent: of.agent, command, block })).terminal;
    } catch (e) {
      setSent(null);
      return toast(`It did not run: ${e instanceof Error ? e.message : String(e)}`, { tone: 'warning' });
    }
    openTab(shellOf(terminal), 'side');
  };
  const Wrap = inline ? 'span' : 'div';
  return (
    <Wrap className={`${inline ? 'ml-1 inline-flex align-middle' : 'flex px-2 pt-1 pb-0.5'} items-center gap-2 text-xs text-muted`}>
      {run && (
        <span role="status" className={run.state === 'failed' ? 'text-danger' : run.state === 'done' ? 'text-success' : ''}>
          {runLine(run, now)}
        </span>
      )}
      <span className={`flex gap-1 ${inline ? '' : 'ml-auto'}`}>
        {run?.state === 'running' && !waiting ? (
          <Button variant="ghost" size="sm" onClick={() => openTab(shellOf(run.terminal), 'side')}>
            <SquareTerminal size={14} />
            Open terminal
          </Button>
        ) : (
          <Button variant="ghost" size="sm" disabled={waiting} onClick={() => void start()}>
            {run ? <RotateCw size={14} /> : <Play size={14} />}
            {run ? 'Run again' : 'Run in terminal'}
          </Button>
        )}
      </span>
    </Wrap>
  );
}
