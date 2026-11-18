// Settings > This Mac: what the machine and the agents use, the shared runs, and keep-awake. Stop is the one change to an agent. Every write dims its control until the daemon answers; a refusal says why under it.
import { useNameOf } from '@handline/app-data';
import { Bot, Coffee, Cpu, Gauge, HardDrive, MemoryStick, TriangleAlert, Zap } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { bytesText, loadFull, LOW_DISK_BYTES, memoryLow } from '@handline/app-core';
import { Bar, Block, Setting, toneOf } from '#src/settings/Meter.tsx';
import { SharedRuns } from '#src/settings/SharedRuns.tsx';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { useMachine } from '#src/settings/data/machine.ts';
import { useService } from '#src/settings/data/service.ts';
import { ServiceBlock } from '#src/settings/ServiceBlock.tsx';
import { Button } from '#src/ui/Button.tsx';
import { NumberStepper } from '#src/ui/NumberStepper.tsx';
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

/** A GB target under the bars; 0 is Off. */
function Target({
  title,
  note,
  value,
  disabled,
  onCommit,
}: {
  title: string;
  note: string;
  value: number;
  disabled: boolean;
  onCommit: (n: number) => Promise<unknown>;
}) {
  return (
    <Setting title={title} note={note}>
      <span className="flex items-center gap-2 text-sm">
        <NumberStepper value={value} max={4096} label={title} zeroWord="Off" disabled={disabled} onCommit={onCommit} />
        GB
      </span>
    </Setting>
  );
}

export function MachineTab() {
  const nameOf = useNameOf();
  const m = useMachine();
  const service = useService();
  const [refused, setRefused] = useState<{ key: string; text: string } | null>(null);
  const send = (key: string, go: () => Promise<unknown>) => {
    setRefused(null);
    const done = go();
    done.catch((e: unknown) => setRefused({ key, text: String(e) }));
    return done;
  };
  const run = (key: string, go: () => Promise<unknown>) => void send(key, go);
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
    <SettingsPage keep="settings-machine" title="This Mac" note="What this computer and its agents use, and whether it stays awake.">
      <Block
        title="Resources"
        icon={<Gauge size={16} />}
        note={`${r?.start_hold ? `Starts are held: ${r.start_hold}. ` : ''}Each bar is the share in use: amber from 60%, red from 85%.`}
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
            <Target
              title="Disk: keep at least this much free"
              note={
                m.diskKeepFree
                  ? `Under ${m.diskKeepFree} GB free, Handline removes free, unpinned worktree folders, oldest first, never one holding changes.`
                  : 'Off: Handline removes no folder for disk space.'
              }
              value={m.diskKeepFree}
              disabled={m.pending !== null}
              onCommit={(n) => send('disk_keep_free_gb', () => m.setTarget('disk_keep_free_gb', n))}
            />
            {why('disk_keep_free_gb')}
            <Target
              title="Memory: agents use at most"
              note={
                m.agentsMemoryMax
                  ? `Over ${m.agentsMemoryMax} GB together, Handline closes the longest-idle agent, never one mid-turn; it wakes later from its handoff.`
                  : 'Off: Handline closes no agent for memory.'
              }
              value={m.agentsMemoryMax}
              disabled={m.pending !== null}
              onCommit={(n) => send('agents_memory_max_gb', () => m.setTarget('agents_memory_max_gb', n))}
            />
            {why('agents_memory_max_gb')}
          </div>
        ) : (
          <p className="text-sm text-muted">The daemon has not reported the machine yet.</p>
        )}
      </Block>

      <Block
        title="Agents"
        icon={<Bot size={16} />}
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
                <Bot size={14} className="shrink-0 text-muted" />
                <span className="min-w-0 flex-1 truncate">{nameOf(a.name)}</span>
                <span className="w-24 text-right tabular-nums text-muted">{Math.round(a.cpu_pct)}% of a core</span>
                <span className="w-16 text-right tabular-nums text-muted">{bytesText(a.rss_bytes)}</span>
                <Button disabled={m.pending !== null} onClick={() => run(key, () => m.stop(a.name))} aria-label={`Stop ${nameOf(a.name)}`}>
                  Stop
                </Button>
              </div>
            );
          })}
          {refused?.key.startsWith('stop:') && why(refused.key)}
        </div>
      </Block>

      <SharedRuns />

      <Block title="Power" icon={<Zap size={16} />}>
        <Setting title="Keep awake" icon={<Coffee size={14} />} note="Stops the Mac sleeping on its own. Closing the lid still sleeps it.">
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
      </Block>
      {service && <ServiceBlock service={service} working={m.working} />}
    </SettingsPage>
  );
}
