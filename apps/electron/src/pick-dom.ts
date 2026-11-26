// What the shell reads of a picked element from the page's own description of itself (the DevTools protocol's DOM tree, box model,
// layout metrics and computed style): where the element is, a CSS path to it, its text, its box in the picture. No Electron in it.
import { PICK_STYLES, type MarkRect } from '@handline/app-core';

/** A node of DOM.getDocument's tree: the fields read here. */
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

const kids = (n: DomNode): DomNode[] => [
  ...(n.children ?? []),
  ...(n.shadowRoots ?? []),
  ...(n.pseudoElements ?? []),
  ...(n.contentDocument ? [n.contentDocument] : []),
];

/** The nodes from the document down to the one with this backend id, through frames and shadow roots; null when the tree has none. */
export function chainTo(root: DomNode, backendNodeId: number): DomNode[] | null {
  // Its own stack: a page nests as deep as it likes.
  const stack: { node: DomNode; next: number; kids: DomNode[] }[] = [{ node: root, next: 0, kids: kids(root) }];
  while (stack.length) {
    const top = stack.at(-1)!;
    if (top.node.backendNodeId === backendNodeId) return stack.map((s) => s.node);
    const child = top.kids[top.next++];
    if (child) stack.push({ node: child, next: 0, kids: kids(child) });
    else stack.pop();
  }
  return null;
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

/** The text under an element as the page holds it, in document order, read until `max` characters are in hand. */
function textOf(n: DomNode, max: number): string {
  const parts: string[] = [];
  let have = 0;
  const walk = (node: DomNode) => {
    if (have >= max) return;
    if (node.nodeType === 3) {
      parts.push(node.nodeValue ?? '');
      have += node.nodeValue?.length ?? 0;
    } else if (!SKIP_TEXT.has(tagOf(node))) for (const c of node.children ?? []) walk(c);
  };
  walk(n);
  return parts.join(' ').replace(/\s+/g, ' ').trim();
}

export interface Located {
  /** The element itself: what was clicked, or the element a clicked pseudo-element or text belongs to. */
  node: DomNode;
  tag: string;
  id: string;
  classes: string[];
  path: string;
  within: string[];
  text: string;
}

/** The element a chain of chainTo ends in, with its path in its own document and the frames and shadow hosts round it. Null when
 * the chain holds no element. */
export function locate(chain: DomNode[], textMax: number): Located | null {
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
    text: textOf(node, textMax),
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
