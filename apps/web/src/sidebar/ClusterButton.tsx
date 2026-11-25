// The status bar's bottom-left: this device and how many clusters it joined; its popover lists the clusters, to leave one.
import { Laptop, MoreHorizontal, Network, TriangleAlert } from 'lucide-react';
import { useState, type ReactNode, type RefObject } from 'react';
import { InCluster, useClusterName, useJoined, useOnline, type Joined } from '#src/connection/clusters.tsx';
import { MASTER } from '#src/settings/data/devices.ts';
import { openSettings } from '#src/settings/open.ts';
import { shell } from '#src/shell.ts';
import { Menu, MenuItem } from '#src/ui/Menu.tsx';
import { confirm } from '#src/ui/modal.tsx';
import { Popover } from '#src/ui/Popover.tsx';

/** `host` is this device's name; `lead` what goes before it (the status dot). */
export function ClusterButton({
  host,
  lead,
  bar,
  title,
}: {
  host: string;
  lead: ReactNode;
  bar: RefObject<HTMLDivElement | null>;
  title?: string;
}) {
  const joined = useJoined();
  const [open, setOpen] = useState(false);
  return (
    <Popover
      side="top"
      align="start"
      corner={bar}
      open={open}
      onOpenChange={setOpen}
      trigger={
        <button type="button" title={title} className="flex min-w-0 items-center gap-2 text-left hover:text-fg">
          {lead}
          <Laptop size={14} className="shrink-0" />
          <span className="truncate">{host}</span>
          {joined.length > 0 && (
            <span className="flex shrink-0 items-center gap-1" aria-label={`${joined.length} joined`}>
              <Network size={12} />
              {joined.length}
            </span>
          )}
        </button>
      }
    >
      <div className="flex w-72 flex-col gap-0.5">
        <button
          type="button"
          onClick={() => (setOpen(false), openSettings({ section: 'devices', device: MASTER }))}
          className="flex h-8 items-center gap-2 rounded-md px-2 text-left text-sm hover:bg-hover/30"
        >
          <OwnName />
        </button>
        {joined.map((j) => (
          <InCluster key={j.id} id={j.id}>
            <JoinedRow cluster={j} />
          </InCluster>
        ))}
      </div>
    </Popover>
  );
}

function OwnName() {
  const said = useClusterName();
  const Icon = (said?.machines ?? 1) > 1 ? Network : Laptop;
  return (
    <>
      <Icon size={14} className="shrink-0 text-muted" />
      <span className="truncate text-fg">{said?.name ?? 'This Mac'}</span>
      {said && <span className="shrink-0 rounded border border-border px-1 text-xs text-muted">This Mac</span>}
    </>
  );
}

function JoinedRow({ cluster }: { cluster: Joined }) {
  const said = useClusterName();
  const name = said?.name || cluster.name;
  const on = useOnline(cluster);
  const Icon = !on ? TriangleAlert : (said?.machines ?? 1) > 1 ? Network : Laptop;
  return (
    <div className="group flex h-8 items-center gap-2 rounded-md px-2 text-sm hover:bg-hover/30">
      <Icon size={14} className={`shrink-0 ${on ? 'text-muted' : 'text-warning'}`} />
      <span className={`min-w-0 flex-1 truncate ${on ? 'text-fg' : 'text-muted'}`}>{on ? name : `${name} · offline`}</span>
      <Menu
        side="bottom"
        align="start"
        trigger={
          <button type="button" aria-label={`${name}: more`} className="rounded text-muted hover:text-fg">
            <MoreHorizontal size={14} />
          </button>
        }
      >
        <MenuItem danger onClick={() => void leave(cluster.id, name)}>
          Leave
        </MenuItem>
      </Menu>
    </div>
  );
}

const leave = (id: string, name: string) =>
  confirm({
    title: `Leave ${name}?`,
    message: `This Mac stops showing ${name}'s work and running its agents.`,
    confirmText: 'Leave',
    danger: true,
    run: async () => {
      const r = await shell()?.leaveCluster?.(id);
      if (r && !r.ok) throw new Error(r.message);
    },
  });
