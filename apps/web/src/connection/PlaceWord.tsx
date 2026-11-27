// Where something is: a cluster's or a machine's icon and name, in the sidebar's small muted look.
import { Hooks } from '@handline/app-data';
import { Laptop, type LucideIcon } from 'lucide-react';

/** An icon and a name; `warn` tints the icon. The icon goes first where there is no room. */
export function PlaceWord({
  icon: Icon,
  name,
  warn = false,
  className = '',
  iconClass = '',
}: {
  icon: LucideIcon;
  name: string;
  warn?: boolean;
  className?: string;
  iconClass?: string;
}) {
  return (
    <span className={`flex min-w-0 shrink items-center gap-1 text-xs text-muted ${className}`}>
      <Icon size={12} className={`shrink-0 ${iconClass} ${warn ? 'text-warning' : ''}`} />
      <span className="truncate">{name}</span>
    </span>
  );
}

/** The board's machines by node id, and whether it has more than one (one machine names none). */
export function useMachines() {
  const nodes = Hooks.useNodeStatus().data?.nodes ?? [];
  const byId = new Map(nodes.map((n) => [n.id, n]));
  return {
    many: nodes.length > 1,
    name: (id: string) => byId.get(id)?.name ?? 'another machine',
    self: (id: string) => !!byId.get(id)?.self,
  };
}

/** A machine of the board: its name, and "this Mac" on the one the app's daemon runs on. */
export function MachineWord({ id }: { id: string }) {
  const m = useMachines();
  return <PlaceWord icon={Laptop} name={m.self(id) ? `${m.name(id)} · this Mac` : m.name(id)} />;
}

/** The machines of a row, each labelled, only when the board has more than one. */
export function MachinesOf({ ids, lead }: { ids: readonly string[]; lead?: string }) {
  const m = useMachines();
  if (!m.many || !ids.length) return null;
  return (
    <span className="flex min-w-0 flex-wrap items-center gap-x-1.5 text-xs text-muted">
      {lead && <span className="shrink-0">{lead}</span>}
      {ids.map((id, i) => (
        <span key={id} className="flex shrink-0 items-center gap-1.5">
          {i > 0 && <span aria-hidden>·</span>}
          <MachineWord id={id} />
        </span>
      ))}
    </span>
  );
}
