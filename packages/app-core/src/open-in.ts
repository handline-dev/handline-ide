// An agent's "Open in" menu on the desktop app: the apps the shell can open its folder in, and whether that folder is on this Mac. Pure.

export type OpenInApp = 'vscode' | 'terminal' | 'claude';

/** The menu's words for each app, in the menu's order. */
export const OPEN_IN_APPS: readonly { app: OpenInApp; label: string }[] = [
  { app: 'vscode', label: 'VS Code' },
  { app: 'terminal', label: 'Terminal' },
  { app: 'claude', label: 'Claude Code session' },
];

/** Whether an agent's folder is on the Mac this app runs on, where the shell can open it. `device` is the paired device this app connects
 *  as (none on the master's own Mac, which connects as its person), `nodes` the machines node.status lists, and `agentNode` the machine
 *  the agent's last session opened on (none before its first, which the master's machine holds).
 *  On the master's own Mac the agents of the daemon's machine are here; on a lent Mac, the agents on its own node; any other paired
 *  device (a Mac that lends nothing) holds no agent's folder. */
export function folderIsHere(a: {
  device: string | undefined;
  nodes: readonly { id: string; self: boolean; device?: string | null }[];
  agentNode: string | null | undefined;
}): boolean {
  if (!a.device) {
    const self = a.nodes.find((n) => n.self);
    return !a.agentNode || !self || a.agentNode === self.id;
  }
  const mine = a.nodes.find((n) => !n.self && n.device === a.device);
  return !!mine && a.agentNode === mine.id;
}
