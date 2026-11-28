import type { MethodName, Methods, Notifications, RequestId } from '#src/protocol.ts';
import type { Transport } from '#src/transport.ts';
import type { MasterSays } from '#src/master.ts';

/** A call's params; the client fills in request_id on writes. */
export type CallParams<M extends MethodName> = Omit<Methods[M]['params'], 'request_id'> & { request_id?: RequestId };

export type HelloResult = Methods['hello']['result'];

export type ConnectionState = 'connecting' | 'open' | 'reconnecting' | 'closed';

export interface ClientOptions {
  connect: () => Promise<Transport>;
  client: string; // who is calling; the daemon names this actor on what it does
  token?: string | (() => string | undefined); // the device token from pairing, or read at each hello; the network listener admits no one without it
  key?: () => string | null; // the founder key, read at each hello: the person's own app proves itself on the Unix socket with it
  founder?: boolean | (() => boolean); // read at each hello: a hello not answered as the founder (no key yet) drops the connection to be retried, not kept at a lower role
  reconnect?: { minMs: number; maxMs: number } | false;
  newRequestId?: () => string;
  /** Methods answered by something other than the daemon connection (the desktop shell, for calls this caller may not make itself); each throws what a failed call throws. */
  relay?: { [M in MethodName]?: (params: CallParams<M>) => Promise<Methods[M]['result']> };
  /** The highest master seen: a hello naming a lower one is refused (and `below` told), so the next connect is tried. */
  master?: { floor: () => MasterSays | null; below?: (said: MasterSays) => void };
}

/** A doc notice, its method in `kind`. */
export type DocNotice = ({ kind: 'doc_changed' } & Notifications['doc_changed']) | ({ kind: 'doc_closed' } & Notifications['doc_closed']);

/** A watched screen source's notice, its method in `kind`. */
export type ScreenNotice =
  | ({ kind: 'screen_frame' } & Notifications['screen_frame'])
  | ({ kind: 'screen_action' } & Notifications['screen_action'])
  | ({ kind: 'screen_holder' } & Notifications['screen_holder'])
  | ({ kind: 'screen_ended' } & Notifications['screen_ended']);

export type Handler<T> = (value: T) => void;

/** A call sent and not yet answered. */
export interface Pending {
  id: number;
  method: string;
  params: unknown;
  resolve: (v: unknown) => void;
  reject: (e: Error) => void;
}
