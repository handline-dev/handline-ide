// Whose a row is, when this Mac shows several clusters: another cluster's icon and name.
import { Laptop, Network, TriangleAlert } from 'lucide-react';
import { useCluster, useClusterName, useJoined, useOnline, type Joined } from '#src/connection/clusters.tsx';

/** Whose a workspace is: another cluster's icon (a device for one machine, a cluster for several) and name, or `<name> · offline` with
 * the warning mark while it does not answer. The icon goes first where there is no room. */
export function ClusterWord({ cluster }: { cluster: Joined }) {
  const said = useClusterName();
  const name = said?.name || cluster.name;
  const on = useOnline(cluster);
  const Icon = !on ? TriangleAlert : (said?.machines ?? 1) > 1 ? Network : Laptop;
  return (
    <span className="ml-auto flex min-w-0 shrink items-center gap-1 text-xs text-muted">
      <Icon size={12} className={`hidden shrink-0 @[12rem]:block ${on ? '' : 'text-warning'}`} />
      <span className="truncate">{on ? name : `${name} · offline`}</span>
    </span>
  );
}

/** The word for a row drawn in its cluster (InCluster): another cluster's, nothing for this Mac's. */
export function HereWord() {
  const id = useCluster();
  const j = useJoined().find((c) => c.id === id);
  return j ? <ClusterWord cluster={j} /> : null;
}
