// Open terminal for a Chat agent: its conversation moves into its terminal, now between turns, else when the turn ends.
import { useAgentDetail, useAgents, useWrite, why } from '@handline/app-data';
import { toast } from '#src/ui/toast.tsx';
import { uuid } from '#src/ui/uuid.ts';

/** `chat` while its session runs (or would start) headless; `open` asks for its terminal and says when that waits on the turn. */
export function useOpenTerminal(agent: string) {
  const inForce = useAgentDetail(agent).data?.settings?.session_mode_in_force;
  const now = useAgents().data?.agents.find((x) => x.key === agent)?.session_mode_now;
  const chat = now ? now === 'chat' : inForce === 'chat';
  const ask = useWrite('agent.open_terminal', agent);
  const open = () =>
    ask.mutate(
      { agent, request_id: uuid() },
      {
        // The answer is the only place that says the move waits for the turn.
        onSuccess: (r) => void (/turn ends/.test(r.said ?? '') && toast('Opens when this turn ends')),
        onError: (e) => toast(why(e), { tone: 'warning' }),
      },
    );
  return { chat, open, busy: ask.isPending };
}
