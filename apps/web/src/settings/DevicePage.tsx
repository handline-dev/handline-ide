import { pct } from '#src/format/count.ts';
// One device's page: its state and Run · Pause · Off, then This Mac's sections for the master, or a lent machine's agents, load and cap.
import { useNameOf, useServes } from '@handline/app-data';
import { bytesText } from '@handline/app-core';
import { BatteryMedium, Bot, Cpu, Ellipsis, Gauge, MemoryStick, Plug, TriangleAlert } from 'lucide-react';
import { SettingsBar } from '#src/settings/SettingsBar.tsx';
import { MachineSections } from '#src/settings/MachineSections.tsx';
import { confirmMakeMaster } from '#src/connection/master.tsx';
import { useCluster } from '#src/connection/clusters.tsx';
import { KIND_ICON, KIND_WORD, ModeControl, StateWord, confirmRevoke, refused } from '#src/settings/DeviceControls.tsx';
import { Block, Setting } from '#src/settings/Meter.tsx';
import { Bar, toneOf } from '#src/ui/Meter.tsx';
import { NumberStepper } from '#src/ui/NumberStepper.tsx';
import { canUpdate, type DeviceRow, type useDevices } from '#src/settings/data/devices.ts';
import { Button } from '#src/ui/Button.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { Menu, MenuItem } from '#src/ui/Menu.tsx';
import { Page } from '#src/ui/Page.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';

function AgentsHere({ row }: { row: DeviceRow }) {
  const nameOf = useNameOf();
  return (
    <Block title="Agents here" icon={<Bot size={16} />}>
      {row.agents.length === 0 && <p className="text-sm text-muted">No agent stays here.</p>}
      {row.agents.map((a) => (
        <div key={a} className="flex items-center gap-2 text-sm">
          <Bot size={14} className="text-muted" />
          {nameOf(a)}
        </div>
      ))}
      {row.lacks.length > 0 && (
        <div className="flex flex-col gap-1 border-t border-border pt-3">
          <p className="text-xs text-muted">Cannot start here now</p>
          {row.lacks.map((l) => (
            <div key={l.agent} className="flex items-center gap-2 text-sm">
              <TriangleAlert size={14} className="text-warning" />
              <span>{nameOf(l.agent)}</span>
              <span className="text-muted">{l.why}</span>
            </div>
          ))}
        </div>
      )}
    </Block>
  );
}

type Devices = ReturnType<typeof useDevices>;

/** A lent machine's load from its newest heartbeat, and its own cap on the agents' memory (empty: the master's cap holds). */
function Resources({ row, q }: { row: DeviceRow; q: Devices }) {
  const load = row.node?.load;
  const cpu = load ? pct(load.load1, load.cores) : 0;
  const used = load ? load.ram_total - load.ram_free : 0;
  const mem = load ? pct(used, load.ram_total) : 0;
  const own = row.node?.memory_max_gb ?? 0;
  return (
    <Block title="Resources" icon={<Gauge size={16} />}>
      {load ? (
        <>
          <div className="flex items-center gap-3 text-sm" data-resource="CPU">
            <Cpu size={16} className="text-muted" />
            <span className="w-16 shrink-0">CPU</span>
            <Bar pct={Math.min(100, cpu)} tone={toneOf(cpu)} />
            <span className="w-48 shrink-0 text-right tabular-nums">
              {cpu}% busy <span className="text-muted">· {load.cores} cores</span>
            </span>
          </div>
          <div className="flex items-center gap-3 text-sm" data-resource="Memory">
            <MemoryStick size={16} className="text-muted" />
            <span className="w-16 shrink-0">Memory</span>
            <Bar pct={mem} tone={toneOf(mem)} />
            <span className="w-48 shrink-0 text-right tabular-nums">
              {bytesText(used)} used <span className="text-muted">of {bytesText(load.ram_total)}</span>
            </span>
          </div>
          {row.node?.battery !== null && row.node?.battery !== undefined && (
            <p className="flex items-center gap-2 text-xs text-muted">
              {row.node.battery ? <BatteryMedium size={14} /> : <Plug size={14} />}
              {row.node.battery ? 'On battery' : 'Plugged in'}
            </p>
          )}
        </>
      ) : (
        <p className="text-sm text-muted">Not heard from yet.</p>
      )}
      <Setting title="Agents may use up to" icon={<MemoryStick size={14} />}>
        <span className="flex items-center gap-2 text-sm">
          <NumberStepper
            value={own}
            max={4096}
            label="Agents may use up to"
            zeroWord={q.memoryMax ? `${q.memoryMax} (master's)` : 'No limit'}
            disabled={!row.mine || q.pending !== null}
            onCommit={(n) => q.setMemoryMax(row, n)}
          />
          GB
        </span>
      </Setting>
    </Block>
  );
}

export function DevicePage({ row, q, back }: { row: DeviceRow; q: Devices; back: () => void }) {
  const Icon = KIND_ICON[row.kind ?? 'desktop'];
  const owner = useServes('device.make_master').served;
  const cluster = useCluster();
  return (
    <ScrollFade keep={`settings-device-${row.id}`} className="min-h-0 flex-1">
      <Page>
        <div className="flex flex-col gap-4 pb-8 pt-3">
          <SettingsBar title={row.name} icon={<Icon size={16} className="shrink-0 text-muted" />} back={back} backTo="Devices" />
          <div className="flex items-center gap-3">
            <span className="flex shrink-0 items-center gap-2 text-sm text-muted">
              {row.kind && KIND_WORD[row.kind]}
              {row.master && ' · Master'}
              <span>·</span>
              <StateWord row={row} />
            </span>
            <span className="flex-1" />
            {canUpdate(row) && (
              <Button
                busy={q.pending === `update:${row.id}`}
                disabled={q.pending !== null}
                onClick={() => void q.update(row).catch(refused)}
              >
                {row.appReady && !row.behind ? `Update to ${row.appReady}` : 'Update'}
              </Button>
            )}
            {row.runner && row.mode && <ModeControl row={row} busy={q.pending !== null} setMode={(m) => q.setMode(row, m)} />}
            {(owner || (row.here && cluster)) && row.runner && row.mine && !row.master && (
              <Button
                disabled={q.pending !== null || row.state === 'offline' || row.mode === 'off'}
                onClick={() => confirmMakeMaster(row.name, () => q.makeMaster(row))}
              >
                Make master
              </Button>
            )}
            {!row.master && (
              <Menu
                side="bottom"
                align="end"
                trigger={
                  <IconButton label={`More for ${row.name}`} className="size-7 rounded-md border border-border">
                    <Ellipsis size={14} />
                  </IconButton>
                }
              >
                <MenuItem danger onClick={() => confirmRevoke([row], (id) => q.revoke(id).then(back))}>
                  Revoke…
                </MenuItem>
              </Menu>
            )}
          </div>
          {row.master && <MachineSections />}
          {!row.master && row.runner && (
            <>
              <AgentsHere row={row} />
              <Resources row={row} q={q} />
            </>
          )}
        </div>
      </Page>
    </ScrollFade>
  );
}
