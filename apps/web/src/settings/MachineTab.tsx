// Settings > Machine: what the machine and the agents use, the limits that hold new starts, keep-awake, and the disk. Stop is the one change to an
// agent. Every write dims its control until the daemon answers; a refusal says why under it.
import { Cpu, HardDrive, MemoryStick, TriangleAlert } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { bytesText, limitSentence, loadFull, LOAD_PRESETS, LOW_DISK_BYTES, memoryLow, MEMORY_PRESETS } from '@handline/app-core';
import { Bar, Block, Setting, toneOf } from '#src/settings/Meter.tsx';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { useMachine } from '#src/settings/data/machine.ts';
import { useService } from '#src/settings/data/service.ts';
import { ServiceBlock } from '#src/settings/ServiceBlock.tsx';
import { Button } from '#src/ui/Button.tsx';
import { Switch } from '#src/usage/UsageViews.tsx';

const AWAKE = ['off', 'on', 'working'] as const;
const AWAKE_WORD = { off: 'Off', on: 'On', working: 'While agents work' } as const;

function Row({
  icon,
  label,
  pct,
  low,
  lowWord,
  children,
}: {
  icon: ReactNode;
  label: string;
  pct: number;
  low?: boolean;
  lowWord: string;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm" data-resource={label}>
      <span className="text-muted">{icon}</span>
      <span className="w-16 shrink-0">{label}</span>
      <Bar pct={pct} tone={toneOf(pct, low)} />
      <span className="w-64 shrink-0 text-right tabular-nums">
        {low && (
          <span className="mr-2 inline-flex items-center gap-1 text-xs text-danger">
            <TriangleAlert size={12} />
            {lowWord}
          </span>
        )}
        {children}
      </span>
    </div>
  );
}

export function MachineTab() {
  const m = useMachine();
  const service = useService();
  const [refused, setRefused] = useState<{ key: string; text: string } | null>(null);
  const run = (key: string, go: () => Promise<unknown>) => {
    setRefused(null);
    go().catch((e: unknown) => setRefused({ key, text: String(e) }));
  };
  const r = m.resources;
  const mc = r?.machine;
  const diskUsed = mc?.disk_total_bytes ? Math.round((1 - mc.disk_free_bytes / mc.disk_total_bytes) * 100) : 0;
  const diskLow = !!mc && mc.disk_free_bytes < LOW_DISK_BYTES;
  const why = (key: string) =>
    refused?.key === key ? (
      <p className="text-xs text-danger" role="alert">
        {refused.text}
      </p>
    ) : null;
  const load = m.limits.load_per_core_pct;
  const free = m.limits.ram_free_min_pct;
  return (
    <SettingsPage keep="settings-machine" title="Machine" note="What this computer and its agents use, and when new agents wait.">
      <Block
        title="Resources"
        note={`${r?.start_hold ? `Starts are held: ${r.start_hold}.` : 'New agents start as usual.'} Each bar is the share in use: amber from 60%, red from 85% or when a limit holds starts.`}
      >
        {mc && r ? (
          <div className="flex flex-col gap-3">
            <Row
              icon={<Cpu size={16} />}
              label="CPU"
              pct={Math.min(100, Math.round(mc.load_per_core * 100))}
              low={loadFull(r, load)}
              lowWord="Load full"
            >
              {Math.round(mc.load_per_core * 100)}% busy <span className="text-muted">· {mc.cores} cores</span>
            </Row>
            <Row
              icon={<MemoryStick size={16} />}
              label="Memory"
              pct={Math.round(100 - mc.ram_free_pct)}
              low={memoryLow(r, free)}
              lowWord="Memory low"
            >
              {bytesText(mc.ram_used_bytes)} used <span className="text-muted">of {bytesText(mc.ram_total_bytes)}</span>
            </Row>
            <Row icon={<HardDrive size={16} />} label="Disk" pct={diskUsed} low={diskLow} lowWord="Disk low">
              {bytesText(mc.disk_total_bytes - mc.disk_free_bytes)} used{' '}
              <span className="text-muted">of {bytesText(mc.disk_total_bytes)}</span>
            </Row>
          </div>
        ) : (
          <p className="text-sm text-muted">The daemon has not reported the machine yet.</p>
        )}
      </Block>

      <Block
        title="Agents"
        note={
          r
            ? `${m.agents.length} running, using ${Math.round(r.agents_total.cpu_pct)}% of one core and ${bytesText(r.agents_total.rss_bytes)} of memory.`
            : undefined
        }
      >
        <div className="flex flex-col gap-2">
          {m.agents.length === 0 && <p className="text-sm text-muted">No agent is running.</p>}
          {m.agents.map((a) => {
            const key = `stop:${a.name}`;
            return (
              <div key={a.name} className={`flex items-center gap-3 text-sm ${m.pending === key ? 'opacity-50' : ''}`}>
                <span className="min-w-0 flex-1 truncate">{a.name}</span>
                <span className="w-24 text-right tabular-nums text-muted">{Math.round(a.cpu_pct)}% of a core</span>
                <span className="w-16 text-right tabular-nums text-muted">{bytesText(a.rss_bytes)}</span>
                <Button disabled={m.pending !== null} onClick={() => run(key, () => m.stop(a.name))} aria-label={`Stop ${a.name}`}>
                  Stop
                </Button>
              </div>
            );
          })}
          {refused?.key.startsWith('stop:') && why(refused.key)}
        </div>
      </Block>

      <Block
        title="Limits"
        note="While a limit is over, new agents wait to start. Running agents are never stopped, and starting one by hand always works."
      >
        <Setting
          title="CPU load per core"
          note={limitSentence(r ? `${r.machine.load_per_core.toFixed(1)}` : '-', !!r && loadFull(r, load), load === 0)}
        >
          <div className={m.pending === 'load_per_core_pct' ? 'opacity-50' : ''}>
            <Switch
              label="Load limit"
              value={String(load)}
              options={LOAD_PRESETS.map((p) => String(p.pct))}
              word={(v) => LOAD_PRESETS.find((p) => String(p.pct) === v)?.label ?? `${v}%`}
              onChange={(v) => m.pending === null && run('load_per_core_pct', () => m.setLimit('load_per_core_pct', Number(v)))}
            />
          </div>
        </Setting>
        {why('load_per_core_pct')}
        <Setting
          title="Memory kept free"
          note={limitSentence(r ? `${r.machine.ram_free_pct}% free` : '-', !!r && memoryLow(r, free), free === 0)}
        >
          <div className={m.pending === 'ram_free_min_pct' ? 'opacity-50' : ''}>
            <Switch
              label="Memory limit"
              value={String(free)}
              options={MEMORY_PRESETS.map((p) => String(p.pct))}
              word={(v) => MEMORY_PRESETS.find((p) => String(p.pct) === v)?.label ?? `${v}%`}
              onChange={(v) => m.pending === null && run('ram_free_min_pct', () => m.setLimit('ram_free_min_pct', Number(v)))}
            />
          </div>
        </Setting>
        {why('ram_free_min_pct')}
      </Block>

      <Block title="Other">
        <Setting title="Keep awake" note="Stops the Mac sleeping on its own. Closing the lid still sleeps it.">
          <div className={m.pending === 'awake' ? 'opacity-50' : ''}>
            <Switch
              label="Keep awake"
              value={m.awake}
              options={AWAKE}
              word={(v) => AWAKE_WORD[v] + (v === 'working' ? ` (${m.working})` : '')}
              onChange={(v) => m.pending === null && run('awake', () => m.setAwake(v))}
            />
          </div>
        </Setting>
        {why('awake')}
        <Setting
          title="Tool record in chat"
          note="Each turn's tool calls, folded under one line in the agent's chat. One database write per call."
        >
          <div className={m.pending === 'record' ? 'opacity-50' : ''}>
            <Switch
              label="Tool record in chat"
              value={m.record ? 'on' : 'off'}
              options={['off', 'on'] as const}
              word={(v) => (v === 'on' ? 'On' : 'Off')}
              onChange={(v) => m.pending === null && run('record', () => m.setRecord(v === 'on'))}
            />
          </div>
        </Setting>
        {why('record')}
      </Block>
      {service && <ServiceBlock service={service} working={m.working} />}
    </SettingsPage>
  );
}
