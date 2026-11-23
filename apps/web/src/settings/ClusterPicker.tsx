// At the top of Settings' menu, with joined clusters: whose settings show, this Mac's cluster or a joined one.
import { ChevronDown, Laptop, Network, TriangleAlert } from 'lucide-react';
import { InCluster, useClusterName, useJoined, useOnline, type Joined } from '#src/connection/clusters.tsx';
import { Menu, MenuItem } from '#src/ui/Menu.tsx';

export function ClusterPicker({ value, onChange }: { value: string; onChange: (cluster: string) => void }) {
  const joined = useJoined();
  return (
    <div className="pb-2">
      <Menu
        trigger={
          <button
            type="button"
            className="flex h-8 w-full items-center gap-2 rounded-md border border-border bg-bg px-2 text-left text-sm text-fg hover:bg-hover/30"
          >
            <InCluster id={value}>
              <Name cluster={joined.find((j) => j.id === value)} />
            </InCluster>
            <ChevronDown size={14} className="ml-auto shrink-0 text-muted" />
          </button>
        }
      >
        <MenuItem checked={value === ''} onClick={() => onChange('')}>
          <span className="flex items-center gap-2">
            <Name />
          </span>
        </MenuItem>
        {joined.map((j) => (
          <MenuItem key={j.id} checked={value === j.id} onClick={() => onChange(j.id)}>
            <InCluster id={j.id}>
              <span className="flex items-center gap-2">
                <Name cluster={j} />
              </span>
            </InCluster>
          </MenuItem>
        ))}
      </Menu>
    </div>
  );
}

/** A cluster's icon (a device for one machine, a cluster for several, the warning mark offline) and name; drawn in that cluster. */
function Name({ cluster }: { cluster?: Joined }) {
  const said = useClusterName();
  return cluster ? (
    <JoinedName cluster={cluster} name={said} />
  ) : (
    <Shown name={said?.name || 'This Mac'} machines={said?.machines ?? 1} off={false} />
  );
}

function JoinedName({ cluster, name: said }: { cluster: Joined; name: ReturnType<typeof useClusterName> }) {
  return <Shown name={said?.name || cluster.name} machines={said?.machines ?? 1} off={!useOnline(cluster)} />;
}

function Shown({ name, machines, off }: { name: string; machines: number; off: boolean }) {
  const Icon = off ? TriangleAlert : machines > 1 ? Network : Laptop;
  return (
    <>
      <Icon size={14} className={`shrink-0 ${off ? 'text-warning' : 'text-muted'}`} />
      <span className="truncate">{off ? `${name} · offline` : name}</span>
    </>
  );
}
