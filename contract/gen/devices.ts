// Generated from contract/spec/ by contract/generate.py; do not edit.
/* eslint-disable */
import type { Accepted, Cluster, RequestId, Role, Timestamp, Topic, WriteKind } from './core.ts';

export type Device = {
  id: string;
  name: string;
  /** The role of the device's person, which it acts with. */
  role: "owner" | "member";
  person?: string | null;
  created: Timestamp;
  last_seen: Timestamp | null;
  connected: boolean;
  /** Whether the device gave a push token (device.push_token) that the daemon still holds. */
  push: boolean;
  /** What the client said it is at device.redeem; null when it did not say. A mac or a server lends its machine. Absent on a daemon that does not serve it. */
  kind?: ("ios" | "android" | "web" | "desktop" | "mac" | "server") | null;
  /** True in devices.list for the caller's own device; false in events. Absent on a daemon that does not serve it. */
  current?: boolean;
  /** The client its last hello named (hello's client); null before one. Absent on a daemon that does not serve it. */
  client?: string | null;
  /** A mac's or a server's Run · Pause · Off (device.set): run starts new work on that machine, pause starts nothing new and lets what runs there finish, off is not lent. null for a phone or a browser. Absent on a daemon that does not serve it. */
  lending?: ("run" | "pause" | "off") | null;
  /** The runner's copy of the board: when it last held all of the master's; null for a runner that keeps none (another person's, or one that has not pulled). Not on the master's own row. */
  copy?: {
    matched_at: Timestamp;
    /** Seconds since its copy last held all of the master's. */
    behind_s: number;
  } | null;
};

/** A machine of this master: its own, and the ones people lend it. */
export type Node = {
  /** The machine's node id (DaemonStatus.machine.id on that machine). */
  id: string;
  /** The key of the person whose machine it is. */
  person: string | null;
  /** The machine's name, as it calls itself. */
  name: string;
  joined_at: Timestamp;
  /** When the master last heard from it: for its own row, the daemon's start. */
  last_seen: Timestamp | null;
  /** The master number this machine last took; the master's own is the board's, raised by one each time the master moves (node.promote). */
  master_number: number;
  /** It is the master. */
  master: boolean;
  /** It is the machine this daemon runs on. */
  self: boolean;
  /** The master hears from it now; always true for the master's own row. */
  online: boolean;
  /** run: the master starts new work there; pause: it starts nothing new there and what runs finishes. The master's own row is always run; a machine on off is not listed. */
  lending: "run" | "pause";
  /** Each workspace's repo on this lent machine, cloned from its remote when the machine links and when a workspace is added. Not on the master's own row, nor on a machine not linked since the master started. */
  projects?: {
    /** The workspace's key. */
    workspace: string;
    /** no_remote: the repo has no origin, so the workspace runs only on the machine that holds its folder. A failed one is tried again when the machine links again. */
    state: "cloning" | "cloned" | "failed" | "no_remote";
    /** Git's word on a failed clone or fetch. */
    error?: string;
  }[];
  /** Each agent that cannot start there now and why ("codex not installed", "full: 2 of 2 sessions", "it is set to pause"); a runner's row only. */
  lacks?: {
    agent: string;
    why: string;
  }[];
  /** The paired device (devices.list id) a lent machine joined as; null on the master's own row. */
  device?: string | null;
  /** This lent machine's own cap on its agents' memory, in GB (device.set): at it, no new session starts there. null where agents_memory_max_gb applies, as it always does on the master's own row. */
  memory_max_gb?: number | null;
  /** A lent machine's newest heartbeat's measures, 0 for one it could not read; null on the master's own row (DaemonStatus has its own) and before its first heartbeat since the master started. */
  load?: {
    cores: number;
    /** The 1-minute load average. */
    load1: number;
    /** Bytes. */
    ram_total: number;
    /** Bytes. */
    ram_free: number;
    /** Bytes its agents' sessions use. */
    agents_rss: number;
  } | null;
  /** A lent machine runs on its battery now, from its newest heartbeat; null as load is. */
  battery?: boolean | null;
  /** Its Handline version: the master's own row the master's, a lent machine's from its newest heartbeat (null before its first since the master started). One not the master row's takes no work until it runs the master's build (node_build_auto, device.update). */
  version?: string | null;
  /** Where it serves, or would serve, as master: addr (host:port) and fp (its certificate's sha256, "" for a plain door); null before it said. */
  door?: {
    addr: string;
    fp: string;
  } | null;
  /** It was master, and a newer master outranked it: since when its own board is set aside, unchanged, and the master number that outranked it. null otherwise. */
  set_aside?: {
    since: Timestamp;
    master: number;
  } | null;
  /** The Mac app around a lent machine's engine, as its heartbeat says: its version, and ready, the newer one its update channel has downloaded ('' for none); null for none (a server, a phone, an older app). */
  app?: {
    version: string;
    ready: string;
  } | null;
};

export interface DevicesMethods {
  "devices.list": { params: { [key: string]: unknown }; result: {
    devices: Device[];
    /** The platforms the daemon has push credentials for (config.yaml). Empty is push off: no device is sent a notification, whatever token it gave. */
    push_platforms: ("apns" | "fcm")[];
  } };
  "device.pair": { params: {
  name?: string;
  /** Seconds the code lives; default 300. */
  ttl_s?: number;
  /** A host a door serves, while Connect a device still offers a choice; absent is Handline's pick: the Tailscale name while Tailscale is up, else the first LAN (Wi-Fi) address, served though network_addresses does not list it (not when the list is empty, and not while -advertise names the address); with neither, -advertise or the listener. Any other is refused, gate address, with data.addresses the hosts served. */
  address?: string;
  /** The person whose device it pairs (p<n>): the owner names one they invited (person.invite); a member only themself. The caller's own when left out. */
  person?: string;
  request_id: RequestId;
}; result: Accepted & {
    code: string;
    expires_at: Timestamp;
    /** The fingerprint of the door's self-made certificate, which a client pins; empty on a plain door and on Tailscale's own certificate, which is verified the normal way. */
    fingerprint: string;
    /** The pairing link, what the QR carries: http(s)://host:port/#pair=code, with &fp= the fingerprint on a self-made certificate. https on the Tailscale name when the tailnet has HTTPS (no fingerprint), http on it otherwise; a Wi-Fi door's link is for a Mac's app, which pins the fingerprint, as a Wi-Fi door serves no web page. */
    web_link: string;
  } };
  "device.redeem": { params: {
  code: string;
  name: string;
  /** What the client is: ios, android, web, desktop, or mac or server for a machine that lends itself, kept as Device.kind. Any other value is kept as null, so a newer client still pairs. */
  kind?: string;
  /** The machine's node id: required with kind mac or server, refused with any other. The device comes with that machine's node row, on off. */
  node?: string;
  request_id: RequestId;
}; result: Accepted & {
    token: string;
    device: string;
  } };
  "device.revoke": { params: {
  device: string;
  request_id: RequestId;
}; result: Accepted };
  "device.set": { params: {
  device: string;
  lending?: "run" | "pause" | "off";
  memory_max_gb?: number | null;
  request_id: RequestId;
}; result: Accepted };
  "device.update": { params: {
  device: string;
  request_id: RequestId;
}; result: Accepted };
  "device.make_master": { params: {
  device: string;
  request_id: RequestId;
}; result: Accepted };
  "device.push_token": { params: {
  platform: "apns" | "fcm";
  token: string;
  /** An APNs token of a development build, which goes to Apple's sandbox host. Default false. */
  sandbox?: boolean;
  request_id: RequestId;
}; result: Accepted };
  "node.status": { params: { [key: string]: unknown }; result: {
    /** The master's node id and its number. */
    master: {
      id: string;
      number: number;
    };
    nodes: Node[];
    cluster: Cluster;
    /** Where each agent's next session starts and why (docs/story4-design.md section 7). Absent on a daemon that does not place agents. */
    placement?: {
      /** The agent_placement setting. */
      mode: "auto" | "manual";
      /** One row per agent of this home. */
      agents: {
        /** The agent's key, as agent.set takes it. */
        agent: string;
        /** The node id its next session starts on: the master's own while it stays. */
        device: string;
        /** Why it is placed so, in words a person reads ("heavy: check.sh 18 h this week", "heavy: ...; Studio: codex not installed, so it stays here", "pinned to Studio"); "" for a light agent on the master's machine. */
        why: string;
        /** Heavy by its last 7 days (over 2 h of heavy commands or a 6 GB peak): placed on the lent device with the most room. */
        heavy: boolean;
        /** Seconds its heavy commands ran in its last 7 days. */
        heavy_s: number;
        /** The most bytes its processes held in one minute in its last 7 days. */
        memory_peak: number;
        /** The device it is pinned to (its device setting), kept on auto too; absent for none. */
        pin?: string;
      }[];
    };
  } };
  "node.forward": { params: {
  /** The agent whose session made the call, as the runner proved it. */
  agent: string;
  method: string;
  /** The call's own params, request_id included for a write. */
  params?: { [key: string]: unknown };
  /** The client the agent's hello named on the runner (hl). */
  client?: string;
  /** The folder that hello was sent from. */
  cwd?: string;
}; result: { [key: string]: unknown } };
  "node.turn": { params: {
  /** The agent whose session is on that runner. */
  agent: string;
  /** The turn as the runner's tracker holds it: state (0 unknown, 1 working, 2 idle), at, doing, background, bg_known, settled, live (its session process is watched there), ended. */
  view: { [key: string]: unknown };
}; result: { [key: string]: unknown } };
  "node.transcript": { params: {
  /** The agent whose session is on that runner. */
  agent: string;
  session: string;
  /** When the line was written, epoch seconds. */
  at: number;
  /** Each {kind, body} as transcript_events keeps it: text, tool_use, tool_result (its output already cut to 16 KiB). */
  rows: { [key: string]: unknown }[];
}; result: { [key: string]: unknown } };
  "node.heartbeat": { params: {
  /** The runner's Handline version: one that is not the master's takes no new session. */
  version: string;
  /** Its GOOS/GOARCH (darwin/arm64): the master hands its own build only to a runner of the same. */
  platform?: string;
  /** The machine's boot: another one means it restarted. */
  boot?: string;
  /** cores, load1, ram_total, ram_free and agents_rss (what its sessions' process trees use), as the runner read them. */
  load?: { [key: string]: unknown };
  /** Each session it holds: agent, handle, and the branch of its worktree with the commit last pushed (branch, pushed). */
  sessions?: { [key: string]: unknown }[];
  /** Each Claude account folder there logged in to an account: name, email, dir (its config dir, relative to the runner's home), reading (its newest usage reading: age_s, and windows of minutes, pct and reset) and login_failed (its login was refused even after a real turn). */
  accounts?: { [key: string]: unknown }[];
  /** What it has for an agent's work: clis, mcp, programs (of those the last answer asked about in check, the ones on its PATH), xcode, simulators. */
  has?: { [key: string]: unknown };
  /** The machine runs on its battery now. */
  battery?: boolean;
  /** Where it would serve as master: addr (host:port) and fp (its certificate's sha256, "" for a plain door). The master keeps it on the node row; a machine with none is not made the master. */
  door?: { [key: string]: unknown };
  /** The board it set aside when it was master and a newer master outranked it: since (unix seconds) and master (the number that outranked it). */
  set_aside?: { [key: string]: unknown };
  /** Why it refused the master's build it was offered (a file whose sha256 is not the one named); its row says so in place of taking work. */
  build_refused?: string;
  /** The Mac app around this runner (a cluster engine's), as its shell wrote it: version, and ready when a newer one is downloaded. */
  app?: { [key: string]: unknown };
}; result: {
    /** The agents of the sessions it holds that the master no longer places there: the runner pushes what each left to a rescue branch and stops it. */
    stop: string[];
    /** The programs its agents' MCP servers run, which its next heartbeat says it has or not (has.programs): an agent goes only to a runner with all it needs. */
    check: string[];
    /** To one of the owner's machines only: each of the owner's machines with a door, in the order they joined: {node, addr, fp}. A runner keeps it and, once its master is gone, asks each of them who is master now. */
    doors?: { [key: string]: unknown }[];
    /** When the master wants it on its own build: it runs another version on the master's platform, and it is the owner's with node_build_auto on, or its row's Update was asked (device.update). version, and files: each {name, size, sum} (handlined and hl, sum their sha256). The runner reads each with node.build.read, checks it, keeps it under its home (build/, the newest only) and starts again on it; at a later start it runs the kept build while its master is on that version. Sessions there go on through the restart. */
    build?: { [key: string]: unknown };
    /** When its Mac app is to install the version it has ready: version, and auto (Auto on the owner's Mac with node_build_auto on, installed once no Handline window is open there) or not (its row's Update, device.update, installed at once). The app starts again on it; sessions there go on. */
    app_update?: { [key: string]: unknown };
  } };
  "node.tokens": { params: {
  /** The account's name on that runner. */
  account: string;
  /** Each call as the token log reads it: session, req, ts, model, cwd, side and tokens {inp, cc, cr, out, thinking, cc1h}. */
  calls: { [key: string]: unknown }[];
}; result: { [key: string]: unknown } };
  "node.copy.files": { params: {
  /** The whole files its copy holds: {path, size}. */
  have?: { [key: string]: unknown }[];
  /** The files it is still receiving, with the bytes it has: {path, size}. */
  parts?: { [key: string]: unknown }[];
}; result: {
    /** Pieces of the files it lacks, at most 512 KiB of data in all: {path, size, offset, data}; a file's last piece carries its sha256 sum, which the runner checks before it renames the file in. */
    send: { [key: string]: unknown }[];
    /** Paths the master's copy no longer has; given only with more false, so a runner never drops a file before what replaces it has arrived. */
    gone: string[];
    /** More data waits: pull again. */
    more: boolean;
  } };
  "node.build.read": { params: {
  name: "handlined" | "hl";
  offset: number;
}; result: {
    /** Up to 512 KiB of the file from offset; empty past its end. */
    data: string;
  } };
  "node.promote": { params: { [key: string]: unknown }; result: {
    /** The master number this machine takes. */
    number: number;
    /** The master was reached and handed over, so nothing was lost; false: this machine took over from its copy, and what the master wrote after the copy's last pull is lost. */
    handed_over: boolean;
  } };
  "node.handover": { params: { [key: string]: unknown }; result: {
    /** The master number the runner takes. */
    number: number;
  } };
  "node.outranked": { params: { [key: string]: unknown }; result: { [key: string]: unknown } };
  "node.launch": { params: {
  /** The agent whose session asks, as the runner proved it. */
  agent: string;
  /** session.launch_prepare's own params, as hl launch sent them on the runner. */
  params?: { [key: string]: unknown };
}; result: {
    argv: string[];
    env: string[];
    warnings: string[];
    files: { [key: string]: unknown }[];
    agent_dir: string;
  } };
  "node.agent_files": { params: {
  /** The agent whose session wrote them, as the runner proved it. */
  agent: string;
  /** Each {path under the agent's folder, data in base64, mode}. */
  files?: { [key: string]: unknown }[];
  /** Paths under the agent's folder it removed. */
  deleted?: string[];
}; result: Accepted };
}

export const DEVICES_METHODS: { [K in keyof DevicesMethods]: { role: Role; callers: Role[] | null; write: boolean; since: number } } = {
  "devices.list": { role: "member", callers: null, write: false, since: 1 },
  "device.pair": { role: "member", callers: null, write: true, since: 1 },
  "device.redeem": { role: "anonymous", callers: null, write: true, since: 1 },
  "device.revoke": { role: "member", callers: null, write: true, since: 1 },
  "device.set": { role: "member", callers: null, write: true, since: 1 },
  "device.update": { role: "member", callers: null, write: true, since: 1 },
  "device.make_master": { role: "owner", callers: null, write: true, since: 1 },
  "device.push_token": { role: "member", callers: null, write: true, since: 1 },
  "node.status": { role: "agent", callers: null, write: false, since: 1 },
  "node.forward": { role: "member", callers: null, write: false, since: 1 },
  "node.turn": { role: "member", callers: null, write: false, since: 1 },
  "node.transcript": { role: "member", callers: null, write: false, since: 1 },
  "node.heartbeat": { role: "member", callers: null, write: false, since: 1 },
  "node.tokens": { role: "member", callers: null, write: false, since: 1 },
  "node.copy.files": { role: "member", callers: null, write: false, since: 1 },
  "node.build.read": { role: "member", callers: null, write: false, since: 1 },
  "node.promote": { role: "owner", callers: null, write: false, since: 1 },
  "node.handover": { role: "member", callers: null, write: false, since: 1 },
  "node.outranked": { role: "member", callers: null, write: false, since: 1 },
  "node.launch": { role: "owner", callers: null, write: false, since: 1 },
  "node.agent_files": { role: "owner", callers: null, write: false, since: 1 },
};

/** x-app-cache of each read (x-app in core.json). */
export const DEVICES_APP_CACHE: { [K in "devices.list" | "node.status" | "node.forward" | "node.turn" | "node.transcript" | "node.heartbeat" | "node.tokens" | "node.copy.files" | "node.build.read" | "node.promote" | "node.handover" | "node.outranked" | "node.launch" | "node.agent_files"]: { resource: string; id?: string; feed: Topic | null } | false } = {
  "devices.list": {"resource": "devices", "feed": "devices"},
  "node.status": {"resource": "nodes", "feed": "nodes"},
  "node.forward": false,
  "node.turn": false,
  "node.transcript": false,
  "node.heartbeat": false,
  "node.tokens": false,
  "node.copy.files": false,
  "node.build.read": false,
  "node.promote": false,
  "node.handover": false,
  "node.outranked": false,
  "node.launch": false,
  "node.agent_files": false,
};

/** x-app-write of each write (x-app in core.json). */
export const DEVICES_APP_WRITES: { [K in "device.pair" | "device.redeem" | "device.revoke" | "device.set" | "device.update" | "device.make_master" | "device.push_token"]: { kind: WriteKind; label: string } | false } = {
  "device.pair": {"kind": "decided", "label": "pair"},
  "device.redeem": false,
  "device.revoke": {"kind": "decided", "label": "revoke"},
  "device.set": {"kind": "decided", "label": "set"},
  "device.update": {"kind": "decided", "label": "update"},
  "device.make_master": {"kind": "decided", "label": "make master"},
  "device.push_token": false,
};
