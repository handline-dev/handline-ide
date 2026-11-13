// The pair sheet: pick the one address to share (.local, a LAN IP, a Tailscale IP or hostname), make a code, and show it with its expiry, a QR and
// the link. The code and link live only in this sheet. It closes with a notice when the device that used the code appears in the list.
import type { NetworkAddress } from '@handline/client';
import qrcode from 'qrcode-generator';
import { useEffect, useMemo, useState } from 'react';
import { useDevices } from '#src/settings/data/devices.ts';
import { useOpenAddress, usePair, usePairAddresses } from '#src/settings/data/pair.ts';
import { Button } from '#src/ui/Button.tsx';
import { useCopy } from '#src/ui/copy.ts';
import { openModal } from '#src/ui/modal.tsx';
import { StepModal } from '#src/ui/StepModal.tsx';
import { toast } from '#src/ui/toast.tsx';

const KIND_WORD: Record<NetworkAddress['kind'], string> = {
  mdns: 'Name on this network (.local)',
  lan: 'Local network IP',
  tailscale: 'Tailscale IP',
  tailscale_name: 'Tailscale name',
};
const ORDER: NetworkAddress['kind'][] = ['mdns', 'lan', 'tailscale', 'tailscale_name'];

const clock = (ms: number) => `${Math.floor(ms / 60000)}:${String(Math.floor(ms / 1000) % 60).padStart(2, '0')}`;

/** The text as an SVG QR, drawn on white so a scanner reads it in the dark theme too. */
function Qr({ text }: { text: string }) {
  const svg = useMemo(() => {
    const q = qrcode(0, 'M');
    q.addData(text);
    q.make();
    return q.createSvgTag({ cellSize: 6, margin: 0, scalable: true });
  }, [text]);
  return (
    <div
      role="img"
      aria-label="Pairing QR code"
      style={{ background: 'white' }}
      className="size-40 shrink-0 rounded-md p-2.5 [&>svg]:size-full"
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  );
}

function Sheet({ close }: { close: () => void }) {
  const { addresses, loading } = usePairAddresses();
  const pair = usePair();
  const openAddress = useOpenAddress();
  const [openError, setOpenError] = useState<string | null>(null);
  const devices = useDevices();
  const [known] = useState(() => new Set(devices.devices.map((d) => d.id)));
  const [chosen, setChosen] = useState<string | null>(null);
  const [invite, setInvite] = useState<{ code: string; until: number; uri: string; link: string | null } | null>(null);
  const [left, setLeft] = useState(0);
  const { copied, copy } = useCopy();
  const list = [...addresses].sort((a, b) => ORDER.indexOf(a.kind) - ORDER.indexOf(b.kind));
  const pick = chosen ?? (list.find((a) => a.listening) ?? list[0])?.host ?? null;

  useEffect(() => {
    if (!invite) return;
    const tick = () => setLeft(Math.max(0, invite.until - Date.now()));
    tick();
    const t = setInterval(tick, 500);
    return () => clearInterval(t);
  }, [invite]);

  const joined = devices.devices.find((d) => !known.has(d.id));
  useEffect(() => {
    if (!joined) return;
    toast(`${joined.name} connected.`);
    close();
  }, [joined]); // eslint-disable-line react-hooks/exhaustive-deps -- once per new device

  const make = () => {
    if (!pick) return;
    setOpenError(null);
    openAddress(pick)
      .then(() => pair.start(pick))
      .then(
        (r) => setInvite({ code: r.code, until: r.expiresAt, uri: r.uri, link: r.link }),
        (e: unknown) => (pair.error ? undefined : setOpenError(String(e))),
      );
  };
  const live = invite && left > 0;
  const pickPage = (
    <div className="flex flex-col gap-3">
      {invite && (
        <p className="text-sm" role="status">
          This invite expired.
        </p>
      )}
      <p className="text-sm text-muted">Pick the address the other device will connect to.</p>
      {loading && <p className="text-sm text-muted">Looking for addresses…</p>}
      {!loading && list.length === 0 && (
        <p className="text-sm text-muted">No address is open to other devices: the app listens on this computer only.</p>
      )}
      <div role="radiogroup" aria-label="Address to share" className="flex flex-col gap-1">
        {list.map((a) => (
          <button
            key={a.host}
            type="button"
            role="radio"
            aria-checked={pick === a.host}
            onClick={() => setChosen(a.host)}
            className="flex items-center justify-between gap-3 rounded-md border border-border px-3 py-2 text-left text-sm hover:bg-hover aria-checked:border-accent"
          >
            <span className="truncate">{a.host}</span>
            <span className="shrink-0 text-xs text-muted">{KIND_WORD[a.kind]}</span>
          </button>
        ))}
      </div>
      {openError && (
        <p className="text-xs text-danger" role="alert">
          {openError}
        </p>
      )}
      {pair.error && (
        <p className="text-xs text-danger" role="alert">
          {pair.error}
        </p>
      )}
    </div>
  );
  const codePage = live && (
    <div className="flex flex-wrap items-start gap-5">
      <Qr text={invite.link ?? invite.uri} />
      <div className="flex min-w-56 flex-1 flex-col gap-2">
        <span className="text-xs text-muted">Code</span>
        <code className="text-2xl font-semibold tracking-widest tabular-nums">{invite.code}</code>
        {invite.link && (
          <>
            <span className="text-xs text-muted">Or open this on the other device</span>
            <code className="rounded-md bg-surface px-3 py-2 text-xs [overflow-wrap:anywhere]">{invite.link}</code>
            <Button className="w-fit" onClick={() => copy(invite.link!, 'the link')}>
              {copied ? 'Copied' : 'Copy link'}
            </Button>
          </>
        )}
        <span className="text-xs text-muted">
          Valid for <b className="tabular-nums text-fg">{clock(left)}</b>
        </span>
        <span className="text-xs text-muted">The other device shows a certificate warning the first time.</span>
      </div>
    </div>
  );
  return (
    <StepModal
      title="Connect a device"
      description="Anyone with the code for 5 minutes gets full control of this computer's agents. It works once."
      onClose={close}
      step={live ? 1 : 0}
      steps={[
        {
          body: pickPage,
          next: {
            label: pair.pending ? 'Making a code…' : invite ? 'New code' : 'Make a code',
            onClick: make,
            disabled: !pick || pair.pending,
          },
        },
        { body: codePage, back: { label: 'Back', onClick: () => setInvite(null) }, next: { label: 'Done', onClick: close } },
      ]}
    />
  );
}

export const openPairSheet = () => openModal<void>((close) => <Sheet close={() => close()} />);
