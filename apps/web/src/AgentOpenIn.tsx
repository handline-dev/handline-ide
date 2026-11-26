// The apps of an agent's "Open in" menu: its folder in VS Code or Terminal, or its Claude Code session in Terminal. The desktop shell
// opens them (openIn), on a Mac, for a folder that is on this Mac; anywhere else the menu keeps only its copy items.
import { folderIsHere, OPEN_IN_APPS, type OpenInApp } from '@handline/app-core';
import { Hooks, useAgentDetail, useAgents } from '@handline/app-data';
import { useThisDevice } from '@handline/app-react';
import { Bot, Code, SquareTerminal } from 'lucide-react';
import { useSyncExternalStore, type ReactNode } from 'react';
import { useCluster } from '#src/connection/clusters.tsx';
import { shell } from '#src/shell.ts';
import { MenuItem, MenuSeparator } from '#src/ui/Menu.tsx';
import { toast } from '#src/ui/toast.tsx';

const NONE: OpenInApp[] = [];
let found = NONE;
let asked = false;
const waiting = new Set<() => void>();

/** Asks the shell which apps this machine has, once per page: the answer does not change while the app runs. */
function subscribe(changed: () => void) {
  waiting.add(changed);
  if (!asked) {
    asked = true;
    void shell()
      ?.openInApps?.()
      .then((r) => {
        if (!r.ok || !r.apps.length) return;
        found = r.apps;
        waiting.forEach((f) => f());
      })
      .catch(() => undefined);
  }
  return () => void waiting.delete(changed);
}

/** The apps the shell can open an agent in; none in a browser, off a Mac, or before the shell has answered. */
export const useOpenInApps = (): OpenInApp[] => useSyncExternalStore(subscribe, () => found);

const ICONS: Record<OpenInApp, ReactNode> = {
  vscode: <Code size={14} />,
  terminal: <SquareTerminal size={14} />,
  claude: <Bot size={14} />,
};

/** The apps as menu items, then a separator; nothing for an agent whose folder is not on this Mac (it lives on another machine, or in a
 *  cluster this Mac only joined) or that has no worktree, where the shell would only answer that it found none. */
export function OpenInItems({ agent, apps }: { agent: string; apps: OpenInApp[] }) {
  const cluster = useCluster();
  const device = useThisDevice();
  const nodes = Hooks.useNodeStatus().data?.nodes ?? [];
  const agentNode = useAgents().data?.agents.find((a) => a.key === agent)?.node;
  const detail = useAgentDetail(agent).data;
  if (cluster !== '' || !detail?.worktree || !folderIsHere({ device, nodes, agentNode })) return null;
  const open = async (app: OpenInApp) => {
    const r = await shell()?.openIn?.({ app, agent });
    if (r && !r.ok) toast(r.message, { tone: 'warning' });
  };
  return (
    <>
      {OPEN_IN_APPS.filter((a) => apps.includes(a.app)).map((a) => (
        // The session is opened by attaching to the agent's terminal: with none running there is nothing to open.
        <MenuItem key={a.app} icon={ICONS[a.app]} disabled={a.app === 'claude' && !detail.terminal} onClick={() => void open(a.app)}>
          {a.label}
        </MenuItem>
      ))}
      <MenuSeparator />
    </>
  );
}
