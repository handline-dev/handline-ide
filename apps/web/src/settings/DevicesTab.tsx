// Settings > Devices: every device that connects to this computer in one table - this one first, who is connected, when the others were last
// seen (stale after 30 days), and Revoke behind a confirm that waits for the daemon.
import { ListLoading } from '#src/ui/Loading.tsx';
import { ChevronRight, MonitorSmartphone, Plus } from 'lucide-react';
import { useState } from 'react';
import { ago } from '@handline/app-core';
import type { Device } from '@handline/client';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { openPairSheet } from '#src/settings/PairSheet.tsx';
import { useDevices } from '#src/settings/data/devices.ts';
import { Button } from '#src/ui/Button.tsx';
import { ModalFrame, openModal } from '#src/ui/modal.tsx';

const KIND_WORD = { ios: 'iPhone / iPad', android: 'Android', web: 'Browser', desktop: 'Desktop' } as const;

type Row = Device & { stale: boolean };

/** This device first, then connected, then the most recently seen. */
export const sortDevices = (list: Row[]): Row[] =>
  [...list].sort(
    (a, b) =>
      Number(!!b.current) - Number(!!a.current) ||
      Number(b.connected) - Number(a.connected) ||
      Date.parse(b.last_seen ?? b.created) - Date.parse(a.last_seen ?? a.created),
  );

function seen(d: Row) {
  if (d.connected) return <span className="text-fg">Connected</span>;
  if (!d.last_seen) return <span className="text-muted">Never seen</span>;
  return <span className={d.stale ? 'text-warning' : 'text-muted'}>Seen {ago(d.last_seen)} ago</span>;
}

/** The confirm waits for the daemon: it stays, dimmed, until the answer, and a refusal says why in it. */
function RevokeConfirm({ title, text, go, close }: { title: string; text: string; go: () => Promise<unknown>; close: () => void }) {
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
            variant="danger"
            disabled={busy}
            onClick={() => {
              setBusy(true);
              setWhy('');
              go().then(close, (e: unknown) => (setWhy(String(e)), setBusy(false)));
            }}
          >
            {busy ? 'Revoking…' : 'Revoke'}
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

function DeviceRow({ d, pending, revoke }: { d: Row; pending: boolean; revoke: (id: string) => Promise<unknown> }) {
  return (
    <div data-device={d.id} className={`flex items-center gap-3 px-4 py-3 text-sm ${pending ? 'opacity-50' : ''}`}>
      <MonitorSmartphone size={16} className="shrink-0 text-muted" />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="[overflow-wrap:anywhere]">{d.name}</span>
          {d.current && <span className="rounded-full border border-border px-2 text-xs text-muted">This device</span>}
        </div>
        {(d.kind || d.client) && (
          <div className="text-xs text-muted">{[d.kind && KIND_WORD[d.kind], d.client].filter(Boolean).join(' · ')}</div>
        )}
      </div>
      <span className="shrink-0">{seen(d)}</span>
      <Button
        disabled={pending}
        aria-label={`Revoke ${d.name}`}
        title="Takes this device's access away"
        onClick={() =>
          void openModal<void>((close) => (
            <RevokeConfirm
              title={`Revoke ${d.name}?`}
              text={`${d.current ? 'This is the device you are using: it loses its access at once.' : `${d.name} loses its access at once.`} ${again}`}
              go={() => revoke(d.id)}
              close={() => close()}
            />
          ))
        }
      >
        Revoke
      </Button>
    </div>
  );
}

export function DevicesTab() {
  const q = useDevices();
  const rows = sortDevices(q.devices);
  const [showOld, setShowOld] = useState(false);
  const recent = rows.filter((d) => !d.stale);
  const old = rows.filter((d) => d.stale);
  const list = (items: Row[]) => (
    <div className="divide-y divide-border rounded-lg border border-border">
      {items.map((d) => (
        <DeviceRow key={d.id} d={d} pending={q.pending !== null} revoke={q.revoke} />
      ))}
    </div>
  );
  return (
    <SettingsPage
      keep="settings-devices"
      title="Devices"
      note="Browsers and phones that connect to this computer. They run no agents."
      action={
        <Button variant="primary" onClick={() => void openPairSheet()}>
          <Plus size={14} />
          Connect a device
        </Button>
      }
    >
      {q.loading && <ListLoading rows={3} label="Loading devices" />}
      {q.error && <p className="text-sm text-danger">Devices did not load.</p>}
      {!q.loading && !q.error && rows.length === 0 && <p className="text-sm text-muted">No devices yet.</p>}
      {recent.length > 0 && list(recent)}
      {old.length > 0 && (
        <div className="flex flex-col gap-2">
          <div className="flex items-center gap-3 text-sm">
            <button
              type="button"
              aria-expanded={showOld}
              onClick={() => setShowOld((v) => !v)}
              className="flex flex-1 items-center gap-2 text-left text-muted hover:text-fg"
            >
              <ChevronRight size={14} className={showOld ? 'rotate-90' : ''} />
              {old.length} not seen for over 30 days
            </button>
            <Button
              disabled={q.pending !== null}
              onClick={() =>
                void openModal<void>((close) => (
                  <RevokeConfirm
                    title={`Revoke ${old.length} devices?`}
                    text={`The ${old.length} devices not seen for over 30 days lose their access at once. ${again}`}
                    go={async () => {
                      for (const d of old) await q.revoke(d.id);
                    }}
                    close={() => close()}
                  />
                ))
              }
            >
              Revoke all {old.length}
            </Button>
          </div>
          {showOld && list(old)}
        </div>
      )}
    </SettingsPage>
  );
}
