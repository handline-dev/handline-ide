import { countdown } from '#src/format/time.ts';
// Connect a device: it asks nothing. Opening it makes a code; Handline picks the address (the Tailscale name, else the Wi-Fi address) and
// the sheet shows the one link, its QR and the code with its expiry. A Wi-Fi link is for another Mac's app only, which pins its
// certificate, so it gets no QR: no phone can open it. The sheet closes with a notice when the device that used the code appears.
import qrcode from 'qrcode-generator';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useDevices } from '#src/settings/data/devices.ts';
import { usePair, type Pairing } from '#src/settings/data/pair.ts';
import { Button } from '#src/ui/Button.tsx';
import { useCopy } from '#src/ui/copy.ts';
import { ModalFrame, openModal } from '#src/ui/modal.tsx';
import { toast } from '#src/ui/toast.tsx';

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

function Invite({ invite, left }: { invite: Pairing; left: number }) {
  const { copied, copy } = useCopy();
  // A self-made certificate's fingerprint marks a Wi-Fi link: a Mac app pins it, a browser could not tell it from a fake.
  const wifi = invite.fingerprint !== '';
  return (
    <div className="flex flex-wrap items-start gap-5">
      {!wifi && <Qr text={invite.link} />}
      <div className="flex min-w-56 flex-1 flex-col gap-2">
        <span className="text-xs text-muted">Code</span>
        <code className="text-2xl font-semibold tracking-widest tabular-nums">{invite.code}</code>
        <span className="text-xs text-muted">Link</span>
        <code className="rounded-md bg-surface px-3 py-2 text-xs [overflow-wrap:anywhere]">{invite.link}</code>
        <Button className="w-fit" onClick={() => copy(invite.link, 'the link')}>
          {copied ? 'Copied' : 'Copy link'}
        </Button>
        {/* Two lines, no more: what works without Tailscale, and what needs it. */}
        <span className="text-xs text-muted">
          On the same Wi-Fi, another Mac&apos;s Handline app connects without Tailscale.
          <br />
          Phones, browsers and anything away from home need{' '}
          <a href="https://tailscale.com/download" target="_blank" rel="noreferrer" className="text-accent hover:underline">
            Tailscale
          </a>
          .
        </span>
        <span className="text-xs text-muted">
          Valid for <b className="tabular-nums text-fg">{countdown(left)}</b>
        </span>
      </div>
    </div>
  );
}

function Sheet({ close }: { close: () => void }) {
  const pair = usePair();
  const devices = useDevices();
  const [known] = useState(() => new Set(devices.rows.filter((d) => !d.master).map((d) => d.id)));
  const [invite, setInvite] = useState<Pairing | null>(null);
  const [left, setLeft] = useState(0);

  // The time left is set with the invite, so a new code never renders a frame as expired.
  const make = () =>
    void pair.start().then(
      (p) => {
        setLeft(Math.max(0, p.expiresAt - Date.now()));
        setInvite(p);
      },
      () => undefined,
    );
  // One code per opening: the guard keeps a remount in development from making a second.
  const made = useRef(false);
  useEffect(() => {
    if (made.current) return;
    made.current = true;
    make();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps -- once, as the sheet opens

  useEffect(() => {
    if (!invite) return;
    const tick = () => setLeft(Math.max(0, invite.expiresAt - Date.now()));
    tick();
    const t = setInterval(tick, 500);
    return () => clearInterval(t);
  }, [invite]);

  const joined = devices.rows.find((d) => !d.master && !known.has(d.id));
  useEffect(() => {
    if (!joined) return;
    toast(`${joined.name} connected.`);
    close();
  }, [joined]); // eslint-disable-line react-hooks/exhaustive-deps -- once per new device

  const live = invite !== null && left > 0;
  const expired = invite !== null && left === 0;
  const footer = (
    <div className="flex w-full justify-end gap-2">
      {live ? (
        <Button variant="primary" onClick={close}>
          Done
        </Button>
      ) : (
        <Button variant="primary" onClick={make} disabled={pair.pending || (!expired && !pair.error)}>
          New code
        </Button>
      )}
    </div>
  );
  return (
    <ModalFrame title="Connect a device" size="lg" onClose={close} footer={footer}>
      {live ? (
        <Invite invite={invite} left={left} />
      ) : (
        <div className="flex min-h-40 flex-col justify-center gap-2 text-sm">
          {pair.error ? (
            <p className="text-danger" role="alert">
              {pair.error}
            </p>
          ) : expired && !pair.pending ? (
            <p role="status">This code expired.</p>
          ) : (
            <p className="text-muted">Making a code…</p>
          )}
        </div>
      )}
    </ModalFrame>
  );
}

export const openPairSheet = () => openModal<void>((close) => <Sheet close={() => close()} />);
