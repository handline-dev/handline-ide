// The clusters this Mac joined (the desktop shell's list), each with a store of its own kept connected beside this Mac's, and the scope
// that makes a subtree read one of them. This Mac's own cluster is id ''.
import { Store, type ShellCluster } from '@handline/app-core';
import { DataProvider, DataScope, Hooks, makeQueryClient } from '@handline/app-data';
import { DaemonProvider, useSelected } from '@handline/app-react';
import { DaemonClient, webSocketTransport } from '@handline/client';
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from 'react';
import { shell } from '#src/shell.ts';
import { TabId } from '#src/persist/tabState.ts';
import { pressedIn, scopeModals } from '#src/ui/modal.tsx';

export type Joined = ShellCluster & { store: Store; queries: ReturnType<typeof makeQueryClient> };

const JoinedContext = createContext<Joined[]>([]);
/** Whether the shell has said which clusters are joined (at once outside the shell). */
const ReadyContext = createContext(true);
/** The cluster a subtree reads: '' for this Mac's. */
const HereContext = createContext('');

/** The clusters this Mac joined, in the shell's order; none outside the desktop shell. */
export const useJoined = () => useContext(JoinedContext);
export const useJoinedReady = () => useContext(ReadyContext);
/** The cluster the subtree reads: '' for this Mac's own. */
export const useCluster = () => useContext(HereContext);

/** The joined cluster that stands as home on a Mac whose own board has no workspace: its `@<id>/` keys read as home's. */
let homeAlias = '';
const aliasSubs = new Set<() => void>();
function setHomeAlias(id: string) {
  homeAlias = id;
  for (const f of aliasSubs) f();
}
const onAlias = (f: () => void) => (aliasSubs.add(f), () => void aliasSubs.delete(f));

/** Keeps a store connected for each joined cluster (its DataProvider keeps its cache current) and gives the list to the tree. */
export function JoinedClusters({ children }: { children: ReactNode }) {
  const [joined, setJoined] = useState<Joined[]>([]);
  const [ready, setReady] = useState(() => !shell()?.onClusters);
  // One store per cluster, kept while it is joined: its master's address or token changing makes a new one.
  useEffect(() => {
    let had = new Map<string, Joined>();
    return shell()?.onClusters?.((list) => {
      const next = list.map((c): Joined => {
        const h = had.get(c.id);
        if (h && h.daemon === c.daemon && h.token === c.token) return { ...h, name: c.name, reachable: c.reachable };
        const client = new DaemonClient({ client: 'web-pc', token: c.token, connect: () => webSocketTransport(c.daemon) });
        return { ...c, store: new Store(client), queries: makeQueryClient() };
      });
      had = new Map(next.map((j) => [j.id, j]));
      setJoined(next);
      setReady(true);
    });
  }, []);
  // A Mac with no workspace of its own that joined a cluster reads that cluster as home, as a browser on its master does.
  const ownList = Hooks.useWorkspacesList();
  const own = ownList.data;
  const home = own?.workspaces.length === 0 ? joined[0] : undefined;
  const alias = home?.id ?? '';
  const applied = useSyncExternalStore(onAlias, () => homeAlias);
  useLayoutEffect(() => setHomeAlias(alias), [alias]);
  const listed = home ? joined.filter((j) => j !== home) : joined;
  // Nothing draws before the shell's list and this Mac's workspaces are known, so it never starts on the wrong daemon.
  const decided = ready && (!joined.length || own !== undefined || ownList.isError) && applied === alias;
  return (
    <ReadyContext.Provider value={ready && decided}>
      <JoinedContext.Provider value={listed}>
        {/* Every press says its cluster, this Mac's here and a joined one's in its InCluster (later in the capture), for the modal it opens. */}
        <div className="contents" onPointerDownCapture={() => pressedIn('')} onKeyDownCapture={() => pressedIn('')}>
          {joined.map((j) => (
            <DaemonProvider key={j.id} store={j.store} where={j.daemon} notify={false}>
              <DataProvider daemonId={j.daemon} client={j.queries} />
            </DaemonProvider>
          ))}
          {!decided ? null : home ? (
            <DaemonProvider store={home.store} connect={false}>
              <DataScope daemonId={home.daemon} client={home.queries}>
                {children}
              </DataScope>
            </DaemonProvider>
          ) : (
            children
          )}
        </div>
      </JoinedContext.Provider>
    </ReadyContext.Provider>
  );
}

/** Makes `children` read and write cluster `id` ('' leaves them on this Mac's); a cluster left meanwhile draws nothing. `dim`: its rows
 * fade to 40% while it does not answer. */
export function InCluster({ id, dim, children }: { id: string; dim?: boolean; children: ReactNode }) {
  const j = useJoined().find((c) => c.id === id);
  if (!id) return children;
  if (!j) return null;
  return (
    <HereContext.Provider value={id}>
      <DaemonProvider store={j.store} connect={false}>
        <DataScope daemonId={j.daemon} client={j.queries}>
          <div className="contents" onPointerDownCapture={() => pressedIn(id)} onKeyDownCapture={() => pressedIn(id)}>
            {dim ? <Dim cluster={j}>{children}</Dim> : children}
          </div>
        </DataScope>
      </DaemonProvider>
    </HereContext.Provider>
  );
}

/** This cluster's name and machine count, from hello; null before it answers or on a daemon without names. */
export function useClusterName(): { name: string; machines: number } | null {
  const name = useSelected((s) => s.client.hello?.capabilities.cluster?.name ?? null);
  const machines = useSelected((s) => s.client.hello?.capabilities.cluster?.machines ?? 0);
  return name === null ? null : { name, machines };
}

function Dim({ cluster, children }: { cluster: Joined; children: ReactNode }) {
  return useOnline(cluster) ? children : <div className="opacity-40">{children}</div>;
}

/** Whether a joined cluster answers: the shell reaches its master and so does the page. */
export function useOnline(j: Joined): boolean {
  const subscribe = useCallback((cb: () => void) => j.store.subscribe(cb), [j.store]);
  const down = useSyncExternalStore(subscribe, () => j.store.unreachable || j.store.conn === 'reconnecting');
  return j.reachable && !down;
}

/** An agent's workspace key in the sidebar and the layouts: a joined cluster's agent is `@<cluster>/<agent>`, this Mac's its bare key. */
export const spaceOf = (cluster: string, agent: string) => (cluster ? `@${cluster}/${agent}` : agent);
/** The cluster and the agent a workspace key names. */
export function splitSpace(space: string): { cluster: string; agent: string } {
  const m = /^@([^/]+)\/(.*)$/.exec(space);
  return m && m[1] !== homeAlias ? { cluster: m[1]!, agent: m[2]! } : { cluster: '', agent: m ? m[2]! : space };
}

export type Of<T> = { cluster: string; data: T };

/** Runs `hook` in this Mac's cluster and in every joined one, and draws `children` with each one's result, this Mac's first, once that
 * one's is in. `sig` says when a result changed: a hook gives a new object each render. */
export function Across<T>({
  hook,
  sig,
  alone,
  children,
}: {
  hook: () => T;
  sig: (t: T) => string;
  /** Only the cluster the subtree reads, under ''. */
  alone?: boolean;
  children: (all: Of<T>[]) => ReactNode;
}) {
  const everyone = useJoined();
  const joined = alone ? [] : everyone;
  const [got, setGot] = useState<Record<string, T>>({});
  const put = useCallback((cluster: string, data: T) => setGot((g) => ({ ...g, [cluster]: data })), []);
  const all = ['', ...joined.map((j) => j.id)].flatMap((c) => (c in got ? [{ cluster: c, data: got[c]! }] : []));
  return (
    <>
      <Probe cluster="" hook={hook} sig={sig} put={put} />
      {joined.map((j) => (
        <InCluster key={j.id} id={j.id}>
          <Probe cluster={j.id} hook={hook} sig={sig} put={put} />
        </InCluster>
      ))}
      {'' in got && children(all)}
    </>
  );
}

function Probe<T>({ cluster, hook, sig, put }: { cluster: string; hook: () => T; sig: (t: T) => string; put: (c: string, t: T) => void }) {
  const useHook = hook; // a hook name, so it is called as one
  const data = useHook();
  const said = sig(data);
  const latest = useRef(data);
  useEffect(() => {
    latest.current = data;
  });
  useEffect(() => put(cluster, latest.current), [cluster, said, put]);
  return null;
}

/** A pane's tab as it draws: one another cluster's row opened (`@<cluster>/<kind>`) names its kind, and draws in that cluster. */
export function tabIn<T extends { opens?: string }>(tab: T | undefined): { tab: T | undefined; cluster: string } {
  const { cluster, agent } = splitSpace(tab?.opens ?? '');
  return { tab: tab && cluster ? { ...tab, opens: agent } : tab, cluster };
}

/** A tab's body: its id (persist/tabState) and its cluster. */
export function TabScope({ id, cluster, children }: { id: string | null; cluster: string; children: ReactNode }) {
  return (
    <InCluster id={cluster}>
      <TabId.Provider value={id}>{children}</TabId.Provider>
    </InCluster>
  );
}

/** The cluster of the workspace on screen ('' for this Mac's and the pages'). */
export const SpaceCluster = createContext('');

/** What a control opens, as a tab of the workspace on screen: one of another cluster carries its cluster (`@<cluster>/<kind>`). */
export function useOpensHere<T extends string | null>(opens: T): T {
  const cluster = useCluster();
  const space = useContext(SpaceCluster);
  return (opens && cluster !== space ? spaceOf(cluster, opens) : opens) as T;
}

scopeModals((cluster, node) => <InCluster id={cluster}>{node}</InCluster>);
