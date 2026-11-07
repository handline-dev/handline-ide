// An agent's terminal in a window of its own (#terminal=<agent>), opened from the chat. The desktop shell lets this hash open
// (apps/electron/src/debug-window.ts).
import { useEffect } from 'react';
import { TerminalTab } from '#src/tools/TerminalTab.tsx';

const HASH = '#terminal=';

/** The agent whose terminal this window shows, or null for the app itself. */
export const terminalWindowAgent = (hash: string): string | null =>
  hash.startsWith(HASH) ? decodeURIComponent(hash.slice(HASH.length)) : null;

export function openTerminalWindow(agent: string) {
  window.open(`${location.pathname}${location.search}${HASH}${encodeURIComponent(agent)}`, `terminal:${agent}`, 'width=960,height=640');
}

export function TerminalWindow({ agent }: { agent: string }) {
  useEffect(() => void (document.title = `${agent} terminal`), [agent]);
  return (
    <div className="flex h-full flex-col">
      <TerminalTab agent={agent} />
    </div>
  );
}
