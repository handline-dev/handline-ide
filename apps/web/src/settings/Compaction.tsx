// Agent switches for every agent (Settings > Agents) and for one agent (its settings page), where its own switch is over the shared one:
// context compaction, and Claude Code's reminder after quiet turns.
import { useSettings, useWrite, why } from '@handline/app-data';
import { Block, Setting } from '#src/settings/Meter.tsx';
import { Toggle } from '#src/ui/Toggle.tsx';

type Key = 'compact_at_task_end' | 'compact_now_stops_turn' | 'no_silence_reminder';
type Values = Partial<Record<Key, boolean | null>>;
/** `inverted`: the switch shows on while the setting is false. */
type Switch = { key: Key; title: string; note: string; inverted?: boolean };

const COMPACTION: Switch[] = [
  {
    key: 'compact_at_task_end',
    title: 'Compact when the task closes',
    note: 'Past Compact at, it waits for the task to close; nothing new is typed in meanwhile.',
  },
  { key: 'compact_now_stops_turn', title: 'Compact now stops the turn', note: 'Off: Compact now waits for the turn to end.' },
];

const QUIET: Switch[] = [
  {
    key: 'no_silence_reminder',
    title: 'Reminder after quiet turns',
    note: 'Asks for a status line after 5 turns with no text. Running agents take a change at their next start.',
    inverted: true,
  },
];

export function CompactionSettings({ agent }: { agent?: string }) {
  return <AgentSwitches agent={agent} title="Compaction" switches={COMPACTION} />;
}

export function QuietSettings({ agent }: { agent?: string }) {
  return <AgentSwitches agent={agent} title="Claude Code" switches={QUIET} />;
}

/** The switches; with `agent`, that agent's own, which writes null when it matches every agent's. */
function AgentSwitches({ agent, title, switches }: { agent?: string; title: string; switches: Switch[] }) {
  const settings = useSettings(agent).data;
  const shared = useSettings().data?.values as Values | undefined;
  const save = useWrite('settings.set');
  if (!settings || !shared) return null;
  const own = (agent ? settings.overrides : undefined) as Values | undefined;
  return (
    <Block title={title}>
      {switches.map(({ key, title, note, inverted = false }) => {
        const on = ((own?.[key] ?? shared[key]) === true) !== inverted;
        const set = (shown: boolean) => {
          const v = shown !== inverted;
          save.mutate(agent ? { agent, changes: { [key]: v === (shared[key] === true) ? null : v } } : { changes: { [key]: v } });
        };
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
