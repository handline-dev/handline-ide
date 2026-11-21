// A device's Run · Pause · Off, its state as a shape and a word, and the confirm that waits for the daemon. Shared by the Devices list, a
// device's page and the status bar's pause popover.
import { Circle, CircleDot, Globe, Laptop, Loader, Pause, Play, Server, Smartphone, TriangleAlert, type LucideIcon } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { ago } from '@handline/app-core';
import { MODES, type DeviceRow, type Mode, type State } from '#src/settings/data/devices.ts';
import { Button } from '#src/ui/Button.tsx';
import { ModalFrame, openModal } from '#src/ui/modal.tsx';
import { toast } from '#src/ui/toast.tsx';

export const KIND_WORD = {
  ios: 'iPhone / iPad',
  android: 'Android',
  web: 'Browser',
  desktop: 'Desktop',
  mac: 'Mac',
  server: 'Server',
} as const;
export const KIND_ICON: Record<keyof typeof KIND_WORD, LucideIcon> = {
  ios: Smartphone,
  android: Smartphone,
  web: Globe,
  desktop: Laptop,
  mac: Laptop,
  server: Server,
};

export const MODE_WORD: Record<Mode, string> = { run: 'Run', pause: 'Pause', off: 'Off' };
const MODE_ICON: Record<Mode, ReactNode> = { run: <Play size={12} />, pause: <Pause size={12} />, off: <Circle size={12} /> };

/** A time today as 14:02, an older one as its date too. */
const clock = (at: string) => {
  const d = new Date(at);
  const hm = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  return d.toDateString() === new Date().toDateString() ? hm : `${d.toLocaleDateString([], { month: 'short', day: 'numeric' })} ${hm}`;
};

/** The state's word with its own shape, never colour alone. */
export function StateWord({ row }: { row: Pick<DeviceRow, 'state' | 'since'> }) {
  const word: Record<State, ReactNode> = {
    running: (
      <>
        <CircleDot size={12} /> Running
      </>
    ),
    paused: (
      <>
        <Pause size={12} /> Paused
      </>
    ),
    off: (
      <>
        <Circle size={12} /> Off
      </>
    ),
    offline: (
      <>
        <TriangleAlert size={12} /> {row.since ? `Offline since ${clock(row.since)}` : 'Offline'}
      </>
    ),
    starting: (
      <>
        <Loader size={12} /> Starting…
      </>
    ),
    connected: <>Connected</>,
    seen: <>{row.since ? `Seen ${ago(row.since)} ago` : 'Never seen'}</>,
  };
  return (
    <span
      data-state={row.state}
      className={`inline-flex items-center gap-1.5 whitespace-nowrap ${row.state === 'offline' ? 'text-warning' : ''}`}
    >
      {word[row.state]}
    </span>
  );
}

/** A refused write says why, as a toast: the control itself has no room for it. */
export const refused = (e: unknown) => void toast(String(e), { tone: 'warning' });

/** What Off does, said once in its confirm. */
export const OFF_TEXT = 'Agents here finish their turn and go on on another device.';

/** Run · Pause · Off as one segmented control. Off asks first; the master's own Mac is never Off. Only the device's own person sets it. */
export function ModeControl({ row, setMode, busy }: { row: DeviceRow; setMode: (m: Mode) => Promise<unknown>; busy: boolean }) {
  const pick = (m: Mode) => {
    if (m === row.mode) return;
    if (m !== 'off') return void setMode(m).catch(refused);
    void openModal<void>((close) => (
      <Confirm
        title={`Turn ${row.name} off?`}
        text={OFF_TEXT}
        act="Turn off"
        busyWord="Turning off…"
        go={() => setMode('off')}
        close={() => close()}
      />
    ));
  };
  return (
    <div
      role="radiogroup"
      aria-label={`${row.name}: Run, Pause or Off`}
      className={`inline-flex shrink-0 overflow-hidden rounded-md border border-border text-xs ${busy || row.state === 'offline' ? 'opacity-50' : ''}`}
      onClick={(e) => e.stopPropagation()}
    >
      {MODES.map((m) => (
        <button
          key={m}
          type="button"
          role="radio"
          aria-checked={row.mode === m}
          disabled={busy || !row.mine || (row.master && m === 'off')}
          title={row.master && m === 'off' ? 'The master always runs.' : undefined}
          onClick={() => pick(m)}
          className="flex h-7 items-center gap-1 border-l border-border px-2.5 first:border-l-0 text-muted enabled:hover:text-fg disabled:opacity-40 aria-checked:bg-fg aria-checked:text-bg aria-checked:opacity-100"
        >
          {MODE_ICON[m]}
          {MODE_WORD[m]}
        </button>
      ))}
    </div>
  );
}

/** A confirm that waits for the daemon: it stays, dimmed, until the answer, and a refusal says why in it. */
export function Confirm({
  title,
  text,
  act,
  busyWord,
  danger,
  go,
  close,
}: {
  title: string;
  text: string;
  act: string;
  busyWord: string;
  danger?: boolean;
  go: () => Promise<unknown>;
  close: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [why, setWhy] = useState('');
  return (
    <ModalFrame
      title={title}
      size="sm"
      onClose={busy ? undefined : close}
      footer={
        <>
          <Button disabled={busy} onClick={close}>
            Cancel
          </Button>
          <Button
            variant={danger ? 'danger' : 'primary'}
            disabled={busy}
            onClick={() => {
              setBusy(true);
              setWhy('');
              go().then(close, (e: unknown) => (setWhy(String(e)), setBusy(false)));
            }}
          >
            {busy ? busyWord : act}
          </Button>
        </>
      }
    >
      <p>{text}</p>
      {why && (
        <p className="pt-2 text-xs text-danger" role="alert">
          {why}
        </p>
      )}
    </ModalFrame>
  );
}

const again = 'To use it again it has to be paired anew.';

export function confirmRevoke(rows: DeviceRow[], revoke: (id: string) => Promise<unknown>) {
  const one = rows.length === 1 ? rows[0]! : null;
  void openModal<void>((close) => (
    <Confirm
      title={one ? `Revoke ${one.name}?` : `Revoke ${rows.length} devices?`}
      text={`${one?.here ? 'This is the device you are using: it loses its access at once.' : `${one ? one.name : `The ${rows.length} devices`} ${one ? 'loses its' : 'lose their'} access at once.`} ${again}`}
      act="Revoke"
      busyWord="Revoking…"
      danger
      go={async () => {
        for (const r of rows) await revoke(r.id);
      }}
      close={() => close()}
    />
  ));
}

/** The device an agent is on, or that it is offline there and since when. */
export function DeviceLine({ device }: { device: { name: string; online: boolean; since: string | null } }) {
  const Icon = device.online ? Laptop : TriangleAlert;
  return (
    <span data-agent-device className={`flex items-center gap-1.5 text-xs ${device.online ? 'text-muted' : 'text-warning'}`}>
      <Icon size={12} />
      {device.online ? `On ${device.name}` : `Offline: ${device.name}${device.since ? ` since ${clock(device.since)}` : ''}`}
    </span>
  );
}
