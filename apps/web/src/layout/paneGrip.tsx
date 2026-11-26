// A tab bar's handle, the dots at the top of its middle as on a tablet: dragged, the whole tab bar goes where it is let go, onto another
// pane (its middle joins the tabs, an edge splits there) or onto a place that takes tab bars (the town's scene). Pointer-driven, so a
// floating tab bar can follow the pointer itself; else its shown tab's title does.
import { useEffect, useRef, type KeyboardEvent, type PointerEvent } from 'react';
import { hideZone, showZone, zoneAt, type Zone } from '#src/ui/dnd.tsx';

/** On a pane's body: its id. */
export const PANE_BODY = 'data-pane-body';
/** On another place that takes a tab bar: its name. */
export const PANE_PLACE = 'data-pane-place';

export type PaneDrop = { pane: string; zone: Zone } | { place: string; zone: Zone };
export type At = { x: number; y: number };

/** What takes a tab bar let go at (x, y). */
export function dropAt({ x, y }: At): { el: HTMLElement; drop: PaneDrop } | null {
  const el = document.elementFromPoint(x, y)?.closest<HTMLElement>(`[${PANE_BODY}],[${PANE_PLACE}]`);
  if (!el) return null;
  const zone = zoneAt({ clientX: x, clientY: y }, el);
  const pane = el.getAttribute(PANE_BODY);
  return { el, drop: pane ? { pane, zone } : { place: el.getAttribute(PANE_PLACE) ?? '', zone } };
}

/** How far the pointer goes before a press is a drag. */
const SLOP = 4;

export type GripProps = {
  /** The shown tab's title, carried under the pointer. */
  label: string;
  onDrop: (drop: PaneDrop | null, at: At) => void;
  /** Called on each move with the offset since the press, and with null when let go: the tab bar then follows the pointer itself. */
  onMove?: (offset: At | null) => void;
  /** Arrow keys on the focused handle: where it goes without a pointer. */
  onKey?: (key: 'ArrowLeft' | 'ArrowRight' | 'ArrowUp') => void;
  /** Said by the handle, for the keys it takes. */
  hint?: string;
};

/** The shown tab's title under the pointer while a docked tab bar is carried: a plain element, as the tab drag's own picture is. */
function chipAt(chip: HTMLElement | null, text: string, p: At): HTMLElement {
  const el = chip ?? document.createElement('div');
  if (!chip) {
    el.textContent = text;
    el.className = 'pointer-events-none fixed z-50 rounded-md bg-accent px-2.5 py-1 text-xs text-accent-fg shadow-md';
    document.body.append(el);
  }
  el.style.left = `${p.x + 10}px`;
  el.style.top = `${p.y + 10}px`;
  return el;
}

export function PaneGrip({ label, onDrop, onMove, onKey, hint = 'Drag to move this tab bar' }: GripProps) {
  const start = useRef<At | null>(null);
  const moved = useRef(false);
  const chip = useRef<HTMLElement | null>(null);
  // A tab bar that goes while it is carried takes its label with it.
  useEffect(
    () => () => {
      if (!chip.current) return;
      chip.current.remove();
      hideZone();
    },
    [],
  );
  const over = (p: At) => {
    const hit = dropAt(p);
    if (hit && !(hit.drop.zone === 'center' && 'place' in hit.drop)) showZone(hit.el, hit.drop.zone);
    else hideZone();
  };
  const end = (p: At | null) => {
    const dragged = moved.current;
    // Where it lands is read while the carried bar still lets the pointer through.
    const drop = dragged && p ? (dropAt(p)?.drop ?? null) : null;
    start.current = null;
    moved.current = false;
    chip.current?.remove();
    chip.current = null;
    onMove?.(null);
    hideZone();
    if (dragged && p) onDrop(drop, p);
  };
  return (
    <button
      type="button"
      aria-label={hint}
      title={hint}
      className="app-no-drag absolute top-0 left-1/2 z-10 flex h-3 w-10 -translate-x-1/2 cursor-grab touch-none items-center justify-center gap-0.5 rounded-b-md outline-none hover:bg-hover focus-visible:bg-hover active:cursor-grabbing"
      onPointerDown={(e: PointerEvent) => {
        e.preventDefault();
        e.currentTarget.setPointerCapture(e.pointerId);
        start.current = { x: e.clientX, y: e.clientY };
      }}
      onPointerMove={(e) => {
        const s = start.current;
        if (!s) return;
        const p = { x: e.clientX, y: e.clientY };
        if (!moved.current && Math.hypot(p.x - s.x, p.y - s.y) < SLOP) return;
        moved.current = true;
        if (onMove) onMove({ x: p.x - s.x, y: p.y - s.y });
        else chip.current = chipAt(chip.current, label, p);
        over(p);
      }}
      onPointerUp={(e) => end({ x: e.clientX, y: e.clientY })}
      onLostPointerCapture={() => start.current && end(null)}
      onKeyDown={(e: KeyboardEvent) => {
        if (!onKey || !['ArrowLeft', 'ArrowRight', 'ArrowUp'].includes(e.key)) return;
        e.preventDefault();
        onKey(e.key as 'ArrowLeft');
      }}
    >
      {[0, 1, 2].map((i) => (
        <span key={i} className="size-1 rounded-full bg-muted" />
      ))}
    </button>
  );
}
