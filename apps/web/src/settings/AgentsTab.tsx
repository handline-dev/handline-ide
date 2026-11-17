// Settings > Agents: the model and effort a new agent starts with, for Claude Code and for Codex. Agents already made keep theirs.
import { classify, hooks, sentence, useSettings, useWrite } from '@handline/app-data';
import type { SettingsChanges } from '@handline/client';
import { useState } from 'react';
import { Block, Setting } from '#src/settings/Meter.tsx';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { Input, Select } from '#src/ui/Field.tsx';
import { ToolLogo } from '#src/ui/ToolLogo.tsx';

type Key = 'default_model_claude' | 'default_effort_claude' | 'default_model_codex' | 'default_effort_codex';

export function AgentsTab() {
  const settings = useSettings();
  const options = hooks.useAgentOptions();
  const save = useWrite('settings.set');
  const put = (key: Key, value: string | null) => save.mutate({ changes: { [key]: value } as SettingsChanges });
  const values = settings.data?.values;
  const opts = options.data;
  const busy = save.isPending;
  /** A select of `list`, the saved value kept in it even when the list lacks it. */
  const choose = (key: Key, label: string, list: string[]) => {
    const now = values?.[key] ?? '';
    return (
      <div className="w-36">
        <Select aria-label={label} value={now} disabled={busy} onChange={(e) => put(key, e.target.value)}>
          {[...new Set([now, ...list])].map((v) => (
            <option key={v}>{v}</option>
          ))}
        </Select>
      </div>
    );
  };
  return (
    <SettingsPage keep="settings-agents" title="Agents" note="The model and effort a new agent gets. Agents you already have keep theirs.">
      {values && opts && (
        <Block title="New agents start with">
          <Setting title="Claude Code" icon={<ToolLogo tool="claude" />}>
            <div className="flex gap-2">
              {choose('default_model_claude', 'Claude model', opts.models)}
              {choose('default_effort_claude', 'Claude effort', opts.efforts)}
            </div>
          </Setting>
          <Setting title="Codex" icon={<ToolLogo tool="codex" />} note="Codex runs xhigh and max effort as high.">
            <div className="flex gap-2">
              <CodexModel value={values.default_model_codex ?? ''} disabled={busy} save={(v) => put('default_model_codex', v || null)} />
              {choose('default_effort_codex', 'Codex effort', opts.efforts)}
            </div>
          </Setting>
        </Block>
      )}
      {!!save.error && (
        <p className="text-xs text-danger" role="alert">
          {sentence(classify(save.error))}
        </p>
      )}
    </SettingsPage>
  );
}

/** Free text, since Codex has no list: saved when it leaves the box or on Enter; empty lets Codex pick. */
function CodexModel({ value, disabled, save }: { value: string; disabled: boolean; save: (v: string) => void }) {
  const [text, setText] = useState(value);
  const [sent, setSent] = useState(value);
  const done = () => {
    const v = text.trim();
    if (v === sent) return;
    setSent(v);
    save(v);
  };
  return (
    <div className="w-36">
      <Input
        aria-label="Codex model"
        value={text}
        placeholder="Codex's default"
        disabled={disabled}
        onChange={(e) => setText(e.target.value)}
        onBlur={done}
        onKeyDown={(e) => e.key === 'Enter' && !e.nativeEvent.isComposing && done()}
      />
    </div>
  );
}
