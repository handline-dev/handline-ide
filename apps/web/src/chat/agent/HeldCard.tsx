import { useAgents, useWrite, why } from '@handline/app-data';
import { CloudOff, RotateCw } from 'lucide-react';
import { Button } from '#src/ui/Button.tsx';
import { toast } from '#src/ui/toast.tsx';
import { ago, useNow } from '#src/format/time.ts';

/** At the chat's foot, in the dots row's place: the agent's session is on a machine gone silent, and one press starts it elsewhere now. */
export function HeldCard({ agent }: { agent: string }) {
  const held = useAgents().data?.agents.find((x) => x.key === agent)?.held_on;
  const restart = useWrite('agent.restart_here', agent);
  const now = useNow();
  if (!held?.restart_here || held.silent_since === null) return null;
  const silent = ago(new Date(held.silent_since * 1000).toISOString(), now);
  const go = () => restart.mutateAsync({ agent }).catch((e: unknown) => toast(why(e), { tone: 'warning' }));
  return (
    <div role="status" className="flex max-w-150 items-center gap-2.5 rounded-lg border border-border p-3 text-sm">
      <span className="grid size-4 shrink-0 place-items-center text-muted">
        <CloudOff size={14} />
      </span>
      <span className="min-w-0 flex-1 truncate">
        <span className="font-medium">{held.machine} is offline</span>
        <span className="text-muted"> · silent {silent}</span>
      </span>
      <Button busy={restart.isPending} onClick={() => void go()}>
        <RotateCw size={14} />
        Start elsewhere
      </Button>
    </div>
  );
}
