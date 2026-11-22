// Opens the daemon: a listener that wants a device token gets the pairing screen until this browser holds one.
import { Store, type KV, type ShellDaemon } from '@handline/app-core';
import { shell } from '#src/shell.ts';
import { DataProvider } from '@handline/app-data';
import { DaemonProvider } from '@handline/app-react';
import { DaemonClient, masterOf, webSocketTransport, type Node } from '@handline/client';
import { useCallback, useEffect, useMemo, useState, useSyncExternalStore, type ReactNode } from 'react';
import { browserStorage, daemonUrl } from '#src/connection/daemon.ts';
import { deviceName, forgetToken, inviteCode, loadToken, needsToken, pairWith, tokenRefused } from '#src/connection/pairing.ts';
import { Button } from '#src/ui/Button.tsx';
import { ModalHost } from '#src/ui/modal.tsx';
import { doorUrl, Machines, masterConnect } from '#src/connection/cluster.ts';
import { KeepMachines, MasterName, PromoteButton, useThisMacRunner } from '#src/connection/master.tsx';

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
  // The owner's machines: the page joins whichever is master now, with the same token.
  const machines = useMemo(() => new Machines(storage, url), [storage, url]);
  const name = useSyncExternalStore(machines.subscribe, () => machines.get().master?.name ?? '');
  const several = useSyncExternalStore(machines.subscribe, () => machines.get().urls.length > 1);
  const master = useMemo(() => ({ name, several }), [name, several]);
  const onRunner = !!useThisMacRunner(); // a runner Mac's page is on a master, never on this Mac's daemon
  const seen = useCallback(
    (st: { master: { id: string; number: number }; nodes: Node[] }) => {
      machines.seen(st);
      const doors = st.nodes.flatMap((n) => (n.door && doorUrl(n.door) ? [n.door] : []));
      if (doors.length) void shell()?.keepDoors?.(doors);
    },
    [machines],
  );
  const store = useMemo(() => {
    const t = token ?? undefined;
    const m = masterConnect({ url, kept: machines.get, open: webSocketTransport, token: t });
    const client = new DaemonClient({
      client: 'web-pc',
      token: t,
      connect: m.connect,
      master: { floor: () => machines.get().master, below: m.below },
    });
    client.onState((st) => {
      const said = st === 'open' ? masterOf(client.hello) : null;
      if (said) machines.joined(m.at(), said);
    });
    const s = new Store(client);
    if (insecure) s.fail(`Refusing ${url}: use wss:// for a daemon that is not on this machine.`);
    return s;
  }, [url, insecure, token, machines]);
  const off = insecure || !!failure;
  const error = useSyncExternalStore(
    (f) => store.subscribe(f),
    () => store.error,
  );
  const unreachable = useSyncExternalStore(
    (f) => store.subscribe(f),
    () => store.unreachable,
  );

  // Refused before it starts: the reason alone, as nothing will be tried.
  if (off) return <Stopped text={error ?? failure ?? ''} />;
  if (needsToken(url) && !token && !shellSaidNone) return <AppSkeleton />;
  if (needsToken(url) && !token) return <Pair url={url} storage={storage} lost={lost} onPaired={setToken} />;
  return (
    // The provider stays while the daemon has not answered yet, so it keeps trying: the frame, with why it waits. A daemon that
    // refuses this client (it is not tried again) gets the reason alone.
    <DaemonProvider store={store} where={url} connect>
      {error && store.served.size === 0 && unreachable ? (
        <AppSkeleton>
          <Stopped text={master.several || onRunner ? `Can't reach the master${master.name ? `, ${master.name}` : ''}.` : error} waiting />
          <ModalHost /> {/* the app's own is not drawn yet: Make this Mac the master asks first */}
        </AppSkeleton>
      ) : error && store.served.size === 0 ? (
        <Stopped text={error} />
      ) : (
        <DataProvider daemonId={url}>
          <KeepMachines seen={seen} />
          <MasterName.Provider value={master}>{children}</MasterName.Provider>
        </DataProvider>
      )}
    </DaemonProvider>
  );
}

/** The app's frame drawn empty while the page waits to connect; `children` (why it waits) sits in the main pane. */
function AppSkeleton({ children }: { children?: ReactNode }) {
  const bar = (w: number, h = 'h-3.5') => <div className={`${h} rounded-sm bg-border`} style={{ width: `${w}%` }} />;
  return (
    <div aria-busy aria-label="Loading" className="flex h-full flex-col">
      <div className="flex min-h-0 flex-1">
        <div className="flex w-60 shrink-0 animate-pulse flex-col gap-3 border-r border-border bg-surface px-4 pt-14">
          {[45, 55, 40].map((w, i) => (
            <div key={i}>{bar(w)}</div>
          ))}
          <div className="my-2 h-px bg-border" />
          {[60, 70, 50, 65, 55, 60, 45].map((w, i) => (
            <div key={i} className="flex items-center gap-2">
              <div className="size-3.5 shrink-0 rounded-full bg-border" />
              {bar(w)}
            </div>
          ))}
        </div>
        <div className="flex min-w-0 flex-1 flex-col">
          <div className="flex h-10 shrink-0 animate-pulse items-center gap-2 border-b border-border px-3">
            <div className="h-6 w-20 rounded-md bg-border" />
            <div className="h-6 w-16 rounded-md bg-border/60" />
          </div>
          <div className="relative flex min-h-0 flex-1 flex-col justify-end gap-4 px-8 pb-6">
            <div className="flex animate-pulse flex-col gap-4">
              <div className="ml-auto h-10 w-1/3 rounded-xl bg-border/60" />
              <div className="flex flex-col gap-2">
                {bar(70)}
                {bar(55)}
              </div>
              <div className="ml-auto h-10 w-1/4 rounded-xl bg-border/60" />
              <div className="h-24 rounded-xl border border-border" />
            </div>
            {children && <div className="absolute inset-x-0 top-1/4 flex justify-center">{children}</div>}
          </div>
        </div>
      </div>
      <div className="h-7 shrink-0 animate-pulse border-t border-border bg-surface" />
    </div>
  );
}

/** In place of the app when it will not connect: why, and Try again when the desktop app can start its daemon. */
/** `waiting`: it is still trying, so the words are not an error's. */
function Stopped({ text, waiting = false }: { text: string; waiting?: boolean }) {
  const [daemon, setDaemon] = useState<ShellDaemon | null>(null);
  useEffect(() => shell()?.onDaemon?.(setDaemon), []);
  const [tried, setTried] = useState(false);
  useEffect(() => {
    if (tried && daemon?.state === 'running') location.reload();
  }, [tried, daemon?.state]);
  const failed = daemon?.state === 'failed';
  return (
    <div role="alert" className="mx-auto flex max-w-sm flex-col gap-3 rounded-lg border border-border bg-bg px-4 py-3 text-sm shadow-sm">
      {failed && <h1 className="text-lg font-semibold text-fg">Handline could not start</h1>}
      <p className={waiting && !failed ? 'text-muted' : 'text-danger'}>{failed && daemon.reason ? daemon.reason : text}</p>
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
      {waiting && !failed && <PromoteButton />}
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
