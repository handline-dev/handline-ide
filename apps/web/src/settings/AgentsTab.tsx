// Settings > Agents: the model and effort a new agent starts with, for Claude Code and for Codex (agents already made keep theirs), and
// each agent with its review level, opening its page (AgentPage).
import { classify, hooks, sentence, useAgents, useNameOf, useSettings, useWrite } from '@handline/app-data';
import type { SettingsChanges } from '@handline/client';
import { ChevronRight } from 'lucide-react';
import { useState } from 'react';
import { AgentPage, LEVEL_WORD } from '#src/settings/AgentPage.tsx';
import { Mark } from '#src/ui/Face.tsx';
import { Block, Setting } from '#src/settings/Meter.tsx';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { Input, Select } from '#src/ui/Field.tsx';
import { ToolLogo } from '#src/ui/ToolLogo.tsx';

type Key = 'default_model_claude' | 'default_effort_claude' | 'default_model_codex' | 'default_effort_codex';

/** The list, or one agent's page: `open` is the agent Settings was opened on (a review notice's Open). */
export function AgentsTab({ open }: { open?: string }) {
  const [agent, setAgent] = useState(open);
  return agent ? <AgentPage agent={agent} back={() => setAgent(undefined)} /> : <AgentsList pick={setAgent} />;
}

function AgentsList({ pick }: { pick: (agent: string) => void }) {
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
      <AgentRows pick={pick} />
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

/** Every agent, with how much of its finished work comes to the founder; a row opens its page. */
function AgentRows({ pick }: { pick: (agent: string) => void }) {
  const agents = useAgents().data?.agents ?? [];
  const nameOf = useNameOf();
  const rows = [...agents].sort((a, b) => nameOf(a.key).localeCompare(nameOf(b.key)));
  if (!rows.length) return null;
  return (
    <Block title="Agents" note="What each has learned, and how much of its finished work you check.">
      <ul className="-mx-2 flex flex-col">
        {rows.map((a) => (
          <li key={a.key}>
            <button
              type="button"
              onClick={() => pick(a.key)}
              className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm text-fg hover:bg-hover/30"
            >
              <Mark who={a.key} />
              <span className="min-w-0 flex-1 truncate">{nameOf(a.key)}</span>
              <span className="text-xs text-muted">{LEVEL_WORD[a.review_level ?? 'every']}</span>
              <ChevronRight size={14} className="text-muted" />
            </button>
          </li>
        ))}
      </ul>
    </Block>
  );
}
