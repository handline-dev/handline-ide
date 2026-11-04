// Opens the daemon: a listener that wants a device token gets the pairing screen until this browser holds one.
import { Store, type KV } from '@handline/app-core';
import { DataProvider, SKELETON_CSS, type BoxComponent } from '@handline/app-data';
import { DaemonProvider } from '@handline/app-react';
import { DaemonClient, webSocketTransport } from '@handline/client';
import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { browserStorage, daemonUrl } from '#src/connection/daemon.ts';
import { deviceName, forgetToken, inviteCode, loadToken, needsToken, pairWith, tokenRefused } from '#src/connection/pairing.ts';
import { Button } from '#src/ui/Button.tsx';

const Box: BoxComponent = (p) => <div {...p} />;

export function Connect({ children }: { children: ReactNode }) {
  const [{ url, insecure }] = useState(() => daemonUrl(location.search, location));
  const [storage] = useState(browserStorage);
  const [token, setToken] = useState(() => (needsToken(url) ? loadToken(storage, url) : null));
  const [lost, setLost] = useState(false);
  // A revoked device, or a token from an older pairing: the listener refuses it, so it goes and pairing comes back.
  useEffect(() => {
    if (!token || !needsToken(url)) return;
    let live = true;
    void tokenRefused(url, token, webSocketTransport).then((no) => {
      if (!live || !no) return;
      forgetToken(storage, url);
      setLost(true);
      setToken(null);
    });
    return () => {
      live = false;
    };
  }, [url, token, storage]);
  const store = useMemo(() => {
    const s = new Store(new DaemonClient({ client: 'web-pc', token: token ?? undefined, connect: () => webSocketTransport(url) }));
    if (insecure) s.fail(`Refusing ${url}: use wss:// for a daemon that is not on this machine.`);
    return s;
  }, [url, insecure, token]);

  if (needsToken(url) && !token && !insecure) return <Pair url={url} storage={storage} lost={lost} onPaired={setToken} />;
  return (
    <DaemonProvider store={store} where={url} connect={!insecure}>
      <DataProvider daemonId={url} box={Box}>
        <style>{SKELETON_CSS}</style>
        {children}
      </DataProvider>
    </DaemonProvider>
  );
}

/** Pairing: the code from `hl device pair`, or an invite link (`#pair=<code>`) that pairs by itself. */
function Pair({ url, storage, lost, onPaired }: { url: string; storage: KV | null; lost: boolean; onPaired: (token: string) => void }) {
  const [invite] = useState(() => inviteCode(location.hash));
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
    if (!invite) return;
    history.replaceState(null, '', location.pathname + location.search);
    void pair(invite);
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
        In a terminal run <code>hl device pair</code> and paste the code, or the <code>handline://</code> line, here.
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
