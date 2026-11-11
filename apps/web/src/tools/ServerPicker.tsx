import { useAgents, useNameOf } from '@handline/app-data';
import type { AgentView } from '@handline/client';
import { Server } from 'lucide-react';
import { ago, useNow } from '#src/format/time.ts';
import { IconButton } from '#src/ui/IconButton.tsx';
import { Menu, MenuItem } from '#src/ui/Menu.tsx';

type AgentServer = NonNullable<AgentView['servers']>[number];

/** The page a server answers at: one listening on every interface or on this machine is localhost. */
export function serverUrl(s: Pick<AgentServer, 'port' | 'address'>): string {
  const host = ['127.0.0.1', '0.0.0.0', '::', ''].includes(s.address)
    ? 'localhost'
    : s.address.includes(':')
      ? `[${s.address}]`
      : s.address;
  return `http://${host}:${s.port}`;
}

const origin = (url: string): string | null => {
  try {
    return new URL(url).origin;
  } catch {
    return null;
  }
};

/** Beside the address bar: the servers the agent's processes listen on, lowest port first; picking one opens it here. */
export function ServerPicker({ agent, current, onPick }: { agent: string; current: string; onPick: (url: string) => void }) {
  const word = useNameOf()(agent);
  // One on a scoped link-local address (fe80::1%en0) has no URL a browser opens.
  const servers = (useAgents().data?.agents.find((a) => a.key === agent)?.servers ?? []).filter((s) => !s.address.includes('%'));
  const now = useNow();
  const at = (s: AgentServer) => origin(current) === origin(serverUrl(s));
  return (
    <Menu
      trigger={
        // Kept when there is none, so the address bar does not move as servers come and go.
        <IconButton label={servers.length ? `${word}'s servers` : `${word} runs no server`} disabled={!servers.length}>
          <Server size={14} />
        </IconButton>
      }
    >
      {servers.map((s) => (
        <MenuItem key={`${s.pid}:${s.port}`} checked={at(s)} onClick={() => onPick(serverUrl(s))}>
          <span className="flex min-w-0 items-baseline gap-2">
            <span className="shrink-0 tabular-nums">{serverUrl(s).replace('http://', '')}</span>
            <span className="max-w-72 truncate text-xs text-muted" title={s.command}>
              {s.command}
            </span>
            <span className="ml-auto shrink-0 pl-3 text-xs text-muted tabular-nums">{ago(s.since, now)}</span>
          </span>
        </MenuItem>
      ))}
    </Menu>
  );
}
