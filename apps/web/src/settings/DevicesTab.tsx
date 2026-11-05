// Settings > Devices: every device that connects to this computer in one table - this one first, who is connected, when the others were last
// seen (stale after 30 days), and Revoke behind a confirm that waits for the daemon.
import { MonitorSmartphone, Plus } from 'lucide-react';
import { useState } from 'react';
import { ago } from '@handline/app-core';
import type { Device } from '@handline/client';
import { openPairSheet } from '#src/settings/PairSheet.tsx';
import { useDevices } from '#src/settings/data/devices.ts';
import { Button } from '#src/ui/Button.tsx';
import { ModalFrame, openModal } from '#src/ui/modal.tsx';
import { Page } from '#src/ui/Page.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';

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
  return (
    <span className={d.stale ? 'text-warning' : 'text-muted'}>
      Seen {ago(d.last_seen)} ago{d.stale ? ' · stale' : ''}
    </span>
  );
}

/** The confirm waits for the daemon: it stays, dimmed, until the answer, and a refusal says why in it. */
function Revoke({ d, close, revoke }: { d: Row; close: () => void; revoke: (id: string) => Promise<unknown> }) {
  const [busy, setBusy] = useState(false);
  const [why, setWhy] = useState('');
  return (
    <ModalFrame
      title={`Revoke ${d.name}?`}
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
              revoke(d.id).then(close, (e: unknown) => (setWhy(String(e)), setBusy(false)));
            }}
          >
            {busy ? 'Revoking…' : 'Revoke'}
          </Button>
        </>
      }
    >
      <p>
        {d.current ? 'This is the device you are using: it loses its access at once. ' : `${d.name} loses its access at once. `}
        To use it again it has to be paired anew.
      </p>
      {why && (
        <p className="pt-2 text-xs text-danger" role="alert">
          {why}
        </p>
      )}
    </ModalFrame>
  );
}

export function DevicesTab() {
  const q = useDevices();
  const rows = sortDevices(q.devices);
  return (
    <ScrollFade keep="settings-devices" className="min-h-0 flex-1">
      <Page>
        <div className="flex items-center justify-between gap-3 py-4">
          <p className="text-sm text-muted">Browsers and phones that connect to this computer. They run no agents.</p>
          <Button variant="primary" onClick={() => void openPairSheet()}>
            <Plus size={14} />
            Connect a device
          </Button>
        </div>
        {q.loading && <p className="text-sm text-muted">Loading devices…</p>}
        {q.error && <p className="text-sm text-danger">Devices did not load.</p>}
        {!q.loading && !q.error && rows.length === 0 && <p className="text-sm text-muted">No devices yet.</p>}
        {rows.length > 0 && (
          <table className="w-full text-sm">
            <thead className="text-left text-xs text-muted">
              <tr>
                <th className="py-2 font-medium">Device</th>
                <th className="py-2 font-medium">Status</th>
                <th className="py-2" />
              </tr>
            </thead>
            <tbody>
              {rows.map((d) => (
                <tr key={d.id} data-device={d.id} className={`border-t border-border ${q.pending === d.id ? 'opacity-50' : ''}`}>
                  <td className="py-2.5 pr-3">
                    <span className="flex items-center gap-2">
                      <MonitorSmartphone size={16} className="shrink-0 text-muted" />
                      <span className="[overflow-wrap:anywhere]">{d.name}</span>
                      {d.current && <span className="rounded-full border border-border px-2 text-xs text-muted">This device</span>}
                    </span>
                    {(d.kind || d.client) && (
                      <span className="block pl-6 text-xs text-muted">
                        {[d.kind && KIND_WORD[d.kind], d.client].filter(Boolean).join(' · ')}
                      </span>
                    )}
                  </td>
                  <td className="py-2.5 pr-3">{seen(d)}</td>
                  <td className="py-2.5 text-right">
                    <Button
                      variant="ghost"
                      disabled={q.pending !== null}
                      aria-label={`Revoke ${d.name}`}
                      onClick={() => void openModal<void>((close) => <Revoke d={d} close={() => close()} revoke={q.revoke} />)}
                    >
                      Revoke
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Page>
    </ScrollFade>
  );
}
