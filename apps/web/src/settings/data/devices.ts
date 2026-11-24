// Settings > Devices: one list of every device, the master's own Mac first. A Mac's or a server's row joins its paired device (devices.list)
// with its node (node.status: Run or Pause, online, what it lacks; no node is Off) and the agents that stay on it (agents.list node).
// The master's own Mac has no paired device: its Pause is the daemon's pause, and it is never Off.
import { read, write } from '#src/persist/kv.ts';
import { createStore } from '#src/persist/store.ts';
import type { Device, Node } from '@handline/client';
import { Hooks, useAgents } from '@handline/app-data';
import { useHello, useThisDevice } from '@handline/app-react';
import { useKeyed } from '#src/settings/data/keyed.ts';

export const STALE_DAYS = 30;
export const isStale = (d: Pick<Device, 'last_seen' | 'connected'>, now = Date.now()): boolean =>
  !d.connected && d.last_seen !== null && now - Date.parse(d.last_seen) > STALE_DAYS * 86_400_000;

/** Opens the master's own Mac in Devices, whatever its node id. */
export const MASTER = 'master';

export type Mode = 'run' | 'pause' | 'off';
export const MODES: readonly Mode[] = ['run', 'pause', 'off'];
/** One word per row: offline beats off beats paused beats running. A runner set to Run that the master has not heard yet is starting. */
export type State = 'running' | 'paused' | 'off' | 'offline' | 'starting' | 'connected' | 'seen';

export type DeviceRow = {
  /** The paired device's id; the node id for the master's own Mac. */
  id: string;
  name: string;
  kind: NonNullable<Device['kind']> | null;
  /** A Mac or a server: it can run agents, so it has Run · Pause · Off. */
  runner: boolean;
  master: boolean;
  /** The device this app runs on. */
  here: boolean;
  /** Its person is the caller: only they set its Run · Pause · Off. */
  mine: boolean;
  mode: Mode | null;
  state: State;
  /** When it was last heard: offline since, or last seen. */
  since: string | null;
  agents: string[];
  lacks: { agent: string; why: string }[];
  node: Node | null;
  stale: boolean;
  /** A lent machine online on another Handline build than the master's: its Update has it take the master's. */
  behind: boolean;
  /** Behind and taking the master's build by itself: Auto, on one of the owner's machines (another person's waits for its Update). */
  updating: boolean;
  /** The newer version its Mac app has downloaded, '' for none: its Update installs it (on Auto, also once no window is open there). */
  appReady: string;
};

/** Its own person updates a Mac on another build that does not take the master's by itself, or whose app has a newer version ready. */
export const canUpdate = (r: DeviceRow): boolean => r.mine && ((r.behind && !r.updating) || !!r.appReady);

const RUNNER_KINDS: ReadonlySet<string> = new Set(['mac', 'server']);

function stateOf(online: boolean, mode: Mode, heard: boolean): State {
  if (!online) return mode !== 'off' && !heard ? 'starting' : 'offline';
  return mode === 'off' ? 'off' : mode === 'pause' ? 'paused' : 'running';
}

/** The rows, the master's own Mac first, then the device this app runs on, then connected, then the most recently seen. */
export function deviceRows(
  devices: Device[],
  nodes: Node[],
  agents: { key: string; node?: string | null }[],
  opts: { me: string | null; here: string | null; paused: boolean; buildAuto?: boolean; now?: number },
): DeviceRow[] {
  const on = (node: string) => agents.filter((a) => a.node === node).map((a) => a.key);
  const self = nodes.find((n) => n.self);
  const rows: DeviceRow[] = [];
  if (self) {
    const mode: Mode = opts.paused ? 'pause' : 'run';
    rows.push({
      id: self.id,
      name: self.name,
      kind: 'mac',
      runner: true,
      master: true,
      here: !devices.some((d) => d.id === opts.here),
      mine: self.person === opts.me,
      mode,
      state: stateOf(true, mode, true),
      since: self.last_seen,
      agents: on(self.id),
      lacks: [],
      node: self,
      stale: false,
      behind: false,
      updating: false,
      appReady: '',
    });
  }
  const sorted = [...devices].sort(
    (a, b) =>
      Number(b.id === opts.here) - Number(a.id === opts.here) ||
      Number(b.connected) - Number(a.connected) ||
      Date.parse(b.last_seen ?? b.created) - Date.parse(a.last_seen ?? a.created),
  );
  for (const d of sorted) {
    const runner = RUNNER_KINDS.has(d.kind ?? '');
    const node = runner ? (nodes.find((n) => !n.self && n.device === d.id) ?? null) : null;
    // node.status lists a lent machine on Run or Pause and leaves it out on Off; its feed is live, devices.list's is not (useOwnMac).
    const mode: Mode | null = runner ? (node?.lending ?? 'off') : null;
    const online = node ? node.online : d.connected;
    const behind = !!node?.online && !!node.version && !!self?.version && node.version !== self.version;
    rows.push({
      id: d.id,
      name: d.name,
      kind: d.kind ?? null,
      runner,
      master: false,
      here: d.id === opts.here,
      mine: !!d.person && d.person === opts.me,
      mode,
      state: mode ? stateOf(online, mode, !!node?.last_seen || d.connected) : d.connected ? 'connected' : 'seen',
      since: node?.last_seen ?? d.last_seen,
      agents: node ? on(node.id) : [],
      lacks: node?.lacks ?? [],
      node,
      stale: isStale(d, opts.now),
      behind,
      updating: behind && opts.buildAuto !== false && !!node?.person && node.person === self?.person,
      appReady: (node?.online && node.app?.ready) || '',
    });
  }
  return rows;
}

/** The master's own Mac is paused and resumed by the daemon's pause. */
function useMasterPause() {
  const pause = Hooks.useDaemonPause();
  const resume = Hooks.useDaemonResume();
  return (on: boolean) => (on ? pause : resume).mutateAsync({});
}

/** The Mac this app's daemon runs on, for the status bar: node.status and the pause only, not devices.list, whose live topic the daemon
 * serves only on a stream of its own. */
export function useOwnMac() {
  const nodes = Hooks.useNodeStatus();
  const status = Hooks.useDaemonStatus();
  const me = useHello()?.caller.key ?? null;
  const pauseMaster = useMasterPause();
  const { pending, run } = useKeyed();
  const row = deviceRows([], nodes.data?.nodes ?? [], [], { me, here: null, paused: !!status.data?.paused }).find((r) => r.master);
  const setMode = (mode: Mode) =>
    mode === 'off' ? Promise.reject(new Error('The master always runs.')) : run('mode', () => pauseMaster(mode === 'pause')).then(() => {});
  return { row: status.isPending ? undefined : row, setMode, pending };
}

export function useDevices() {
  const list = Hooks.useDevicesList();
  const nodes = Hooks.useNodeStatus();
  const agents = useAgents();
  const status = Hooks.useDaemonStatus();
  const here = useThisDevice() ?? null;
  const me = useHello()?.caller.key ?? null;
  const revoke = Hooks.useDeviceRevoke();
  const set = Hooks.useDeviceSet();
  const update = Hooks.useDeviceUpdate();
  const pauseMaster = useMasterPause();
  const settings = Hooks.useSettingsGet();
  const setSettings = Hooks.useSettingsSet();
  const { pending, run } = useKeyed();
  const values = settings.data?.values;
  const rows = deviceRows(list.data?.devices ?? [], nodes.data?.nodes ?? [], agents.data?.agents ?? [], {
    me,
    here,
    paused: !!status.data?.paused,
    buildAuto: values?.node_build_auto,
  });
  /** The master's own Mac pauses and resumes the daemon; any other runner is device.set. */
  const setMode = (row: DeviceRow, mode: Mode) =>
    run(`mode:${row.id}`, () =>
      row.master
        ? mode === 'off'
          ? Promise.reject(new Error('The master always runs.'))
          : pauseMaster(mode === 'pause')
        : set.mutateAsync({ device: row.id, lending: mode }),
    ).then(() => {});
  return {
    loading: list.isPending || nodes.isPending || status.isPending,
    error: list.isError,
    rows,
    pushPlatforms: list.data?.push_platforms ?? [],
    setMode,
    revoke: (id: string) => run(`revoke:${id}`, () => revoke.mutateAsync({ device: id })).then(() => {}),
    /** The master's cap on its agents' memory, in GB, 0 off; a lent machine without its own cap is held to it too. */
    memoryMax: values?.agents_memory_max_gb ?? 0,
    /** A lent machine's own cap; 0 puts it back on the master's. */
    setMemoryMax: (row: DeviceRow, gb: number) =>
      run(`memory:${row.id}`, () => set.mutateAsync({ device: row.id, memory_max_gb: gb || null })).then(() => {}),
    /** On, a lent Mac that goes on battery is set to Off, and back to Run when plugged in. */
    battery: values?.node_battery_switch ?? false,
    setBattery: (on: boolean) => run('battery', () => setSettings.mutateAsync({ changes: { node_battery_switch: on } })).then(() => {}),
    /** How long a device may be offline before its agents' work goes to another device, in seconds. */
    reclaimS: values?.node_reclaim_s ?? 1800,
    /** Auto: a lent Mac of the owner's on another build takes the master's by itself; Manual leaves it to the row's Update. */
    buildAuto: values?.node_build_auto ?? true,
    setBuildAuto: (on: boolean) => run('build', () => setSettings.mutateAsync({ changes: { node_build_auto: on } })).then(() => {}),
    update: (row: DeviceRow) => run(`update:${row.id}`, () => update.mutateAsync({ device: row.id })).then(() => {}),
    setReclaimS: (s: number) => run('reclaim', () => setSettings.mutateAsync({ changes: { node_reclaim_s: s } })).then(() => {}),
    pending,
  };
}

/** The device an agent stays on, by its name: online or not, and since when it was last heard. Null on a single Mac, where it says nothing. */
export function useAgentDevice(agent: string): { name: string; online: boolean; since: string | null } | null {
  const nodes = Hooks.useNodeStatus().data?.nodes ?? [];
  const at = useAgents().data?.agents.find((a) => a.key === agent)?.node;
  const n = nodes.length > 1 ? nodes.find((x) => x.id === at) : undefined;
  return n ? { name: n.name, online: n.online, since: n.last_seen } : null;
}

/** A machine whose own board was set aside when a newer master outranked it, and the master now; null when none was. */
export function useSetAside(): { name: string; since: string; master: string; key: string } | null {
  const nodes = Hooks.useNodeStatus().data?.nodes ?? [];
  const n = nodes.find((x) => x.set_aside);
  if (!n?.set_aside) return null;
  return { name: n.name, since: n.set_aside.since, master: nodes.find((x) => x.master)?.name ?? '', key: `${n.id}@${n.set_aside.since}` };
}

// The set-aside board is told once: the status row shows it until Settings > Devices has shown its note.
const SEEN = 'set-aside-seen';
const seen = createStore(read(SEEN, ''));
export function markSetAsideSeen(key: string): void {
  if (seen.get() === key) return;
  write(SEEN, key);
  seen.set(key);
}
/** The set-aside board, while the person has not seen its note. */
export function useSetAsideUnseen() {
  const a = useSetAside();
  const was = seen.useValue();
  return a && a.key !== was ? a : null;
}
