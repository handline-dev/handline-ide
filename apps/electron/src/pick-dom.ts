// What the shell reads of a picked element from the page's own description of itself (the DevTools protocol's DOM nodes, outer HTML,
// box model, layout metrics and computed style): where the element is, a CSS path to it, its text, its box in the picture. Of the
// page's tree it holds the way down to the element and no more. No Electron in it.
import { PICK_STYLES, type MarkRect } from '@handline/app-core';

/** A node as the DevTools protocol's DOM domain describes it: the fields read here. */
export interface DomNode {
  nodeId: number;
  backendNodeId: number;
  /** 1 an element, 3 text, 9 a document, 11 a shadow root. */
  nodeType: number;
  localName?: string;
  nodeValue?: string;
  /** Names and values in turn. */
  attributes?: string[];
  pseudoType?: string;
  children?: DomNode[];
  contentDocument?: DomNode;
  shadowRoots?: DomNode[];
  pseudoElements?: DomNode[];
}

/** The part of a page's tree the shell holds for one pick: the document, and the children of each node on the way down to the
 * picked one, as the page sends them (DOM.setChildNodes) when that node's path is asked for. Never the whole page, and never more
 * than `max` nodes: past that it takes no more and says so (`full`). */
export class Tree {
  full = false;
  private max: number;
  private at = new Map<number, { node: DomNode; parent: number | null }>();
  constructor(max: number) {
    this.max = max;
  }

  /** Starts over from this document. */
  document(root: DomNode): void {
    this.at.clear();
    this.full = false;
    this.add(root, null);
  }

  /** The children of the node with this id; those of a node not held are let go. */
  children(parent: number, nodes: DomNode[]): void {
    const p = this.at.get(parent);
    if (!p || this.full) return;
    p.node.children = nodes;
    for (const n of nodes) this.add(n, parent);
  }

  /** The nodes from the document down to the one with this id, through frames and shadow roots; null when it is not held. */
  chainTo(nodeId: number): DomNode[] | null {
    const chain: DomNode[] = [];
    let id: number | null = nodeId;
    // No path is longer than the nodes held: a loop in what the page sent ends here.
    for (let steps = 0; id !== null && steps <= this.at.size; steps++) {
      const e = this.at.get(id);
      if (!e) return null;
      chain.push(e.node);
      id = e.parent;
    }
    return id === null && chain.length ? chain.reverse() : null;
  }

  /** Holds a node and what it carries inside it (a frame's document, shadow roots, pseudo-elements, children already there). */
  private add(node: DomNode, parent: number | null): void {
    const todo: [DomNode, number | null][] = [[node, parent]];
    while (todo.length) {
      const [n, up] = todo.pop()!;
      if (this.at.size >= this.max) return void (this.full = true);
      this.at.set(n.nodeId, { node: n, parent: up });
      for (const inner of [n.contentDocument, ...(n.shadowRoots ?? []), ...(n.pseudoElements ?? []), ...(n.children ?? [])])
        if (inner) todo.push([inner, n.nodeId]);
    }
  }
}

const isElement = (n: DomNode): boolean => n.nodeType === 1 && !n.pseudoType;
const attr = (n: DomNode, name: string): string => {
  const a = n.attributes ?? [];
  for (let i = 0; i + 1 < a.length; i += 2) if (a[i] === name) return a[i + 1]!;
  return '';
};
const tagOf = (n: DomNode): string => (n.localName ?? '').toLowerCase();
const NAME = /^[A-Za-z_][\w-]*$/;

/** One step of a path: the tag, with its id when that is a plain name, else its place among its parent's elements of that tag. */
function step(n: DomNode, parent: DomNode | undefined): { text: string; anchored: boolean } {
  const tag = NAME.test(tagOf(n)) ? tagOf(n) : '*';
  const id = attr(n, 'id');
  if (NAME.test(id)) return { text: `${tag}#${id}`, anchored: true };
  const same = (parent?.children ?? []).filter((c) => isElement(c) && tagOf(c) === tagOf(n));
  return { text: same.length > 1 ? `${tag}:nth-of-type(${same.indexOf(n) + 1})` : tag, anchored: false };
}

/** A CSS path to the last of `chain` inside one document or shadow root (`chain` starts at that root): from the nearest element
 * with an id, or from the top. */
function pathIn(chain: DomNode[]): string {
  const steps: string[] = [];
  for (let i = chain.length - 1; i >= 1; i--) {
    const s = step(chain[i]!, chain[i - 1]);
    steps.unshift(s.text);
    if (s.anchored) break;
  }
  return steps.join(' > ');
}

const SKIP_TEXT = new Set(['script', 'style', 'noscript', 'template']);

const ENTITIES: Record<string, string> = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'", nbsp: ' ' };
/** The most of an element's HTML read for its text. */
const TEXT_SCAN = 1_000_000;

/** Where the tag that opens at `from` ends: after its `>`, which a quoted attribute value may hold too. */
function tagEnd(html: string, from: number, stop: number): number {
  let quote = '';
  for (let i = from + 1; i < stop; i++) {
    const c = html[i]!;
    if (quote) quote = c === quote ? '' : quote;
    else if (c === '"' || c === "'") quote = c;
    else if (c === '>') return i + 1;
  }
  return stop;
}

/** The text an element's HTML holds, spaces collapsed: what is between its tags, without comments, scripts and styles. Read from
 * the front until `max` characters are in hand, in one pass over at most the first million characters, however deep the HTML nests. */
export function textOfHtml(html: string, max: number): string {
  const stop = Math.min(html.length, TEXT_SCAN);
  const parts: string[] = [];
  let have = 0;
  let i = 0;
  while (i < stop && have < max) {
    const lt = html.indexOf('<', i);
    const end = lt < 0 || lt > stop ? stop : lt;
    const piece = html.slice(i, Math.min(end, i + max * 8)).trim();
    if (piece) {
      parts.push(piece);
      have += piece.length;
    }
    if (end === stop) break;
    if (html.startsWith('<!--', lt)) {
      const close = html.indexOf('-->', lt + 4);
      i = close < 0 ? stop : close + 3;
      continue;
    }
    const name = /^<([A-Za-z][^\s/>]*)/.exec(html.slice(lt, lt + 40))?.[1]?.toLowerCase();
    i = tagEnd(html, lt, stop);
    if (!name || !SKIP_TEXT.has(name)) continue;
    // What such an element holds is not text to read: on to where it closes.
    const closes = new RegExp(`</${name}`, 'gi');
    closes.lastIndex = i;
    i = closes.exec(html)?.index ?? stop;
  }
  return decode(parts.join(' ')).replace(/\s+/g, ' ').trim();
}

/** The few character references HTML text is written with, as their characters. */
const decode = (text: string): string =>
  text.replace(/&(?:#(\d{1,7})|#x([0-9a-f]{1,6})|([a-z]{2,6}));/gi, (all: string, dec?: string, hex?: string, named?: string) => {
    if (named) return ENTITIES[named.toLowerCase()] ?? all;
    const code = dec ? Number(dec) : parseInt(hex ?? '', 16);
    return code > 0 && code <= 0x10ffff ? String.fromCodePoint(code) : all;
  });

export interface Located {
  /** The element itself: what was clicked, or the element a clicked pseudo-element or text belongs to. */
  node: DomNode;
  tag: string;
  id: string;
  classes: string[];
  path: string;
  within: string[];
}

/** The element a chain of Tree.chainTo ends in, with its path in its own document and the frames and shadow hosts round it. Null
 * when the chain holds no element. */
export function locate(chain: DomNode[]): Located | null {
  let end = chain.length - 1;
  while (end >= 0 && !isElement(chain[end]!)) end--;
  if (end < 0) return null;
  const node = chain[end]!;
  // One part per document or shadow root on the way down, each starting at its root.
  const parts: DomNode[][] = [];
  for (const n of chain.slice(0, end + 1)) {
    if (n.nodeType === 9 || n.nodeType === 11 || !parts.length) parts.push([n]);
    else parts.at(-1)!.push(n);
  }
  const own = parts.pop()!;
  return {
    node,
    tag: tagOf(node),
    id: attr(node, 'id'),
    classes: attr(node, 'class').split(/\s+/).filter(Boolean),
    path: pathIn(own),
    // Each part above ends in the frame or the shadow host the next one is inside.
    within: parts.map((p, i) => `${pathIn(p)}${(parts[i + 1] ?? own)[0]!.nodeType === 11 ? ' (shadow)' : ''}`),
  };
}

type Viewport = { clientWidth?: number } | undefined;

/** Picture pixels per CSS pixel of the page: the device's scale times the page's zoom, as the page's layout says it. The picture's
 * width over the viewport's stands in when the layout does not say (a scrollbar makes that a little large). */
export function scaleOf(metrics: { layoutViewport?: Viewport; cssLayoutViewport?: Viewport }, pictureWidth: number): number {
  const css = metrics.cssLayoutViewport?.clientWidth ?? 0;
  const device = metrics.layoutViewport?.clientWidth ?? 0;
  if (!(css > 0)) return 1;
  return device > 0 ? device / css : pictureWidth > 0 ? pictureWidth / css : 1;
}

/** A box model's quad (four corners, x and y in turn, CSS pixels of the viewport) as a rect in the picture's pixels, cut to the
 * picture. Null when nothing of it is in the picture. */
export function boxIn(quad: readonly number[] | undefined, scale: number, picture: { width: number; height: number }): MarkRect | null {
  if (!quad || quad.length !== 8 || !quad.every(Number.isFinite) || !(scale > 0)) return null;
  const xs = [quad[0]!, quad[2]!, quad[4]!, quad[6]!].map((v) => v * scale);
  const ys = [quad[1]!, quad[3]!, quad[5]!, quad[7]!].map((v) => v * scale);
  const left = Math.max(0, Math.min(...xs));
  const top = Math.max(0, Math.min(...ys));
  const right = Math.min(picture.width, Math.max(...xs));
  const bottom = Math.min(picture.height, Math.max(...ys));
  if (right - left < 1 || bottom - top < 1) return null;
  return { x: Math.round(left), y: Math.round(top), w: Math.round(right - left), h: Math.round(bottom - top) };
}

const SIDES = ['top', 'right', 'bottom', 'left'] as const;

/** The listed styles out of a computed style. The page gives margin and padding a side at a time: the four make the one value. */
export function stylesOf(computed: readonly { name: string; value: string }[]): Record<string, string> {
  const all = new Map(computed.map((p) => [p.name, p.value]));
  const out: Record<string, string> = {};
  for (const name of PICK_STYLES) {
    const direct = all.get(name);
    if (direct) {
      out[name] = direct;
      continue;
    }
    const sides = SIDES.map((s) => all.get(`${name}-${s}`));
    if (sides.some((s) => !s)) continue;
    const [t, r, b, l] = sides as string[];
    out[name] = r !== l ? `${t} ${r} ${b} ${l}` : t !== b ? `${t} ${r} ${b}` : t !== r ? `${t} ${r}` : t!;
  }
  return out;
}
