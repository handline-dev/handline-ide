import { useEffect, useRef, type RefObject } from 'react';

type Layer = { take: () => void; root?: RefObject<HTMLElement | null> };

/** Open layers, oldest first: Esc goes to the newest only, so a note closes before the viewer it sits on. */
const layers: Layer[] = [];
const POPUP = '[role="dialog"], [role="alertdialog"], [role="menu"], [role="listbox"]';

/** A popup the top layer does not hold (a menu or modal opened over it) keeps its own Esc. */
function heldElsewhere(top: Layer, target: EventTarget | null): boolean {
  const popup = target instanceof Element ? target.closest(POPUP) : null;
  if (!popup) return false;
  const root = top.root?.current;
  return !root || !root.contains(popup);
}

function take(e: KeyboardEvent, top: Layer) {
  e.preventDefault();
  e.stopPropagation();
  top.take();
}

/** A layer with an element (a full-screen viewer) takes a key pressed inside it before anything under it does. */
function onCapture(e: KeyboardEvent) {
  const top = layers.at(-1);
  const root = top?.root?.current;
  if (e.key !== 'Escape' || e.isComposing || !top || !root || !(e.target instanceof Node) || !root.contains(e.target)) return;
  if (!heldElsewhere(top, e.target)) take(e, top);
}

/** Anywhere else, Esc reaches the top layer last, after the focused field or popup had its turn. */
function onBubble(e: KeyboardEvent) {
  const top = layers.at(-1);
  if (e.key !== 'Escape' || e.defaultPrevented || e.isComposing || !top || heldElsewhere(top, e.target)) return;
  take(e, top);
}

/** Exported for the order test. */
export function pushEscapeLayer(layer: Layer): () => void {
  if (!layers.length) {
    addEventListener('keydown', onCapture, true);
    document.addEventListener('keydown', onBubble);
  }
  layers.push(layer);
  return () => {
    const i = layers.indexOf(layer);
    if (i >= 0) layers.splice(i, 1);
    if (!layers.length) {
      removeEventListener('keydown', onCapture, true);
      document.removeEventListener('keydown', onBubble);
    }
  };
}

/** While `on`, this is a layer Esc closes: the newest open layer takes Esc, one layer per press.
 * `root` is the layer's element, so a popup inside it does not count as one over it. */
export function useEscape(on: boolean, onEscape: () => void, root?: RefObject<HTMLElement | null>) {
  const latest = useRef(onEscape);
  useEffect(() => {
    latest.current = onEscape;
  });
  useEffect(() => {
    if (!on) return;
    return pushEscapeLayer({ take: () => latest.current(), root });
  }, [on, root]);
}
