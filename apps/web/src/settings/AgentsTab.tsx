// Settings > Agents: the model and effort a new agent starts with, for Claude Code and for Codex (agents already made keep theirs), and
// the founder's preferences (removable), and each agent with its review level, opening its page (AgentPage).
import { useAgents, useNameOf, useRead, useSettings, useWrite, why } from '@handline/app-data';
import type { SettingsChanges } from '@handline/client';
import { ChevronRight } from 'lucide-react';
import { useState } from 'react';
import { useMe } from '@handline/app-react';
import { LEVEL_WORD, Lessons, WORK_WORD } from '#src/settings/AgentPage.tsx';
import { Mark } from '#src/ui/Face.tsx';
import { agentSettingsOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { closeSettings } from '#src/settings/open.ts';
import type { ReactNode } from 'react';
import { Block, Setting } from '#src/settings/Meter.tsx';
import { ScreenCaptureAll } from '#src/settings/ScreenCapture.tsx';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { Input, Select } from '#src/ui/Field.tsx';
import { ToolLogo } from '#src/ui/ToolLogo.tsx';
import { Toggle } from '#src/ui/Toggle.tsx';

type Key = 'default_model_claude' | 'default_effort_claude' | 'default_model_codex' | 'default_effort_codex';

export function AgentsTab() {
  const settings = useSettings();
  const options = useRead('agent.options', {});
  const save = useWrite('settings.set');
  const put = (key: Key, value: string | null) => save.mutate({ changes: { [key]: value } as SettingsChanges });
  const values = settings.data?.values;
  const opts = options.data;
  const busy = save.isPending;
  const me = useMe();
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
      {values && (
        <Block title="Review">
          <Setting title="Check finished work before it lands">
            <Toggle
              label="Check finished work before it lands"
              on={values.review_flow === true}
              disabled={busy}
              onChange={(on) => save.mutate({ changes: { review_flow: on } })}
            />
          </Setting>
        </Block>
      )}
      <ScreenCaptureAll />
      {me && <Lessons agent={me} founder />}
      <AgentRows />
      {!!save.error && (
        <p className="text-xs text-danger" role="alert">
          {why(save.error)}
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

/** Every agent, with how much of its finished work comes to the founder; a row opens its settings tab and leaves Settings. */
function AgentRows() {
  const agents = useAgents().data?.agents ?? [];
  const nameOf = useNameOf();
  const rows = [...agents].sort((a, b) => nameOf(a.key).localeCompare(nameOf(b.key)));
  if (!rows.length) return null;
  return (
    <Block title="Agents" note="What each has learned, and how much of its finished work you check.">
      <ul className="-mx-2 flex flex-col">
        {rows.map((a) => (
          <li key={a.key}>
            <AgentRow agent={a.key}>
              <Mark who={a.key} />
              <span className="min-w-0 flex-1 truncate">{nameOf(a.key)}</span>
              <span className="text-xs text-muted">
                {a.review_work ? `${WORK_WORD[a.review_work]} ${LEVEL_WORD[a.review_level ?? 'every']}` : LEVEL_WORD.every}
              </span>
              <ChevronRight size={14} className="text-muted" />
            </AgentRow>
          </li>
        ))}
      </ul>
    </Block>
  );
}

/** One row: opens the agent's settings tab by the app's scheme, and Settings closes so the tab shows. */
function AgentRow({ agent, children }: { agent: string; children: ReactNode }) {
  const open = useOpenable(agentSettingsOf(agent), { onOpen: closeSettings });
  return (
    <button
      type="button"
      {...open}
      className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm text-fg hover:bg-hover/30"
    >
      {children}
    </button>
  );
}
