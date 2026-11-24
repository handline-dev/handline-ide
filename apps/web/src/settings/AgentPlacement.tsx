// Settings > Devices, under the device rows: where each agent's next session starts (node.status placement), with Auto or Manual
// (agent_placement). Manual adds a device menu per agent; a pin is agent.set device, and Auto in the menu clears it.
import { Hooks, useNameOf, useWrite } from '@handline/app-data';
import { Bot, ChevronDown, Pin, PinOff, Weight } from 'lucide-react';
import { KIND_ICON } from '#src/settings/DeviceControls.tsx';
import type { DeviceRow } from '#src/settings/data/devices.ts';
import { Menu, MenuItem, MenuSeparator } from '#src/ui/Menu.tsx';
import { Segmented } from '#src/ui/Segmented.tsx';

const MODES = ['auto', 'manual'] as const;

/** Nothing on a single Mac, where every agent runs here. `rows` are the device rows above, for each node's icon. */
export function AgentPlacement({ rows }: { rows: DeviceRow[] }) {
  const status = Hooks.useNodeStatus().data;
  const nameOf = useNameOf();
  const settings = useWrite('settings.set');
  const pin = useWrite('agent.set');
  const placement = status?.placement;
  const nodes = status?.nodes ?? [];
  if (!placement || nodes.length < 2) return null;
  const manual = placement.mode === 'manual';
  const names = new Map(nodes.map((n) => [n.id, n.name]));
  const kinds = new Map(rows.flatMap((r) => (r.node ? [[r.node.id, r.kind ?? 'mac'] as const] : [])));
  const device = (id: string) => {
    const Icon = KIND_ICON[kinds.get(id) ?? 'mac'];
    return (
      <>
        <Icon size={14} className="shrink-0 text-muted" />
        <span className="truncate">{names.get(id) ?? id}</span>
      </>
    );
  };
  const setPin = (agent: string, to: string) => pin.mutate({ agent, values: { device: to } });
  return (
    <section className="flex flex-col gap-2" aria-label="Where agents run">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-sm text-muted">Where agents run</h2>
        <Segmented
          label="Where agents run"
          value={placement.mode}
          options={MODES}
          word={(m) => (m === 'auto' ? 'Auto' : 'Manual')}
          disabled={settings.isPending}
          onChange={(m) => {
            if (m !== placement.mode) settings.mutate({ changes: { agent_placement: m } });
          }}
        />
      </div>
      <div className="divide-y divide-border rounded-lg border border-border">
        {placement.agents.map((a) => (
          // The daemon's reason is on hover; the row shows only the device, a pin in force or not taken, and Heavy.
          <div key={a.agent} data-agent={a.agent} title={a.why || undefined} className="flex h-10 items-center gap-3 px-4 text-sm">
            <span className="flex w-40 shrink-0 items-center gap-2">
              <Bot size={14} className="shrink-0 text-muted" />
              <span className="truncate">{nameOf(a.agent)}</span>
            </span>
            <span className="flex min-w-0 flex-1 items-center gap-2">
              {device(a.device)}
              {manual &&
                a.pin &&
                (a.pin === a.device ? (
                  <Pin size={12} aria-label="Pinned" className="shrink-0 text-muted" />
                ) : (
                  <span className="flex min-w-0 items-center gap-1 text-xs text-muted">
                    <PinOff size={12} aria-label="Pin not taken" className="shrink-0" />
                    <span className="truncate">{names.get(a.pin) ?? a.pin}</span>
                  </span>
                ))}
            </span>
            {a.heavy && (
              <span className="flex shrink-0 items-center gap-1 text-xs text-muted">
                <Weight size={12} />
                Heavy
              </span>
            )}
            {manual && (
              <Menu
                align="end"
                trigger={
                  <button
                    type="button"
                    aria-label={`Device for ${nameOf(a.agent)}`}
                    disabled={pin.isPending}
                    className="flex h-7 shrink-0 items-center gap-1.5 rounded-md border border-border px-2 text-xs text-fg disabled:opacity-50"
                  >
                    {a.pin ? (names.get(a.pin) ?? a.pin) : 'Auto'}
                    <ChevronDown size={12} className="text-muted" />
                  </button>
                }
              >
                <MenuItem
                  checked={!a.pin}
                  onClick={() => {
                    if (a.pin) setPin(a.agent, '');
                  }}
                >
                  Auto
                </MenuItem>
                <MenuSeparator />
                {nodes.map((n) => {
                  const lack = n.lacks?.find((l) => l.agent === a.agent);
                  return (
                    <MenuItem
                      key={n.id}
                      checked={a.pin === n.id}
                      onClick={() => {
                        if (a.pin !== n.id) setPin(a.agent, n.id);
                      }}
                    >
                      {n.name}
                      {lack && <span className="text-muted"> · {lack.why}</span>}
                    </MenuItem>
                  );
                })}
              </Menu>
            )}
          </div>
        ))}
      </div>
    </section>
  );
}
