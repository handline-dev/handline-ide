// Opens the daemon: a listener that wants a device token gets the pairing screen until this browser holds one.
import { Store, type KV, type ShellDaemon } from '@handline/app-core';
import { shell } from '#src/shell.ts';
import { DataProvider, SKELETON_CSS, type BoxComponent } from '@handline/app-data';
import { DaemonProvider } from '@handline/app-react';
import { DaemonClient, webSocketTransport } from '@handline/client';
import { useEffect, useMemo, useState, useSyncExternalStore, type ReactNode } from 'react';
import { browserStorage, daemonUrl } from '#src/connection/daemon.ts';
import { deviceName, forgetToken, inviteCode, loadToken, needsToken, pairWith, tokenRefused } from '#src/connection/pairing.ts';
import { Button } from '#src/ui/Button.tsx';

const Box: BoxComponent = (p) => <div {...p} />;

export function Connect({ children }: { children: ReactNode }) {
  const [storage] = useState(browserStorage);
  // What the desktop shell left for the page (apps/electron preload.ts): this machine's own address, and why its daemon did not start.
  const [{ url, insecure }] = useState(() => daemonUrl(location.search, location, storage?.getItem('shell:own-host') || null));
  const [failure] = useState(() => storage?.getItem('shell:daemon-failure') || null);
  const [token, setToken] = useState(() => (needsToken(url) ? loadToken(storage, url) : null));
  const [lost, setLost] = useState(false);
  // The desktop shell's newer device token (the preload has already kept it): the page connects with it, no reload.
  // In the shell a page without a token waits for the shell's word (a token, or '' for none) on the app's skeleton, not the pairing form.
  const [shellSaidNone, setShellSaidNone] = useState(() => !shell()?.onToken || !storage?.getItem('shell:token-pending'));
  useEffect(() => shell()?.onToken?.((t) => (t ? setToken(t) : setShellSaidNone(true))), []);
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
  const off = insecure || !!failure;
  const error = useSyncExternalStore(
    (f) => store.subscribe(f),
    () => store.error,
  );

  // Refused before it starts, or a failure before the daemon has answered once: the reason, not a skeleton that waits for ever.
  if (off || (error && store.served.size === 0)) return <Stopped text={error ?? failure ?? ''} />;
  if (needsToken(url) && !token && !shellSaidNone) return <AppSkeleton />;
  if (needsToken(url) && !token) return <Pair url={url} storage={storage} lost={lost} onPaired={setToken} />;
  return (
    <DaemonProvider store={store} where={url} connect>
      <DataProvider daemonId={url} box={Box}>
        <style>{SKELETON_CSS}</style>
        {children}
      </DataProvider>
    </DaemonProvider>
  );
}

/** The app's frame drawn empty while the page waits to connect: sidebar, main pane, status bar. */
function AppSkeleton() {
  return (
    <div aria-busy aria-label="Loading" className="flex h-full animate-pulse flex-col">
      <div className="flex min-h-0 flex-1">
        <div className="flex w-60 shrink-0 flex-col gap-3 bg-surface px-4 pt-14">
          {[50, 60, 45, 0, 70, 55, 65, 50].map((w, i) =>
            w ? <div key={i} className="h-3.5 rounded-sm bg-border" style={{ width: `${w}%` }} /> : <div key={i} className="h-3" />,
          )}
        </div>
        <div className="flex flex-1 flex-col">
          <div className="h-10 border-b border-border bg-surface" />
        </div>
      </div>
      <div className="h-7 border-t border-border bg-surface" />
    </div>
  );
}

/** In place of the app when it will not connect: why, and Try again when the desktop app can start its daemon. */
function Stopped({ text }: { text: string }) {
  const [daemon, setDaemon] = useState<ShellDaemon | null>(null);
  useEffect(() => shell()?.onDaemon?.(setDaemon), []);
  const [tried, setTried] = useState(false);
  useEffect(() => {
    if (tried && daemon?.state === 'running') location.reload();
  }, [tried, daemon?.state]);
  const failed = daemon?.state === 'failed';
  return (
    <div role="alert" className="mx-auto mt-32 flex max-w-sm flex-col gap-3 px-4 text-sm">
      {failed && <h1 className="text-lg font-semibold text-fg">Handline could not start</h1>}
      <p className="text-danger">{failed && daemon.reason ? daemon.reason : text}</p>
      {failed && daemon.canStart && (
        <Button
          variant="primary"
          className="w-fit"
          onClick={() => {
            setTried(true);
            void shell()?.startDaemon?.();
          }}
        >
          Try again
        </Button>
      )}
      {tried && daemon?.state === 'starting' && <p className="text-muted">Starting…</p>}
    </div>
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
