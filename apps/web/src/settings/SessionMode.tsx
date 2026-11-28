// Chat or Terminal: how a workspace's agents run, and one agent's own choice beside it (Settings › Agents, an agent's page).
import { useAgentDetail, useAgents, useRead, useWrite, why } from '@handline/app-data';
import { Block, Setting } from '#src/settings/Meter.tsx';
import { Segmented } from '#src/ui/Segmented.tsx';
import { toast } from '#src/ui/toast.tsx';
import { uuid } from '#src/ui/uuid.ts';

type Mode = 'chat' | 'terminal';
const MODES: readonly Mode[] = ['chat', 'terminal'];
const WORD: Record<Mode, string> = { chat: 'Chat', terminal: 'Terminal' };
const failed = (e: unknown) => toast(why(e) || 'Did not change', { tone: 'warning' });

/** One row per workspace. */
export function WorkspaceSessionMode() {
  const list = useRead('workspaces.list', {}).data?.workspaces ?? [];
  const set = useWrite('workspace.set');
  if (!list.length) return null;
  const one = list.length === 1;
  return (
    <Block title="Workspaces run agents as">
      {list.map((w) => (
        <Setting key={w.key} title={one ? 'Agents run as' : w.name}>
          <Segmented
            label={`${w.name}: agents run as`}
            value={w.session_mode}
            options={MODES}
            word={(m) => WORD[m]}
            disabled={set.isPending}
            onChange={(m) => set.mutate({ workspace: w.key, session_mode: m, request_id: uuid() }, { onError: failed })}
          />
        </Setting>
      ))}
    </Block>
  );
}

/** Default (its workspace's mode), or Chat or Terminal for this agent alone. */
export function AgentSessionMode({ agent }: { agent: string }) {
  const settings = useAgentDetail(agent).data?.settings;
  const wsKey = useAgents().data?.agents.find((a) => a.key === agent)?.workspace;
  const ws = useRead('workspaces.list', {}).data?.workspaces.find((w) => w.key === wsKey);
  const set = useWrite('agent.set', agent);
  if (!settings || !ws) return null;
  const own: '' | Mode = settings.session_mode ?? '';
  const word = (m: '' | Mode) => (m ? WORD[m] : 'Default');
  return (
    <Block title="Session" note={`Default follows its workspace: ${WORD[ws.session_mode]}.`}>
      <Setting title="Runs as">
        <Segmented
          label="Runs as"
          value={own}
          options={['', ...MODES] as const}
          word={word}
          disabled={set.isPending}
          onChange={(m) => set.mutate({ agent, values: { session_mode: m } }, { onError: failed })}
        />
      </Setting>
    </Block>
  );
}
