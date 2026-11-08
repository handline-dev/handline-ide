// The app's bottom bar, full width: a quiet status dot, the host, keep-awake, the bell (a preview of Requests), the week's Usage %; the host opens Settings > Machine.
import { Check, Coffee, Pause } from 'lucide-react';
import { BellButton } from '#src/requests/BellButton.tsx';
import { useRef, useState, type RefObject } from 'react';
import type { KeepAwake } from '#src/settings/data/machine.ts';
import { Popover } from '#src/ui/Popover.tsx';
import { openSettings } from '#src/settings/open.ts';
import { useStatusRow, type Level } from '#src/sidebar/status.ts';

const AWAKE: { mode: KeepAwake; label: string; line: string }[] = [
  { mode: 'off', label: 'Off', line: 'The Mac sleeps as usual.' },
  { mode: 'on', label: 'Always on', line: 'The Mac stays awake while Handline runs.' },
  { mode: 'working', label: 'While agents work', line: 'The Mac stays awake only while an agent is working.' },
];

/** What holds now, in one line: the daemon's word, or the mode alone from a daemon that does not say. */
const heldLine = (awake: KeepAwake, held: boolean | null): string =>
  awake === 'off'
    ? 'Not keeping the Mac awake.'
    : held === null
      ? ''
      : held
        ? 'Keeping the Mac awake now.'
        : awake === 'working'
          ? 'Not holding now: no agent is working.'
          : 'Not holding now: this machine cannot be kept awake.';

const DOT: Record<Level, string> = { ok: 'bg-muted', near: 'bg-warning', held: 'bg-danger', down: 'bg-danger' };

export function StatusRow() {
  const s = useStatusRow();
  const busy = s.pending !== null;
  const bar = useRef<HTMLDivElement>(null);
  return (
    <div
      ref={bar}
      className={`flex h-7 shrink-0 items-center gap-2 border-t border-border bg-surface px-3 text-xs text-muted ${busy ? 'opacity-60' : ''}`}
    >
      <button
        type="button"
        title={s.text || undefined}
        onClick={() => openSettings({ section: 'machine' })}
        className="flex min-w-0 flex-1 items-center gap-2 text-left hover:text-fg"
      >
        <span aria-label={s.level} className={`size-2 shrink-0 rounded-full ${s.paused ? 'bg-warning' : DOT[s.level]}`} />
        <span className="truncate">{s.host}</span>
        {s.usagePct !== null && <span className="tabular-nums">Week {s.usagePct}% used</span>}
      </button>
      <KeepAwakeButton s={s} busy={busy} bar={bar} />
      <button
        type="button"
        aria-label={s.paused ? 'Resume new work' : 'Pause new work'}
        aria-pressed={!!s.paused}
        disabled={busy}
        onClick={() => void s.togglePause().catch(() => {})}
      >
        <Pause size={14} className={s.paused ? 'text-warning' : ''} />
      </button>
      <BellButton bar={bar} />
    </div>
  );
}

/** The coffee: lit while the Mac is held awake now (not just set to); a click opens the three choices and what holds now. */
function KeepAwakeButton({ s, busy, bar }: { s: ReturnType<typeof useStatusRow>; busy: boolean; bar: RefObject<HTMLDivElement | null> }) {
  const [open, setOpen] = useState(false);
  const lit = s.held ?? s.awake !== 'off';
  return (
    <Popover
      side="top"
      align="end"
      corner={bar}
      open={open}
      onOpenChange={setOpen}
      trigger={
        <button type="button" aria-label={`Keep awake: ${s.awake}${lit ? ', holding now' : ''}`} disabled={busy}>
          <Coffee size={14} className={lit ? 'text-fg' : ''} />
        </button>
      }
    >
      <div className="flex w-80 flex-col gap-1">
        <p className="px-2 pb-1 text-xs font-medium text-muted">Keep the Mac awake</p>
        {AWAKE.map((a) => (
          <button
            key={a.mode}
            type="button"
            onClick={() => void s.setAwake(a.mode).catch(() => {})}
            className="flex gap-2 rounded-md px-2 py-1.5 text-left hover:bg-hover/30"
          >
            <Check size={14} className={`mt-0.5 shrink-0 ${s.awake === a.mode ? '' : 'invisible'}`} />
            <span className="flex flex-col">
              <span className="text-fg">{a.label}</span>
              <span className="text-xs text-muted">{a.line}</span>
            </span>
          </button>
        ))}
        {heldLine(s.awake, s.held) && <p className="border-t border-border px-2 pt-2 text-xs text-muted">{heldLine(s.awake, s.held)}</p>}
      </div>
    </Popover>
  );
}
