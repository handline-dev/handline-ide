// Pairing this browser: the code from `hl device pair`, or a link from Settings › Devices › Connect a device (`#pair=<code>`), whose page
// first offers the Handline app, which opens the same link as handline://, or this browser, which pairs by itself.
import type { KV } from '@handline/app-core';
import { webSocketTransport } from '@handline/client';
import { useEffect, useState } from 'react';
import { appLink, deviceName, invite as inviteOf, pairWith, type Invite } from '#src/connection/pairing.ts';
import { Button } from '#src/ui/Button.tsx';
import { ChoiceCards } from '#src/ui/ChoiceCards.tsx';
import { AppWindow, Globe } from 'lucide-react';

export function Pair({
  url,
  storage,
  lost,
  onPaired,
}: {
  url: string;
  storage: KV | null;
  lost: boolean;
  onPaired: (token: string) => void;
}) {
  const [invite] = useState(() => inviteOf(location.hash));
  useEffect(() => {
    if (invite) history.replaceState(null, '', location.pathname + location.search); // the code is not left in the address bar
  }, [invite]);
  const [here, setHere] = useState(false);
  if (invite && !here) return <Landing invite={invite} onBrowser={() => setHere(true)} />;
  return <PairForm url={url} storage={storage} lost={lost} onPaired={onPaired} invite={invite?.code ?? null} />;
}

/** The link's page: open it in the Handline app, or use it in this browser. */
function Landing({ invite, onBrowser }: { invite: Invite; onBrowser: () => void }) {
  return (
    <div className="mx-auto mt-32 flex max-w-md flex-col gap-4 px-4 text-sm text-fg">
      <h1 className="text-lg font-semibold">Open this link in</h1>
      <ChoiceCards
        label="Open this link in"
        onPick={(v) => (v === 'app' ? (location.href = appLink(location.host, invite)) : onBrowser())}
        cards={[
          { value: 'app', icon: AppWindow, label: 'The Handline app' },
          { value: 'browser', icon: Globe, label: 'This browser' },
        ]}
      />
    </div>
  );
}

function PairForm({
  url,
  storage,
  lost,
  onPaired,
  invite,
}: {
  url: string;
  storage: KV | null;
  lost: boolean;
  onPaired: (token: string) => void;
  invite: string | null;
}) {
  const [text, setText] = useState(invite ?? '');
  const [busy, setBusy] = useState(!!invite);
  const [error, setError] = useState('');
  const pair = (code: string) =>
    pairWith(url, code, storage, webSocketTransport, deviceName()).then(onPaired, (e: unknown) => {
      setError(e instanceof Error ? e.message : String(e));
      setBusy(false);
    });
  const submit = (code: string) => {
    setBusy(true);
    setError('');
    void pair(code);
  };
  useEffect(() => {
    if (invite) void pair(invite);
  }, []); // eslint-disable-line react-hooks/exhaustive-deps -- once, for the link the page opened with

  return (
    <form
      className="mx-auto mt-32 flex max-w-sm flex-col gap-3 px-4 text-sm text-fg"
      onSubmit={(e) => {
        e.preventDefault();
        if (text.trim() && !busy) submit(text);
      }}
    >
      <h1 className="text-lg font-semibold">Pair this browser</h1>
      {lost && <p className="text-muted">This browser is no longer paired.</p>}
      <p className="text-muted">
        In a terminal run <code>hl device pair</code> and paste the code or the link here.
      </p>
      <input
        autoFocus
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder="Pairing code"
        aria-label="Pairing code"
        className="h-8 rounded-md border border-border bg-bg px-2 outline-none placeholder:text-muted focus:border-accent"
      />
      <Button type="submit" variant="primary" disabled={busy || !text.trim()}>
        {busy ? 'Pairing…' : 'Pair'}
      </Button>
      {error && (
        <p role="alert" className="text-danger">
          {error}
        </p>
      )}
    </form>
  );
}
