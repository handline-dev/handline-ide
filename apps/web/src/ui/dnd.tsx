/** Drag and drop into the workspace: one payload format, and the geometry of where a drop lands on a pane
 * or a tab bar. Anything that can become a tab is a source: a tab moving from its pane, or a thing to open
 * (an agent in the sidebar, say). */
import type { DragEvent } from 'react';

const MIME = 'application/x-handline-tab';
export type TabDrag = { move: { pane: string; tab: string } } | { open: { opens: string; title: string } };

export function startTabDrag(e: DragEvent, drag: TabDrag) {
  e.dataTransfer.setData(MIME, JSON.stringify(drag));
  e.dataTransfer.effectAllowed = 'move' in drag ? 'move' : 'copy';
}
/** True during dragover too, when the payload itself cannot be read yet. */
export const isTabDrag = (e: DragEvent) => e.dataTransfer.types.includes(MIME);
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

const zoneArea: Record<Zone, string> = {
  center: 'inset-0',
  left: 'inset-y-0 left-0 w-1/2',
  right: 'inset-y-0 right-0 w-1/2',
  up: 'inset-x-0 top-0 h-1/2',
  down: 'inset-x-0 bottom-0 h-1/2',
};

/** The highlight of where a dragged tab would land on a pane. */
export function ZoneHighlight({ zone }: { zone: Zone }) {
  return (
    <div
      className={`pointer-events-none absolute z-10 rounded-md border-2 border-accent bg-accent/10 transition-all duration-100 ${zoneArea[zone]}`}
    />
  );
}

/** The insert line in a tab bar, `x` px from its left. */
export function InsertLine({ x }: { x: number }) {
  return <div style={{ left: x }} className="pointer-events-none absolute inset-y-2 z-10 w-0.5 rounded-full bg-accent" />;
}
