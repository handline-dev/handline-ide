// Settings > This Mac: what the machine and the agents use, the simulator and browser limits, and keep-awake. Stop is the one change to an agent. Every write dims its control until the daemon answers; a refusal says why under it.
import { Bot, Clock, Coffee, Cpu, Gauge, Globe, HardDrive, MemoryStick, Smartphone, TriangleAlert, Zap } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { bytesText, loadFull, LOW_DISK_BYTES, memoryLow } from '@handline/app-core';
import { Bar, Block, LineRow, Setting, toneOf } from '#src/settings/Meter.tsx';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { useMachine } from '#src/settings/data/machine.ts';
import { useService } from '#src/settings/data/service.ts';
import { ServiceBlock } from '#src/settings/ServiceBlock.tsx';
import { Button } from '#src/ui/Button.tsx';
import { NumberStepper } from '#src/ui/NumberStepper.tsx';
import { Switch } from '#src/usage/UsageViews.tsx';

const LEASE_KINDS = ['simulator', 'browser'] as const;
const ordinal = (n: number) => `${n}${['th', 'st', 'nd', 'rd'][n % 100 >= 11 && n % 100 <= 13 ? 0 : n % 10 < 4 ? n % 10 : 0]}`;
/** "since 14:02", or with the day when it is not today. */
function sinceText(iso: string) {
  const d = new Date(iso);
  const time = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  return d.toDateString() === new Date().toDateString()
    ? `since ${time}`
    : `since ${d.toLocaleDateString([], { month: 'short', day: 'numeric' })} ${time}`;
}
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
        title="Simulators and browsers"
        icon={<Smartphone size={16} />}
        note="The most all agents hold together. Over it, an agent waits its turn. 0 means no limit."
      >
        {LEASE_KINDS.flatMap((kind) => m.leases.find((l) => l.kind === kind) ?? []).map((k) => {
          const key = `${k.kind}_max`;
          const word = k.kind === 'simulator' ? 'Simulators' : 'Browsers';
          const rows = [
            ...k.held.map((h) => ({
              id: `h${h.lease}`,
              icon: <Bot size={12} />,
              who: h.agent,
              what: `in use ${sinceText(h.since)}`,
              title: undefined,
            })),
            ...[...k.queue]
              .sort((x, y) => x.position - y.position)
              .map((w) => ({
                id: `w${w.position}`,
                icon: <Clock size={12} />,
                who: w.agent,
                what: `waiting, ${ordinal(w.position)} in line`,
                title: w.says,
              })),
          ];
          const over = k.max > 0 && k.held.length > k.max;
          return (
            <Setting
              key={k.kind}
              title={word}
              icon={k.kind === 'simulator' ? <Smartphone size={14} /> : <Globe size={14} />}
              note={
                <>
                  {k.max === 0 ? 'Any number at once.' : `Up to ${k.max} at once.`}{' '}
                  {k.held.length === 0 ? 'None in use.' : `${k.held.length} in use.`}
                  {k.queue.length > 0 && ` ${k.queue.length} waiting.`}
                  {over && ' Over the limit: they keep running and new asks wait.'}
                </>
              }
              below={
                rows.length > 0 || refused?.key === key ? (
                  <>
                    {rows.slice(0, 5).map((r) => (
                      <LineRow key={r.id} icon={r.icon} who={r.who} what={r.what} title={r.title} />
                    ))}
                    {rows.length > 5 && <span className="text-xs text-muted">and {rows.length - 5} more</span>}
                    {why(key)}
                  </>
                ) : undefined
              }
            >
              <NumberStepper
                value={k.max}
                label={`${word} at once`}
                stepLabels={[`Fewer ${word.toLowerCase()}`, `More ${word.toLowerCase()}`]}
                zeroWord="No limit"
                disabled={m.pending !== null}
                onCommit={(n) => send(key, () => m.setLeaseMax(k.kind, n))}
              />
            </Setting>
          );
        })}
      </Block>

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
