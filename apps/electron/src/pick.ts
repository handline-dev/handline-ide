// Picking one element of a web tab's page with a click. For the length of one pick the shell attaches the page's debugger, turns on
// the browser's own inspect mode (the highlight that follows the pointer, and a click that names an element and does not reach the
// page), and reads the clicked element. Nothing is put into the page and its lockdown (preview.ts) is as it was. The app window
// names only which page: the commands sent are the fixed ones below, and what comes back is cut to size here. Of the page's tree
// the shell asks only for the way down to the clicked element, and a page with more nodes than NODE_CAP is not read at all.
import { boundElement, PICK_LIMITS, type Fail, type PickedElement } from '@handline/app-core';
import { fromWindow, type Sender } from '#src/bridge.ts';
import { pngSize } from '#src/devices.ts';
import { boxIn, locate, scaleOf, stylesOf, textOfHtml, Tree, type DomNode, type Located } from '#src/pick-dom.ts';

type OnDetach = () => void;
type OnMessage = (event: unknown, method: string, params: unknown, sessionId: string) => void;
type PageEvent = 'destroyed' | 'render-process-gone' | 'did-navigate';
/** The part of a page's debugger (Electron's webContents.debugger) a pick uses. */
export interface PickDebugger {
  isAttached(): boolean;
  attach(version?: string): void;
  detach(): void;
  sendCommand(method: string, params?: object, sessionId?: string): Promise<unknown>;
  on(event: 'detach', listener: OnDetach): unknown;
  on(event: 'message', listener: OnMessage): unknown;
  removeListener(event: 'detach', listener: OnDetach): unknown;
  removeListener(event: 'message', listener: OnMessage): unknown;
}
/** The part of a page (Electron's WebContents) a pick uses. */
export interface PickPage {
  hostWebContents: unknown;
  debugger: PickDebugger;
  getType(): string;
  getURL(): string;
  isDestroyed(): boolean;
  capturePage(): Promise<{ isEmpty(): boolean; toPNG(): Buffer }>;
  on(event: PageEvent, listener: () => void): unknown;
  removeListener(event: PageEvent, listener: () => void): unknown;
}
export interface PickDeps {
  /** The page with this id (webContents.fromId). */
  page(id: number): PickPage | null | undefined;
  /** How long a pick waits for its click. */
  limitMs?: number;
  /** How long the highlight is given to leave the screen before the picture is taken. */
  settleMs?: number;
  /** The most nodes a page may hold for an element of it to be read (NODE_CAP). */
  nodeCap?: number;
}

export type PickFail = Fail<'bad_request' | 'busy' | 'cancelled' | 'timeout' | 'other_site' | 'unavailable'>;
export type PickResult = ({ ok: true; png: string } & PickedElement) | PickFail;
/** The IPC event's sender check needs the frame; the page check needs the window that sent it. */
export type Caller = Sender & { sender: unknown };

type Stop = 'cancelled' | 'timeout' | 'navigated' | 'gone' | 'detached';
type Hit = { backendNodeId: number; frame: boolean };
type Send = (method: string, params?: object, session?: string) => Promise<unknown>;
/** One pick's exchange with its page: the commands, the click when it comes, the other-site frames armed, the part of the tree held
 * and the most nodes it may hold. */
type Exchange = { send: Send; clicked: Promise<Hit>; frames: string[]; tree: Tree; cap: number };

const LIMIT_MS = 120_000;
const SETTLE_MS = 100;
/** The most nodes a page's process may hold for an element of it to be read. What reaches the shell is the children of each node on
 * the way down to the element, so a page whose nodes are all one node's children would send every one of them at once (measured:
 * about 3.5 KB of the shell's memory for each, for the length of the read), and the outer HTML of the page's body is as long as the
 * page. Past this many the shell does not ask. */
export const NODE_CAP = 250_000;
const fail = <C extends PickFail['code']>(code: C, message: string): Fail<C> => ({ ok: false, code, message });
const count = (n: number) => n.toLocaleString('en-US');
const tooLarge = (cap: number, held?: number): Fail<'unavailable'> =>
  fail(
    'unavailable',
    `This page is too large to pick from: ${held === undefined ? '' : `it holds ${count(held)} nodes, and `}picking reads pages of up to ${count(cap)}.`,
  );
const STOPPED: Record<Stop, PickFail> = {
  cancelled: fail('cancelled', 'Picking was cancelled.'),
  timeout: fail('timeout', 'No element was picked in time.'),
  navigated: fail('unavailable', 'The page changed before an element was picked.'),
  gone: fail('unavailable', 'The page closed before an element was picked.'),
  // Opening the page's developer tools takes the debugger away: the person chose to, so it ends like Esc.
  detached: fail('cancelled', 'Picking stopped: the developer tools took over the page.'),
};
const rgba = (r: number, g: number, b: number, a: number) => ({ r, g, b, a });
const INSPECT = {
  mode: 'searchForNode',
  highlightConfig: {
    showInfo: true,
    contentColor: rgba(111, 168, 220, 0.66),
    paddingColor: rgba(147, 196, 125, 0.55),
    borderColor: rgba(255, 229, 153, 0.66),
    marginColor: rgba(246, 178, 107, 0.66),
  },
};
const INSPECT_OFF = { mode: 'none', highlightConfig: {} };

export class ElementPicker {
  private origin: string;
  private d: PickDeps;
  /** The pages being picked from, each with the call that stops its pick. */
  private picking = new Map<number, (why: Stop) => void>();
  constructor(origin: string, d: PickDeps) {
    this.origin = origin;
    this.d = d;
  }

  /** Waits for the person's click in that page and answers with the element under it and the page as drawn then. */
  async pick(e: Caller, id: unknown): Promise<PickResult> {
    const own = this.own(e, id);
    if ('ok' in own) return own;
    if (this.picking.has(own.id)) return fail('busy', 'An element is already being picked on this page.');
    return this.run(own.id, own.page);
  }

  /** Stops the pick on that page, if one is waiting: asked by the window the page is in, as the pick was. */
  cancel(e: Caller, id: unknown): { ok: true } | Fail<'bad_request'> {
    const own = this.own(e, id);
    if ('ok' in own) return own;
    this.picking.get(own.id)?.('cancelled');
    return { ok: true };
  }

  /** The web tab page a call is about: named by the app window's top frame, by its id, and one of that window's own pages. */
  private own(e: Caller, id: unknown): { id: number; page: PickPage } | Fail<'bad_request'> {
    if (!fromWindow(e, this.origin)) return fail('bad_request', 'not from the app window');
    if (typeof id !== 'number' || !Number.isInteger(id)) return fail('bad_request', 'id is a number');
    const page = this.d.page(id);
    if (!page || !page.hostWebContents || page.hostWebContents !== e.sender) return fail('bad_request', 'not a page of this window');
    if (page.getType() !== 'webview') return fail('bad_request', 'not a web tab page');
    return { id, page };
  }

  private async run(id: number, page: PickPage): Promise<PickResult> {
    const dbg = page.debugger;
    let stop!: (why: Stop) => void;
    const stopped = new Promise<Stop>((resolve) => (stop = resolve));
    let hit!: (h: Hit) => void;
    const clicked = new Promise<Hit>((resolve) => (hit = resolve));
    const frames: string[] = [];
    const cap = this.d.nodeCap ?? NODE_CAP;
    const tree = new Tree(cap);
    const send: Send = (method, params = {}, session) => dbg.sendCommand(method, params, session);
    const onMessage: OnMessage = (_e, method, params, session) => {
      const p = (params ?? {}) as { backendNodeId?: unknown; sessionId?: unknown; targetInfo?: { type?: unknown } };
      if (method === 'Overlay.inspectNodeRequested') hit({ backendNodeId: Number(p.backendNodeId), frame: !!session });
      else if (method === 'Overlay.inspectModeCanceled') stop('cancelled');
      else if (method === 'DOM.setChildNodes' && !session) takeChildren(tree, params);
      else if (method === 'Target.attachedToTarget' && typeof p.sessionId === 'string' && p.targetInfo?.type === 'iframe')
        armFrame(send, p.sessionId, frames);
    };
    const onDetach = () => stop('detached');
    const onGone = () => stop('gone');
    const onNavigate = () => stop('navigated');
    const timer = setTimeout(() => stop('timeout'), this.d.limitMs ?? LIMIT_MS);
    this.picking.set(id, stop);
    try {
      try {
        dbg.attach('1.3');
      } catch {
        return fail('unavailable', "Close the page's developer tools, then pick again.");
      }
      dbg.on('message', onMessage);
      dbg.on('detach', onDetach);
      page.on('destroyed', onGone);
      page.on('render-process-gone', onGone);
      page.on('did-navigate', onNavigate);
      const done = await Promise.race([this.read(page, { send, clicked, frames, tree, cap }), stopped]);
      return typeof done === 'string' ? STOPPED[done] : done;
    } catch (err) {
      return fail('unavailable', `The element could not be read: ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      clearTimeout(timer);
      this.picking.delete(id);
      dbg.removeListener('message', onMessage);
      dbg.removeListener('detach', onDetach);
      page.removeListener('destroyed', onGone);
      page.removeListener('render-process-gone', onGone);
      page.removeListener('did-navigate', onNavigate);
      release(page);
    }
  }

  /** The whole exchange with the page, in order. */
  private async read(page: PickPage, x: Exchange): Promise<PickResult> {
    const { send } = x;
    await send('DOM.enable');
    await send('Overlay.enable');
    // A frame from another site is a page of its own: it is told to wait for the click too, so a click there is known and is not a click on the page.
    await send('Target.setAutoAttach', { autoAttach: true, waitForDebuggerOnStart: false, flatten: true });
    await send('Overlay.setInspectMode', INSPECT);
    const { backendNodeId, frame } = await x.clicked;
    // The highlight goes before the picture is taken.
    await send('Overlay.setInspectMode', INSPECT_OFF);
    await send('Overlay.hideHighlight');
    await Promise.all(x.frames.map((s) => send('Overlay.setInspectMode', INSPECT_OFF, s).catch(() => undefined)));
    if (frame) return fail('other_site', 'That is inside a frame from another site. Elements there cannot be picked yet.');
    const at = await this.find(x, backendNodeId);
    if ('ok' in at) return at;
    const node = { backendNodeId: at.node.backendNodeId };
    await send('CSS.enable');
    const style = (await send('CSS.getComputedStyleForNode', { nodeId: at.node.nodeId })) as {
      computedStyle?: { name: string; value: string }[];
    };
    const got = (await send('DOM.getOuterHTML', node)) as { outerHTML?: unknown };
    const html = typeof got.outerHTML === 'string' ? got.outerHTML : '';
    // An element with nothing drawn has no box: it goes without one.
    const model = (await send('DOM.getBoxModel', node).catch(() => null)) as { model?: { border?: number[] } } | null;
    const metrics = (await send('Page.getLayoutMetrics')) as Parameters<typeof scaleOf>[0];
    await new Promise((resolve) => setTimeout(resolve, this.d.settleMs ?? SETTLE_MS));
    const image = await page.capturePage();
    if (image.isEmpty()) return fail('unavailable', 'The page is blank.');
    const png = image.toPNG();
    const size = pngSize(png) ?? { width: 0, height: 0 };
    const element = boundElement({
      page: page.getURL(),
      tag: at.tag,
      id: at.id,
      classes: at.classes,
      path: at.path,
      within: at.within,
      text: textOfHtml(html, PICK_LIMITS.text),
      styles: stylesOf(style.computedStyle ?? []),
      html,
      box: boxIn(model?.model?.border, scaleOf(metrics, size.width), size),
    });
    return { ok: true, png: png.toString('base64'), ...element };
  }

  /** Where the clicked node is in its page. The page is first asked how many nodes it holds, a count it keeps, and one past NODE_CAP
   * is refused unread. Then only the way down to the node is asked for: the page sends the children of each node on that way
   * (DOM.setChildNodes, kept in `tree`), never the page whole. */
  private async find(x: Exchange, backendNodeId: number): Promise<Located | Fail<'unavailable'>> {
    const nodes = async () => ((await x.send('Memory.getDOMCounters')) as { nodes?: unknown }).nodes;
    let held = await nodes();
    // The count is of the page's whole process, and a page left behind is counted until its memory is collected: collected once, then counted again.
    if (typeof held === 'number' && held > x.cap) {
      await x.send('HeapProfiler.collectGarbage');
      held = await nodes();
    }
    if (typeof held !== 'number') return fail('unavailable', 'The page did not say how large it is, so nothing of it was read.');
    if (held > x.cap) return tooLarge(x.cap, held);
    const doc = (await x.send('DOM.getDocument', { depth: 0 })) as { root?: DomNode };
    if (doc.root) x.tree.document(doc.root);
    const pushed = (await x.send('DOM.pushNodesByBackendIdsToFrontend', { backendNodeIds: [backendNodeId] })) as { nodeIds?: unknown };
    // The page grew between the count and the read.
    if (x.tree.full) return tooLarge(x.cap);
    const nodeId = Array.isArray(pushed.nodeIds) ? Number(pushed.nodeIds[0]) : 0;
    const chain = x.tree.chainTo(nodeId);
    return (chain && locate(chain)) ?? fail('unavailable', 'That element is no longer on the page.');
  }
}

/** One DOM.setChildNodes event of the page's own session, into the part of the tree held. */
function takeChildren(tree: Tree, params: unknown): void {
  const p = (params ?? {}) as { parentId?: unknown; nodes?: unknown };
  if (typeof p.parentId === 'number' && Array.isArray(p.nodes)) tree.children(p.parentId, p.nodes as DomNode[]);
}

/** Puts a frame from another site in inspect mode on its own session; one that cannot be is left as it is. */
function armFrame(send: Send, session: string, frames: string[]): void {
  frames.push(session);
  void (async () => {
    await send('DOM.enable', {}, session);
    await send('Overlay.enable', {}, session);
    await send('Overlay.setInspectMode', INSPECT, session);
  })().catch(() => undefined);
}

/** Lets go of the page: detaching ends inspect mode and every session with it. */
function release(page: PickPage): void {
  try {
    if (!page.isDestroyed() && page.debugger.isAttached()) page.debugger.detach();
  } catch {
    /* already let go */
  }
}
