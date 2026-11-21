// Generated from contract/spec/ by contract/generate.py; do not edit.
/* eslint-disable */
import type { Accepted, RequestId, Role, Timestamp, Topic, WriteKind } from './core.ts';

export type Device = {
  id: string;
  name: string;
  /** A paired device acts with its person's role: the owner's, before team mode. */
  role: "owner";
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
  /** The master it belongs to; 1 until masters can change. */
  master_number: number;
  /** It is the master. */
  master: boolean;
  /** It is the machine this daemon runs on. */
  self: boolean;
  /** The master hears from it now; always true for the master's own row. */
  online: boolean;
  /** run: the master starts new work there; pause: it starts nothing new there and what runs finishes. The master's own row is always run; a machine on off is not listed. */
  lending: "run" | "pause";
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
  /** A host a door serves; absent is the default door: -advertise (or the listener) while network_addresses is unset, else the first LAN host served, else the first host served. Any other is refused, gate address, with data.addresses the hosts served, the default first. */
  address?: string;
  request_id: RequestId;
}; result: Accepted & {
    code: string;
    expires_at: Timestamp;
    /** Certificate fingerprint of the door; empty on a plain door. */
    fingerprint: string;
    /** What the QR carries: handline://pair with the host:port and, on a TLS door, the fingerprint. */
    uri: string;
    /** The browser's invite on the same door: http(s)://host:port/#pair=code. null when the web page is not served there. */
    web_link: string | null;
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
  lending: "run" | "pause" | "off";
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
}

export const DEVICES_METHODS: { [K in keyof DevicesMethods]: { role: Role; callers: Role[] | null; write: boolean; since: number } } = {
  "devices.list": { role: "owner", callers: null, write: false, since: 1 },
  "device.pair": { role: "owner", callers: null, write: true, since: 1 },
  "device.redeem": { role: "anonymous", callers: null, write: true, since: 1 },
  "device.revoke": { role: "owner", callers: null, write: true, since: 1 },
  "device.set": { role: "owner", callers: null, write: true, since: 1 },
  "device.push_token": { role: "owner", callers: null, write: true, since: 1 },
  "node.status": { role: "agent", callers: null, write: false, since: 1 },
  "node.forward": { role: "owner", callers: null, write: false, since: 1 },
};

/** x-app-cache of each read (x-app in core.json). */
export const DEVICES_APP_CACHE: { [K in "devices.list" | "node.status" | "node.forward"]: { resource: string; id?: string; feed: Topic | null } | false } = {
  "devices.list": {"resource": "devices", "feed": "devices"},
  "node.status": {"resource": "nodes", "feed": "nodes"},
  "node.forward": false,
};

/** x-app-write of each write (x-app in core.json). */
export const DEVICES_APP_WRITES: { [K in "device.pair" | "device.redeem" | "device.revoke" | "device.set" | "device.push_token"]: { kind: WriteKind; label: string } | false } = {
  "device.pair": {"kind": "decided", "label": "pair"},
  "device.redeem": false,
  "device.revoke": {"kind": "decided", "label": "revoke"},
  "device.set": {"kind": "decided", "label": "set"},
  "device.push_token": false,
};
