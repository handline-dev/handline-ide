// Screen capture: Block screen capture for every agent and Send screenshot commands through Handline (Settings > Agents), and the
// same block for one agent (its settings page), which is on while the shared one is.
import { useSettings, useWrite, why } from '@handline/app-data';
import { Block, Setting } from '#src/settings/Meter.tsx';
import { Toggle } from '#src/ui/Toggle.tsx';

const BLOCK = 'Block screen capture';
const VIA = 'Send screenshot commands through Handline';
const NAMES = 'The refusal names this setting.';

type Values = { block_screen_capture?: boolean; screencapture_via_handline?: boolean };

/** Settings > Agents: both switches, for every agent. */
export function ScreenCaptureAll() {
  const values = useSettings().data?.values as Values | undefined;
  const save = useWrite('settings.set');
  if (!values) return null;
  return (
    <Block title="Screen capture">
      <Setting title={BLOCK} note={`Every agent's window capture is refused. ${NAMES}`}>
        <Toggle
          label={BLOCK}
          on={values.block_screen_capture === true}
          disabled={save.isPending}
          onChange={(on) => save.mutate({ changes: { block_screen_capture: on } })}
        />
      </Setting>
      <Setting title={VIA} note={`A screenshot command in an agent's shell is refused and points to Handline's own capture. ${NAMES}`}>
        <Toggle
          label={VIA}
          on={values.screencapture_via_handline === true}
          disabled={save.isPending}
          onChange={(on) => save.mutate({ changes: { screencapture_via_handline: on } })}
        />
      </Setting>
      {!!save.error && (
        <p className="text-xs text-danger" role="alert">
          {why(save.error)}
        </p>
      )}
    </Block>
  );
}

/** One agent's settings: its own block; while every agent's is on, it shows on and says so. Off writes null, back to the shared one. */
export function ScreenCaptureAgent({ agent }: { agent: string }) {
  const settings = useSettings(agent).data;
  const save = useWrite('settings.set');
  if (!settings) return null;
  const shared = (settings.values as Values).block_screen_capture === true;
  const own = (settings.overrides as Values | undefined)?.block_screen_capture === true;
  return (
    <Block title="Screen capture">
      <Setting title={BLOCK} note={shared ? 'On for every agent in Settings > Agents.' : `Its window capture is refused. ${NAMES}`}>
        <Toggle
          label={BLOCK}
          on={shared || own}
          disabled={shared || save.isPending}
          onChange={(on) => save.mutate({ agent, changes: { block_screen_capture: on ? true : null } })}
        />
      </Setting>
      {!!save.error && (
        <p className="text-xs text-danger" role="alert">
          {why(save.error)}
        </p>
      )}
    </Block>
  );
}
