// Whose a row is, when this Mac shows several clusters: another cluster's icon and name.
import { Laptop, Network, TriangleAlert } from 'lucide-react';
import { PlaceWord } from '#src/connection/PlaceWord.tsx';
import { useCluster, useClusterName, useJoined, useOnline, type Joined } from '#src/connection/clusters.tsx';

/** Whose a workspace is: another cluster's icon (a device for one machine, a cluster for several) and name, or `<name> · offline` with
 * the warning mark while it does not answer. The icon goes first where there is no room. */
export function ClusterWord({ cluster }: { cluster: Joined }) {
  const said = useClusterName();
  const name = said?.name || cluster.name;
  const on = useOnline(cluster);
  const Icon = !on ? TriangleAlert : (said?.machines ?? 1) > 1 ? Network : Laptop;
  return <PlaceWord icon={Icon} name={on ? name : `${name} · offline`} warn={!on} className="ml-auto" iconClass="hidden @[12rem]:block" />;
}

/** The word for a row drawn in its cluster (InCluster): another cluster's, nothing for this Mac's. */
export function HereWord() {
  const id = useCluster();
  const j = useJoined().find((c) => c.id === id);
  return j ? <ClusterWord cluster={j} /> : null;
}
