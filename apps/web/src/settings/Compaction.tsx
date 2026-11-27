// Context compaction: Compact when the task closes and Compact now stops the turn, for every agent (Settings > Agents) and for one agent
// (its settings page), where its own switch is over the shared one.
import { useSettings, useWrite, why } from '@handline/app-data';
import { Block, Setting } from '#src/settings/Meter.tsx';
import { Toggle } from '#src/ui/Toggle.tsx';

type Key = 'compact_at_task_end' | 'compact_now_stops_turn';
type Values = Partial<Record<Key, boolean | null>>;

const SWITCHES: { key: Key; title: string; note: string }[] = [
  {
    key: 'compact_at_task_end',
    title: 'Compact when the task closes',
    note: 'Past Compact at, it waits for the task to close; nothing new is typed in meanwhile.',
  },
  { key: 'compact_now_stops_turn', title: 'Compact now stops the turn', note: 'Off: Compact now waits for the turn to end.' },
];

/** Both switches; with `agent`, that agent's own, which writes null when it matches every agent's. */
export function CompactionSettings({ agent }: { agent?: string }) {
  const settings = useSettings(agent).data;
  const shared = useSettings().data?.values as Values | undefined;
  const save = useWrite('settings.set');
  if (!settings || !shared) return null;
  const own = (agent ? settings.overrides : undefined) as Values | undefined;
  return (
    <Block title="Compaction">
      {SWITCHES.map(({ key, title, note }) => {
        const on = (own?.[key] ?? shared[key]) === true;
        const set = (v: boolean) =>
          save.mutate(agent ? { agent, changes: { [key]: v === (shared[key] === true) ? null : v } } : { changes: { [key]: v } });
        return (
          <Setting key={key} title={title} note={note}>
            <Toggle label={title} on={on} disabled={save.isPending} onChange={set} />
          </Setting>
        );
      })}
      {!!save.error && (
        <p className="text-xs text-danger" role="alert">
          {why(save.error)}
        </p>
      )}
    </Block>
  );
}
