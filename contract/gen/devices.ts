// Generated from contract/spec/ by contract/generate.py; do not edit.
/* eslint-disable */
import type { Accepted, RequestId, Role, Timestamp, Topic, WriteKind } from './core.ts';

export type Device = {
  id: string;
  name: string;
  /** A paired device is the founder. */
  role: "founder";
  created: Timestamp;
  last_seen: Timestamp | null;
  connected: boolean;
  /** Whether the device gave a push token (device.push_token) that the daemon still holds. */
  push: boolean;
  /** What the client said it is at device.redeem; null when it did not say. Absent on a daemon that does not serve it. */
  kind?: ("ios" | "android" | "web" | "desktop") | null;
  /** True in devices.list for the caller's own device; false in events. Absent on a daemon that does not serve it. */
  current?: boolean;
  /** The client its last hello named (hello's client); null before one. Absent on a daemon that does not serve it. */
  client?: string | null;
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
  /** A host a door serves (connection.get listeners); absent is the default door (listener). Any other is refused, gate address, with data.addresses the hosts served. */
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
  /** What the client is: ios, android, web or desktop, kept as Device.kind. Any other value is kept as null, so a newer client still pairs. */
  kind?: string;
  request_id: RequestId;
}; result: Accepted & {
    token: string;
    device: string;
  } };
  "device.revoke": { params: {
  device: string;
  request_id: RequestId;
}; result: Accepted };
  "device.rename": { params: {
  device: string;
  /** Stored trimmed: not empty, at most 64 characters. */
  name: string;
  request_id: RequestId;
}; result: Accepted };
  "device.push_token": { params: {
  platform: "apns" | "fcm";
  token: string;
  /** An APNs token of a development build, which goes to Apple's sandbox host. Default false. */
  sandbox?: boolean;
  request_id: RequestId;
}; result: Accepted };
}

export const DEVICES_METHODS: { [K in keyof DevicesMethods]: { role: Role; callers: Role[] | null; write: boolean; since: number } } = {
  "devices.list": { role: "founder", callers: null, write: false, since: 1 },
  "device.pair": { role: "founder", callers: null, write: true, since: 1 },
  "device.redeem": { role: "anonymous", callers: null, write: true, since: 1 },
  "device.revoke": { role: "founder", callers: null, write: true, since: 1 },
  "device.rename": { role: "founder", callers: null, write: true, since: 1 },
  "device.push_token": { role: "founder", callers: null, write: true, since: 1 },
};

/** x-app-cache of each read (x-app in core.json). */
export const DEVICES_APP_CACHE: { [K in "devices.list"]: { resource: string; id?: string; feed: Topic | null } | false } = {
  "devices.list": {"resource": "devices", "feed": "devices"},
};

/** x-app-write of each write (x-app in core.json). */
export const DEVICES_APP_WRITES: { [K in "device.pair" | "device.redeem" | "device.revoke" | "device.rename" | "device.push_token"]: { kind: WriteKind; label: string } | false } = {
  "device.pair": {"kind": "decided", "label": "pair"},
  "device.redeem": false,
  "device.revoke": {"kind": "decided", "label": "revoke"},
  "device.rename": {"kind": "decided", "label": "rename device"},
  "device.push_token": false,
};
