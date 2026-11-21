// Settings > Devices: one list of every device, the master's own Mac first. A Mac's or a server's row joins its paired device (devices.list,
// where its Run · Pause · Off lives) with its node (node.status: online, what it lacks) and the agents that stay on it (agents.list node).
// The master's own Mac has no paired device: its Pause is the daemon's pause, and it is never Off.
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
};

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
  opts: { me: string | null; here: string | null; paused: boolean; now?: number },
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
    const mode: Mode | null = runner ? (d.lending ?? 'off') : null;
    const online = node ? node.online : d.connected;
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
    });
  }
  return rows;
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
  const pause = Hooks.useDaemonPause();
  const resume = Hooks.useDaemonResume();
  const settings = Hooks.useSettingsGet();
  const setSettings = Hooks.useSettingsSet();
  const { pending, run } = useKeyed();
  const values = settings.data?.values;
  const rows = deviceRows(list.data?.devices ?? [], nodes.data?.nodes ?? [], agents.data?.agents ?? [], {
    me,
    here,
    paused: !!status.data?.paused,
  });
  /** The master's own Mac pauses and resumes the daemon; any other runner is device.set. */
  const setMode = (row: DeviceRow, mode: Mode) =>
    run(`mode:${row.id}`, () =>
      row.master
        ? mode === 'off'
          ? Promise.reject(new Error('The master always runs.'))
          : (mode === 'pause' ? pause : resume).mutateAsync({})
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
