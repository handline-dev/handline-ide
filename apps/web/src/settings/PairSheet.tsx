import { countdown } from '#src/format/time.ts';
// Connect a device, in two steps: first the address the device reaches this Mac by (Tailscale first when it is here, else the Wi-Fi
// address); Next makes a code for it, and the sheet shows the link, its QR and the code with its expiry. A Wi-Fi link is for another Mac's app only, which pins its
// certificate, so it gets no QR: no phone can open it. The sheet closes with a notice when the device that used the code appears.
import qrcode from 'qrcode-generator';
import { useEffect, useMemo, useState } from 'react';
import { useDevices } from '#src/settings/data/devices.ts';
import { usePair, type Pairing } from '#src/settings/data/pair.ts';
import { Button } from '#src/ui/Button.tsx';
import { ChoiceList } from '#src/ui/ChoiceList.tsx';
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

/** The Tailscale choice while this Mac has no Tailscale address: shown faded, a click opens how to set it up. */
const TAILSCALE = 'Tailscale';

/** The addresses a door listens on, Tailscale's first. */
function useHosts() {
  const all = (Hooks.useConnectionAddresses().data?.addresses ?? []).filter((a) => a.listening);
  // Its IP, or its name when only the name is listening.
  const ts = all.find((a) => a.kind === 'tailscale') ?? all.find((a) => a.kind === 'tailscale_name');
  const rest = all.filter((a) => a.kind !== 'tailscale' && a.kind !== 'tailscale_name');
  const listed = ts ? [ts, ...rest] : rest;
  // The network port each is on (en0, utun3), for whoever needs to tell them apart.
  const ports = new Map(listed.map((a) => [a.host, a.interface]));
  return { ts: ts?.host, hosts: listed.map((a) => a.host), ports };
}

/** How to set up Tailscale, in a popup: the dimmed Tailscale row opens it while this Mac has none. */
const openTailscaleGuide = () =>
  void openModal<void>((close) => (
    <ModalFrame title="Set up Tailscale" onClose={() => close()} footer={<Button onClick={() => close()}>Close</Button>}>
      <ol className="list-decimal pl-5 text-sm">
        <li>
          Install{' '}
          <a href="https://tailscale.com/download" target="_blank" rel="noreferrer" className="text-accent hover:underline">
            Tailscale
          </a>{' '}
          on this Mac and on the device, and sign in to the same account on both.
        </li>
        <li>Open Connect a device again: its Tailscale address is then a choice.</li>
      </ol>
    </ModalFrame>
  ));

/** Step one: the address the device connects to, each by its IP; Tailscale's carries a badge. Without Tailscale its row is dimmed
 *  and opens how to set it up. */
function Hosts({ value, onPick }: { value: string; onPick: (host: string) => void }) {
  const { ts, hosts, ports } = useHosts();
  const options = ts ? hosts : [...hosts, TAILSCALE];
  return (
    <div className="flex min-h-40 flex-col gap-3">
      <span className="text-sm">Which address should the device connect to?</span>
      <ChoiceList
        label="Address"
        value={value}
        rows={options.map((h) => ({
          value: h,
          label: h === TAILSCALE ? '–' : h,
          badges: [ports.get(h), h === ts || h === TAILSCALE ? 'tailscale' : ''].filter((b): b is string => !!b),
          aside: h === TAILSCALE ? 'not set up' : undefined,
          dim: h === TAILSCALE,
        }))}
        onPick={(h) => (h === TAILSCALE ? openTailscaleGuide() : onPick(h))}
      />
    </div>
  );
}

function Invite({ invite, left }: { invite: Pairing; left: number }) {
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
  // Step one picks the address; Next makes the code for it. Until a pick, Tailscale or else the first address.
  const { hosts } = useHosts();
  const [picked, setPicked] = useState<string | null>(null);
  const host = picked ?? hosts[0] ?? '';
  const [step, setStep] = useState<'address' | 'code'>('address');
  const next = () => {
    setStep('code');
    make(host);
  };

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
      {step === 'code' && (
        <Button
          onClick={() => {
            setStep('address');
            setInvite(null);
          }}
        >
          Back
        </Button>
      )}
      {step === 'address' ? (
        <Button variant="primary" onClick={next} disabled={!host}>
          Next
        </Button>
      ) : live ? (
        <Button variant="primary" onClick={close}>
          Done
        </Button>
      ) : (
        <Button variant="primary" onClick={() => make(host)} disabled={pair.pending || (!expired && !pair.error)}>
          New code
        </Button>
      )}
    </div>
  );
  return (
    <ModalFrame title="Connect a device" size="lg" onClose={close} footer={footer}>
      {step === 'address' ? (
        <Hosts value={host} onPick={setPicked} />
      ) : live ? (
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
