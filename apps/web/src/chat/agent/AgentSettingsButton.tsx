// The agent's settings, from its chat: opens them as a tab, by the app's one scheme (click here, Cmd-click a new tab, Option-click beside).
import { Settings } from 'lucide-react';
import { agentSettingsOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';

export function AgentSettingsButton({ agent }: { agent: string }) {
  const open = useOpenable(agentSettingsOf(agent));
  return (
    <button
      type="button"
      aria-label="Agent settings"
      title="Agent settings"
      {...open}
      className="flex size-7 items-center justify-center rounded-md text-muted hover:text-fg"
    >
      <Settings size={14} />
    </button>
  );
}
