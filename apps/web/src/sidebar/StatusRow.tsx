// The app's bottom bar, full width so later rows (a cluster of machines) have room: a quiet status dot, the host, keep-awake and the bell, the week's Usage %; a click goes to Settings > Machine.
import { Bell, Coffee, Pause } from 'lucide-react';
import { usePersisted } from '#src/persist/kv.ts';
import { openSettings } from '#src/settings/open.ts';
import { useStatusRow, type Level } from '#src/sidebar/status.ts';

const DOT: Record<Level, string> = { ok: 'bg-muted', near: 'bg-warning', held: 'bg-danger', down: 'bg-danger' };

export function StatusRow() {
  const s = useStatusRow();
  const [, setSection] = usePersisted<string>('settings:section', 'accounts');
  const busy = s.pending !== null;
  return (
    <div
      className={`flex h-7 shrink-0 items-center gap-2 border-t border-border bg-surface px-3 text-xs text-muted ${busy ? 'opacity-60' : ''}`}
    >
      <button
        type="button"
        title={s.text || undefined}
        onClick={() => (setSection('machine'), openSettings())}
        className="flex min-w-0 flex-1 items-center gap-2 text-left hover:text-fg"
      >
        <span aria-label={s.level} className={`size-2 shrink-0 rounded-full ${s.paused ? 'bg-warning' : DOT[s.level]}`} />
        <span className="truncate">{s.host}</span>
        {s.usagePct !== null && <span className="tabular-nums">Week {s.usagePct}% used</span>}
      </button>
      <button
        type="button"
        aria-label={`Keep awake: ${s.awake}`}
        aria-pressed={s.awake !== 'off'}
        disabled={busy}
        onClick={() => void s.cycleAwake().catch(() => {})}
      >
        <Coffee size={14} className={s.awake === 'off' ? '' : 'text-fg'} />
      </button>
      <button
        type="button"
        aria-label={s.paused ? 'Resume new work' : 'Pause new work'}
        aria-pressed={!!s.paused}
        disabled={busy}
        onClick={() => void s.togglePause().catch(() => {})}
      >
        <Pause size={14} className={s.paused ? 'text-warning' : ''} />
      </button>
      <span className="relative" aria-label={`${s.unread} unread`}>
        <Bell size={14} />
        {s.unread > 0 && <span className="absolute -top-1 -right-1 size-1.5 rounded-full bg-accent" />}
      </span>
    </div>
  );
}
