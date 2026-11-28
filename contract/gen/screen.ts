// Generated from contract/spec/ by contract/generate.py; do not edit.
/* eslint-disable */
import type { Accepted, RequestId, Role, Topic, Who, WriteKind } from './core.ts';

/** A kind of screen an agent can have open: its browser (hl browser). Later kinds arrive on the same methods. */
export type ScreenSource = "browser";

export type ScreenFrame = {
  /** Base64 of a JPEG. */
  data: string;
  width: number;
  /** With width, the size of what the frame shows in the units screen.input and ScreenBox use; the picture may be smaller. */
  height: number;
};

/** Who drives the source: the agent unless a person or device took it. */
export type ScreenHolder = {
  kind: "agent" | "person" | "device";
  name: string;
  /** When the person or device took it; null for the agent. */
  since: unknown;
};

export type ScreenSnapshot = {
  agent: string;
  source: ScreenSource;
  /** The frame's number; each screen_frame after it is one more. */
  seq: number;
  frame: ScreenFrame | null;
  holder: ScreenHolder;
  in_use: boolean;
};

/** A box on the source from its top left, in the frame's units. */
export type ScreenBox = {
  x: number;
  y: number;
  width: number;
  height: number;
};

export type ScreenInput = {
  kind: "mouse" | "wheel" | "key" | "text";
  /** mouse: move, down or up; key: down or up. */
  action?: "move" | "down" | "up";
  x?: number;
  y?: number;
  /** A mouse down or up's. */
  button?: "left" | "middle" | "right";
  /** A mouse down or up's click count; default 1. */
  clicks?: number;
  dx?: number;
  dy?: number;
  /** The DOM key value (Enter, a, ArrowUp). */
  key?: string;
  /** The DOM code (KeyA). */
  code?: string;
  /** key down: the text it types (\r for Enter), absent for a key that types nothing; text: 1 to 16384 bytes, pasted as it is. */
  text?: string;
  /** Alt 1, Ctrl 2, Meta 4, Shift 8, summed. */
  modifiers?: number;
};

export interface ScreenMethods {
  "screen.attach": { params: {
  agent: Who;
  source: ScreenSource;
}; result: ScreenSnapshot };
  "screen.detach": { params: {
  agent: Who;
  source: ScreenSource;
}; result: { [key: string]: unknown } };
  "screen.take": { params: {
  agent: Who;
  source: ScreenSource;
  /** Take it from the person or device holding it instead of being refused; the screen_holder notice tells them. */
  force?: boolean;
  request_id: RequestId;
}; result: Accepted & {
    holder: ScreenHolder;
  } };
  "screen.give_back": { params: {
  agent: Who;
  source: ScreenSource;
  request_id: RequestId;
}; result: Accepted & {
    holder: ScreenHolder;
  } };
  "screen.input": { params: {
  agent: Who;
  source: ScreenSource;
  event: ScreenInput;
  request_id: RequestId;
}; result: Accepted };
  "screen.act": { params: {
  source: ScreenSource;
  /** open, click, type, key, scroll, eval and close act on the source and are sent to watchers as screen_action; read, wait and shot only read it. */
  kind: "open" | "click" | "type" | "key" | "scroll" | "eval" | "close" | "read" | "wait" | "shot";
  /** The target's box on the page. click and type are called once before the target is found, which only checks the hold, and once with its box, which is shown. */
  box?: ScreenBox;
}; result: Accepted };
}

export const SCREEN_METHODS: { [K in keyof ScreenMethods]: { role: Role; callers: Role[] | null; write: boolean; since: number } } = {
  "screen.attach": { role: "member", callers: null, write: false, since: 1 },
  "screen.detach": { role: "member", callers: null, write: false, since: 1 },
  "screen.take": { role: "owner", callers: null, write: true, since: 1 },
  "screen.give_back": { role: "owner", callers: null, write: true, since: 1 },
  "screen.input": { role: "owner", callers: null, write: true, since: 1 },
  "screen.act": { role: "agent", callers: null, write: false, since: 1 },
};

/** x-app-cache of each read (x-app in core.json). */
export const SCREEN_APP_CACHE: { [K in "screen.attach" | "screen.detach" | "screen.act"]: { resource: string; id?: string; feed: Topic | null } | false } = {
  "screen.attach": false,
  "screen.detach": false,
  "screen.act": false,
};

/** x-app-write of each write (x-app in core.json). */
export const SCREEN_APP_WRITES: { [K in "screen.take" | "screen.give_back" | "screen.input"]: { kind: WriteKind; label: string } | false } = {
  "screen.take": {"kind": "decided", "label": "take over"},
  "screen.give_back": {"kind": "decided", "label": "return control"},
  "screen.input": {"kind": "uncached", "label": "key"},
};
