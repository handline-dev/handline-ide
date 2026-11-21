// The app's bottom bar, full width: a quiet status dot, the host, keep-awake, this device's Run · Pause · Off, the bell (a preview of Requests),
// the week's Usage %; the host opens this Mac's page in Settings > Devices.
import { Check, Coffee, Pause } from 'lucide-react';
import { Confirm, MODE_WORD, OFF_TEXT, StateWord, refused } from '#src/settings/DeviceControls.tsx';
import { openModal } from '#src/ui/modal.tsx';
import { BellButton } from '#src/requests/BellButton.tsx';
import { useRef, useState, type RefObject } from 'react';
import type { KeepAwake } from '#src/settings/data/machine.ts';
import { Popover } from '#src/ui/Popover.tsx';
import { openSettings } from '#src/settings/open.ts';
import { MASTER, MODES, useDevices, type Mode } from '#src/settings/data/devices.ts';
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
        onClick={() => openSettings({ section: 'devices', device: MASTER })}
        className="flex min-w-0 flex-1 items-center gap-2 text-left hover:text-fg"
      >
        <span aria-label={s.level} className={`size-2 shrink-0 rounded-full ${s.paused ? 'bg-warning' : DOT[s.level]}`} />
        <span className="truncate">{s.host}</span>
        {s.usagePct !== null && <span className="tabular-nums">Week {s.usagePct}% used</span>}
      </button>
      <KeepAwakeButton s={s} busy={busy} bar={bar} />
      <ModeButton busy={busy} bar={bar} />
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

const MODE_LINE: Record<Mode, string> = {
  run: 'Agents start new work here.',
  pause: 'Nothing new starts here; what runs finishes.',
  off: OFF_TEXT,
};

/** This device's Run · Pause · Off: the pause icon, lit while paused or off; a click opens the three choices. Off asks first. */
function ModeButton({ busy, bar }: { busy: boolean; bar: RefObject<HTMLDivElement | null> }) {
  const q = useDevices();
  const [open, setOpen] = useState(false);
  const row = q.rows.find((r) => r.here && r.runner) ?? q.rows.find((r) => r.master);
  const lit = !!row && row.mode !== 'run';
  const pick = (m: Mode) => {
    if (!row || m === row.mode) return;
    if (m !== 'off') return void q.setMode(row, m).catch(refused);
    setOpen(false);
    void openModal<void>((close) => (
      <Confirm
        title={`Turn ${row.name} off?`}
        text={OFF_TEXT}
        act="Turn off"
        busyWord="Turning off…"
        go={() => q.setMode(row, 'off')}
        close={() => close()}
      />
    ));
  };
  return (
    <Popover
      side="top"
      align="end"
      corner={bar}
      open={open}
      onOpenChange={setOpen}
      trigger={
        <button
          type="button"
          aria-label={row ? `${row.name}: ${MODE_WORD[row.mode ?? 'run']}` : 'Run, Pause or Off'}
          disabled={busy || !row}
        >
          <Pause size={14} className={lit ? 'text-warning' : ''} />
        </button>
      }
    >
      {row && (
        <div className="flex w-80 flex-col gap-1">
          <p className="flex items-center justify-between gap-2 px-2 pb-1 text-xs text-muted">
            <span className="truncate font-medium">{row.name}</span>
            <StateWord row={row} />
          </p>
          {MODES.map((m) => (
            <button
              key={m}
              type="button"
              role="menuitemradio"
              aria-checked={row.mode === m}
              disabled={!row.mine || q.pending !== null || (row.master && m === 'off')}
              onClick={() => pick(m)}
              className="flex gap-2 rounded-md px-2 py-1.5 text-left enabled:hover:bg-hover/30 disabled:opacity-40"
            >
              <Check size={14} className={`mt-0.5 shrink-0 ${row.mode === m ? '' : 'invisible'}`} />
              <span className="flex flex-col">
                <span className="text-fg">{MODE_WORD[m]}</span>
                <span className="text-xs text-muted">{row.master && m === 'off' ? 'The master always runs.' : MODE_LINE[m]}</span>
              </span>
            </button>
          ))}
        </div>
      )}
    </Popover>
  );
}
