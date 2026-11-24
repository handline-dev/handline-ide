// Settings > Devices, under the device rows: where each agent's next session starts and why (node.status placement), with Auto or Manual
// (agent_placement). Manual adds a device menu per agent; a pin is agent.set device, and Auto in the menu clears it.
import { Hooks, useNameOf, useWrite } from '@handline/app-data';
import { ChevronDown } from 'lucide-react';
import { Menu, MenuItem, MenuSeparator } from '#src/ui/Menu.tsx';
import { Segmented } from '#src/ui/Segmented.tsx';

const MODES = ['auto', 'manual'] as const;

const sentence = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

/** Nothing on a single Mac, where every agent runs here. */
export function AgentPlacement() {
  const status = Hooks.useNodeStatus().data;
  const nameOf = useNameOf();
  const settings = useWrite('settings.set');
  const pin = useWrite('agent.set');
  const placement = status?.placement;
  const nodes = status?.nodes ?? [];
  if (!placement || nodes.length < 2) return null;
  const manual = placement.mode === 'manual';
  const names = new Map(nodes.map((n) => [n.id, n.name]));
  const nodeName = (id: string) => names.get(id) ?? id;
  const setPin = (agent: string, device: string) => pin.mutate({ agent, values: { device } });
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
          <div key={a.agent} data-agent={a.agent} className="flex min-h-10 items-center gap-3 px-4 py-1.5 text-sm">
            <span className="w-40 shrink-0 truncate">{nameOf(a.agent)}</span>
            <span className="w-40 shrink-0 truncate">{nodeName(a.device)}</span>
            <span className="min-w-0 flex-1 text-right text-muted">{a.why && sentence(a.why)}</span>
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
                    {a.pin ? nodeName(a.pin) : 'Auto'}
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
