import { countdown } from '#src/format/time.ts';
// Connect a device: it asks nothing. Opening it makes a code; Handline picks the address (the Tailscale name, else the Wi-Fi address) and
// the sheet shows the one link, its QR and the code with its expiry. A Wi-Fi link is for another Mac's app only, which pins its
// certificate, so it gets no QR: no phone can open it. The sheet closes with a notice when the device that used the code appears.
import qrcode from 'qrcode-generator';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useDevices } from '#src/settings/data/devices.ts';
import { usePair, type Pairing } from '#src/settings/data/pair.ts';
import { Button } from '#src/ui/Button.tsx';
import { Segmented } from '#src/ui/Segmented.tsx';
import { Hooks } from '@handline/app-data';
import { Secret } from '#src/ui/Secret.tsx';
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

const hostOf = (link: string) => {
  try {
    return new URL(link).hostname.replace(/^\[|\]$/g, ''); // an IPv6 host without its brackets, as the addresses list it
  } catch {
    return '';
  }
};

/** The Tailscale choice while this Mac has no Tailscale address: shown faded, a click opens how to set it up. */
const TAILSCALE = 'Tailscale';

/** The address the link names, as a small switch over the ones a door listens on: the one selected is Handline's pick until another is
 *  clicked, which makes a new code for another. Tailscale is always a choice: without it, faded, and a click shows how to set it up. */
function Hosts({ current, onHost, busy }: { current: string; onHost: (host: string) => void; busy: boolean }) {
  const [guide, setGuide] = useState(false);
  const all = (Hooks.useConnectionAddresses().data?.addresses ?? []).filter((a) => a.listening);
  // Tailscale is one choice, named so: its name when MagicDNS gives one, else its address.
  const ts = all.find((a) => a.kind === 'tailscale_name') ?? all.find((a) => a.kind === 'tailscale');
  const hosts = all.filter((a) => a === ts || (a.kind !== 'tailscale' && a.kind !== 'tailscale_name')).map((a) => a.host);
  // In the daemon's order, so the choices never move when one is picked.
  const listed = hosts.includes(current) || !current ? hosts : [current, ...hosts];
  const options = ts ? listed : [...listed, TAILSCALE];
  const word = (h: string) => (h === ts?.host ? 'Tailscale' : h === TAILSCALE ? 'Tailscale (not set up)' : h);
  if (options.length < 2) return null;
  return (
    <>
      <Segmented
        label="Link address"
        disabled={busy}
        value={guide ? TAILSCALE : current}
        options={options}
        word={word}
        dim={(h) => h === TAILSCALE}
        onChange={(h) => (h === TAILSCALE ? setGuide(true) : (setGuide(false), h !== current && onHost(h)))}
      />
      {guide && (
        <ol className="list-decimal pl-5 text-xs text-muted">
          <li>
            Install{' '}
            <a href="https://tailscale.com/download" target="_blank" rel="noreferrer" className="text-accent hover:underline">
              Tailscale
            </a>{' '}
            on this Mac and on the device, and sign in to the same account on both.
          </li>
          <li>Open Connect a device again: Tailscale is then a choice here.</li>
        </ol>
      )}
    </>
  );
}

function Invite({ invite, left, onHost, busy }: { invite: Pairing; left: number; onHost: (host: string) => void; busy: boolean }) {
  // A self-made certificate's fingerprint marks a Wi-Fi link: a Mac app pins it, a browser could not tell it from a fake.
  const wifi = invite.fingerprint !== '';
  return (
    <div className="flex flex-wrap items-start gap-5">
      {!wifi && <Qr text={invite.link} />}
      <div className="flex min-w-56 flex-1 flex-col gap-2">
        {/* The code and the link, each in its own box: hidden until its eye, a click copies it. */}
        <span className="text-xs text-muted">Code</span>
        <Secret value={invite.code} name="the code">
          <code className="text-2xl font-semibold tracking-widest tabular-nums">{invite.code}</code>
        </Secret>
        <span className="text-xs text-muted">Link</span>
        <Hosts current={hostOf(invite.link)} onHost={onHost} busy={busy} />
        <Secret value={invite.link} name="the link">
          <code className="text-xs [overflow-wrap:anywhere]">{invite.link}</code>
        </Secret>
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
  const make = (host?: string) =>
    void pair.start(host).then(
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
        <Button variant="primary" onClick={() => make()} disabled={pair.pending || (!expired && !pair.error)}>
          New code
        </Button>
      )}
    </div>
  );
  return (
    <ModalFrame title="Connect a device" size="lg" onClose={close} footer={footer}>
      {live ? (
        <Invite invite={invite} left={left} onHost={make} busy={pair.pending} />
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
