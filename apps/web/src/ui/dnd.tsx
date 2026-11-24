/** Drag and drop into the workspace: one payload format, and the geometry of where a drop lands on a pane
 * or a tab bar. Anything that can become a tab is a source: a tab moving from its pane, or a thing to open
 * (an agent in the sidebar, say). */
import { motion } from 'framer-motion';
import { useEffect, useSyncExternalStore, type DragEvent } from 'react';
import { spring } from '#src/motion.ts';
import { listeners } from '#src/persist/store.ts';

const MIME = 'application/x-handline-tab';
/** Also set on a tab that moves, so a place that takes only those can tell during dragover. */
const MOVE = 'application/x-handline-tab-move';
export type TabMove = { pane: string; tab: string; tabs?: string[] };
export type TabDrag = { move: TabMove } | { open: { opens: string; title: string } };

export function startTabDrag(e: DragEvent, drag: TabDrag) {
  e.dataTransfer.setData(MIME, JSON.stringify(drag));
  if ('move' in drag) e.dataTransfer.setData(MOVE, '');
  e.dataTransfer.effectAllowed = 'move' in drag ? 'move' : 'copy';
}
/** True during dragover too, when the payload itself cannot be read yet. */
export const isTabDrag = (e: DragEvent) => e.dataTransfer.types.includes(MIME);
/** A tab moving out of a pane, during dragover too. */
export const isTabMove = (e: DragEvent) => e.dataTransfer.types.includes(MOVE);
export function readTabDrag(e: DragEvent): TabDrag | null {
  const raw = e.dataTransfer.getData(MIME);
  return raw ? (JSON.parse(raw) as TabDrag) : null;
}

export type Zone = 'center' | 'left' | 'right' | 'up' | 'down';

/** The outer quarter of each side is that side's zone; the rest is the center. */
export function zoneAt(e: DragEvent, el: HTMLElement): Zone {
  const r = el.getBoundingClientRect();
  const d = {
    left: (e.clientX - r.left) / r.width,
    right: (r.right - e.clientX) / r.width,
    up: (e.clientY - r.top) / r.height,
    down: (r.bottom - e.clientY) / r.height,
  };
  const [side, dist] = Object.entries(d).sort((x, y) => x[1] - y[1])[0];
  return dist < 0.25 ? (side as Zone) : 'center';
}

/** The index a drop at the pointer takes among the tabs in `bar` ([role=tab] children), and the x of the insert line within the bar. */
export function slotAt(e: DragEvent, bar: HTMLElement): { index: number; x: number } {
  const from = bar.getBoundingClientRect().left;
  const tabs = [...bar.querySelectorAll<HTMLElement>('[role=tab]')].map((el) => el.getBoundingClientRect());
  const i = tabs.findIndex((r) => e.clientX < r.left + r.width / 2);
  if (i >= 0) return { index: i, x: tabs[i].left - from - 2 };
  const last = tabs[tabs.length - 1];
  return { index: tabs.length, x: (last ? last.right : from) - from + 2 };
}

/** Where on screen a drop in `zone` of `pane` lands. */
function zoneRect(pane: DOMRect, zone: Zone) {
  const { left, top, width, height } = pane;
  if (zone === 'left') return { left, top, width: width / 2, height };
  if (zone === 'right') return { left: left + width / 2, top, width: width / 2, height };
  if (zone === 'up') return { left, top, width, height: height / 2 };
  if (zone === 'down') return { left, top: top + height / 2, width, height: height / 2 };
  return { left, top, width, height };
}

type Box = { left: number; top: number; width: number; height: number };
/** One highlight for the whole screen, so it glides from zone to zone and pane to pane instead of jumping. */
let dropBox = { box: { left: 0, top: 0, width: 0, height: 0 }, on: false, glide: false };
const dropMoved = listeners();
/** Shows where a dragged tab would land on `pane` (its body element). Leaving a place keeps the highlight where it is: crossing the
 *  gap between two panes or bars it waits there and glides on; it goes when the drag leaves the drop area or ends (ZoneHighlight). */
export function showZone(pane: HTMLElement, zone: Zone) {
  place(zoneRect(pane.getBoundingClientRect(), zone));
}
/** Shows the insert line `x` px from the left of tab bar `bar`: the same highlight, narrowed to a line. */
export function showSlot(bar: HTMLElement, x: number) {
  const r = bar.getBoundingClientRect();
  place({ left: r.left + x - 1, top: r.top + 8, width: 3, height: r.height - 16 });
}
function place(box: Box) {
  // Shown again, it appears where it is; only a move while shown glides.
  dropBox = { box, on: true, glide: dropBox.on };
  dropMoved.emit();
}
function hide() {
  if (!dropBox.on) return;
  dropBox = { ...dropBox, on: false };
  dropMoved.emit();
}
/** Marks the element that holds every drop place (the workspace): over anything else, the highlight goes. */
export const DROP_AREA = 'data-drop-area';

/** The highlight of where a dragged tab would land; mounted once by the app. */
export function ZoneHighlight() {
  const { box, on, glide } = useSyncExternalStore(dropMoved.subscribe, () => dropBox);
  useEffect(() => {
    const over = (e: globalThis.DragEvent) => {
      if (!(e.target instanceof Element && e.target.closest(`[${DROP_AREA}]`))) hide();
    };
    addEventListener('dragover', over);
    addEventListener('dragend', hide);
    addEventListener('drop', hide);
    return () => {
      removeEventListener('dragover', over);
      removeEventListener('dragend', hide);
      removeEventListener('drop', hide);
    };
  }, []);
  return (
    <motion.div
      aria-hidden
      initial={false}
      animate={{ ...box, opacity: on ? 1 : 0 }}
      transition={{ ...(glide ? spring : { duration: 0 }), opacity: { duration: 0.1 } }}
      className="pointer-events-none fixed z-40 rounded-md border-2 border-accent bg-accent/10"
    />
  );
}
