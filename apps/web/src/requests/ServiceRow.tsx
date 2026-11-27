// This Mac's background service while it is stopped: one Your turn row with Start, drawn from the shell, since the daemon is what is down.
import { ServerOff } from 'lucide-react';
import { useService } from '#src/settings/data/service.ts';
import { Button } from '#src/ui/Button.tsx';
import { toast } from '#src/ui/toast.tsx';

/** True while this Mac's service is stopped or failed; false in a browser, where the shell is absent. */
export function useServiceDown(): boolean {
  const v = useService();
  return !!v && (v.state === 'stopped' || v.state === 'failed');
}

export function ServiceRow() {
  const v = useService();
  if (!v || (v.state !== 'stopped' && v.state !== 'failed')) return null;
  const start = () => void v.start().catch((e: unknown) => toast(e instanceof Error ? e.message : String(e), { tone: 'warning' }));
  return (
    <div className="flex items-center gap-2.5 border-b border-border px-2 py-3">
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="flex min-w-0 items-baseline gap-2">
          <span className="grid h-lh w-4 shrink-0 self-start place-items-center">
            <ServerOff size={16} className="text-danger" />
          </span>
          <span className="font-medium break-words text-fg">Background service stopped</span>
        </span>
        {v.reason && <span className="line-clamp-2 break-words text-muted">{v.reason}</span>}
      </span>
      <Button variant="primary" disabled={v.pending !== null} onClick={start}>
        Start
      </Button>
    </div>
  );
}
