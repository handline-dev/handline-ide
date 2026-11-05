// The picture on the viewer's scrim: fit to the room; the wheel, a trackpad pinch or two fingers zoom toward the pointer; a double
// click goes between fit and twice fit; a drag pans once zoomed. While marking, a drag draws a numbered mark and a tap pins one
// (app-core marks.ts); a drag with Space held, or with the middle button, still pans. Gestures come from @use-gesture.
import { MARK_COLOR, marksToShapes, shapesToSvg, type Mark, type MarksAction, type MarksState } from '@handline/app-core';
import { useGesture } from '@use-gesture/react';
import { X } from 'lucide-react';
import {
  useEffect,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type Dispatch,
  type ReactNode,
  type RefObject,
} from 'react';

type View = { s: number; x: number; y: number };
const MAX_ZOOM = 8;
/** A small picture is drawn at most this much larger to fit. */
const FILL = 4;
const NOTE_W = 256;

export interface StageControls {
  fit: () => void;
  zoomBy: (k: number) => void;
}

export interface StageProps {
  url: string;
  name: string;
  marking: boolean;
  marks: MarksState;
  /** Marks on the pictures before this one: its numbers continue from them. */
  from?: number;
  dispatch: Dispatch<MarksAction>;
  image: RefObject<HTMLImageElement | null>;
  /** The zoom against fit (1 is fit), as it changes. */
  onZoom: (k: number) => void;
  controls: RefObject<StageControls | null>;
  /** A tap on the scrim beside the picture. */
  onScrim: () => void;
}

/** Whether Space is held (not while typing): while marking, a drag with it pans. */
function useSpaceHeld() {
  const [held, setHeld] = useState(false);
  useEffect(() => {
    const typing = (e: KeyboardEvent) => e.target instanceof Element && !!e.target.closest('input, textarea');
    const down = (e: KeyboardEvent) => {
      if (e.code !== 'Space' || typing(e)) return;
      e.preventDefault();
      setHeld(true);
    };
    const up = (e: KeyboardEvent) => e.code === 'Space' && setHeld(false);
    const blur = () => setHeld(false);
    addEventListener('keydown', down);
    addEventListener('keyup', up);
    addEventListener('blur', blur);
    return () => {
      removeEventListener('keydown', down);
      removeEventListener('keyup', up);
      removeEventListener('blur', blur);
    };
  }, []);
  return held;
}

const Badge = ({ n }: { n: number }) => (
  <span className="grid size-4 flex-none place-items-center rounded-full bg-danger text-xs leading-none font-semibold text-accent-fg">
    {n}
  </span>
);

/** The open mark's note: its number, the words, and remove. Enter or Escape closes it. */
function NoteBox({ n, mark, dispatch, style }: { n: number; mark: Mark; dispatch: Dispatch<MarksAction>; style: CSSProperties }) {
  return (
    <div
      data-note
      className="absolute z-10 flex w-64 items-start gap-2 rounded-md border border-danger bg-bg p-2 text-fg shadow-popover"
      style={style}
    >
      <Badge n={n} />
      <textarea
        autoFocus
        rows={2}
        aria-label="Comment"
        placeholder="What should change here?"
        className="min-w-0 flex-1 resize-none bg-transparent text-sm text-fg outline-none placeholder:text-muted"
        value={mark.text}
        onChange={(e) => dispatch({ type: 'memo', id: mark.id, text: e.target.value })}
        onKeyDown={(e) => {
          if ((e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) || e.key === 'Escape') {
            e.preventDefault();
            e.stopPropagation();
            dispatch({ type: 'open', id: null });
          }
        }}
      />
      <button
        type="button"
        aria-label="Remove mark"
        title="Remove mark"
        className="grid size-5 flex-none place-items-center text-muted outline-none hover:text-fg"
        onClick={() => dispatch({ type: 'remove', id: mark.id })}
      >
        <X size={14} aria-hidden />
      </button>
    </div>
  );
}

export function Stage({ url, name, marking, marks, from = 0, dispatch, image, onZoom, controls, onScrim }: StageProps): ReactNode {
  const stage = useRef<HTMLDivElement>(null);
  const [room, setRoom] = useState({ w: 1, h: 1 });
  const [nat, setNat] = useState({ w: 0, h: 0 });
  const [view, setView] = useState<View | null>(null); // null is fit
  const space = useSpaceHeld();
  const drawing = useRef(false);

  useLayoutEffect(() => {
    const el = stage.current;
    if (!el) return;
    const read = () => setRoom({ w: el.clientWidth || 1, h: el.clientHeight || 1 });
    read();
    const ro = new ResizeObserver(read);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const fit = nat.w ? Math.min(room.w / nat.w, room.h / nat.h, FILL) : 1;
  const at: View = view ?? { s: fit, x: (room.w - nat.w * fit) / 2, y: (room.h - nat.h * fit) / 2 };
  const zoomed = !!view && view.s > fit * 1.001;
  useEffect(() => onZoom(at.s / fit), [at.s, fit, onZoom]);

  /** A view kept on the stage: centred on an axis where the picture is smaller than the room, else no edge pulled in. */
  const keep = (v: View): View | null => {
    const s = Math.min(MAX_ZOOM, Math.max(fit, v.s));
    if (s <= fit * 1.001) return null;
    const axis = (pos: number, size: number, box: number) => (size <= box ? (box - size) / 2 : Math.min(0, Math.max(box - size, pos)));
    return { s, x: axis(v.x, nat.w * s, room.w), y: axis(v.y, nat.h * s, room.h) };
  };
  const zoomTo = (s: number, px: number, py: number) =>
    setView(keep({ s, x: px - (px - at.x) * (s / at.s), y: py - (py - at.y) * (s / at.s) }));
  const local = (cx: number, cy: number) => {
    const r = stage.current!.getBoundingClientRect();
    return { x: cx - r.left, y: cy - r.top };
  };
  const natural = (cx: number, cy: number) => {
    const p = local(cx, cy);
    return { x: (p.x - at.x) / at.s, y: (p.y - at.y) / at.s };
  };
  useImperativeHandle(controls, () => ({ fit: () => setView(null), zoomBy: (k) => zoomTo(at.s * k, room.w / 2, room.h / 2) }));

  const onPhoto = (t: EventTarget | null) => t instanceof Element && !!t.closest('[data-photo]');
  const onNote = (t: EventTarget | null) => t instanceof Element && !!t.closest('[data-note]');

  useGesture(
    {
      onDrag: ({ first, last, tap, delta: [dx, dy], xy: [cx, cy], event, cancel, pinching }) => {
        if (pinching) return cancel();
        const e = event as PointerEvent;
        if (first) {
          if (onNote(e.target)) return cancel();
          drawing.current = marking && !space && e.button === 0 && onPhoto(e.target);
          if (drawing.current) {
            dispatch({ type: 'scale', slop: Math.max(4, 6 / at.s), radius: Math.max(12, 14 / at.s) });
            dispatch({ type: 'open', id: null });
            dispatch({ type: 'down', ...natural(cx, cy) });
          }
        }
        if (drawing.current) {
          dispatch({ type: last ? 'up' : 'move', ...natural(cx, cy) });
          if (last) drawing.current = false;
          return;
        }
        if (last && tap && !onPhoto(e.target)) return onScrim();
        if (zoomed) setView(keep({ ...at, x: at.x + dx, y: at.y + dy }));
      },
      onPinch: ({ origin: [ox, oy], offset: [s], first, memo }) => {
        if (first && drawing.current) {
          dispatch({ type: 'cancel' });
          drawing.current = false;
        }
        const p = local(ox, oy);
        zoomTo(s, p.x, p.y);
        return memo;
      },
      // A mouse wheel zooms; a trackpad pinch arrives as onPinch (ctrl + wheel).
      onWheel: ({ event, delta: [, dy], pinching }) => {
        if (pinching || (event as WheelEvent).ctrlKey) return;
        const p = local((event as WheelEvent).clientX, (event as WheelEvent).clientY);
        zoomTo(at.s * Math.exp(-dy * 0.002), p.x, p.y);
      },
    },
    {
      target: stage,
      eventOptions: { passive: false },
      drag: { filterTaps: true, pointer: { buttons: [1, 4] } },
      pinch: { from: () => [at.s, 0], scaleBounds: { min: fit, max: MAX_ZOOM }, rubberband: false },
      wheel: { preventDefault: true },
    },
  );

  const line = Math.max(3, Math.round(nat.w / 250));
  const draw = shapesToSvg(marksToShapes(marks.marks, line, true, from), nat.w || 1, nat.h || 1);
  const path = marks.draft?.map((p) => `${p.x},${p.y}`).join(' ');
  const screen = (x: number, y: number) => ({ left: at.x + x * at.s, top: at.y + y * at.s });
  const current = marks.marks.find((m) => m.id === marks.open);
  // The note sits beside the mark's far corner, flipped left when the room is there, and kept inside the stage.
  const corner = current ? screen(current.rect.x + current.rect.w, current.rect.y + current.rect.h) : { left: 0, top: 0 };
  const noteLeft = Math.max(
    8,
    Math.min(room.w - NOTE_W - 8, corner.left + NOTE_W + 16 > room.w ? corner.left - NOTE_W - 8 : corner.left + 8),
  );
  const noteTop = Math.max(8, Math.min(room.h - 120, corner.top + 8));
  const cursor = space || !marking ? (zoomed ? 'cursor-grab active:cursor-grabbing' : '') : 'cursor-crosshair';

  return (
    <div
      ref={stage}
      className={`relative min-h-0 flex-1 touch-none overflow-hidden select-none ${cursor}`}
      onDoubleClick={(e) => {
        if (marking || !onPhoto(e.target)) return;
        const p = local(e.clientX, e.clientY);
        if (zoomed) setView(null);
        else zoomTo(fit * 2, p.x, p.y);
      }}
    >
      <div
        data-photo
        // Hidden until its size is known, so it is never drawn at the corner before it is placed; it eases only between zooms.
        className={`absolute top-0 left-0 origin-top-left ${nat.w ? '' : 'invisible'} ${view && !marks.draft ? 'transition-transform duration-75 ease-out' : ''}`}
        style={{ width: nat.w || undefined, height: nat.h || undefined, transform: `translate(${at.x}px, ${at.y}px) scale(${at.s})` }}
      >
        <img
          ref={image}
          src={url}
          alt={name}
          draggable={false}
          className="block h-full w-full max-w-none"
          onLoad={(e) => setNat({ w: e.currentTarget.naturalWidth, h: e.currentTarget.naturalHeight })}
        />
        {(marking || marks.marks.length > 0) && (
          <svg className="absolute inset-0 h-full w-full" viewBox={draw.viewBox} aria-hidden>
            {draw.elements.map((el, i) =>
              el.type === 'path' ? (
                <path
                  key={i}
                  d={el.d}
                  fill="none"
                  stroke={el.stroke}
                  strokeWidth={el.strokeWidth}
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
              ) : null,
            )}
            {path && (
              <polyline points={path} fill="none" stroke={MARK_COLOR} strokeWidth={line} strokeDasharray={`${line * 2} ${line * 2}`} />
            )}
          </svg>
        )}
      </div>
      {marks.marks.map((m, i) => (
        <button
          key={m.id}
          type="button"
          data-note
          aria-label={`Mark ${from + i + 1}${m.text.trim() ? `: ${m.text.trim()}` : ''}`}
          className="absolute grid size-6 -translate-x-1/2 -translate-y-1/2 place-items-center rounded-full bg-danger text-xs font-semibold text-accent-fg outline-none focus-visible:ring-2 focus-visible:ring-accent"
          style={screen(m.rect.x, m.rect.y)}
          onClick={() => dispatch({ type: 'open', id: m.id })}
        >
          {from + i + 1}
        </button>
      ))}
      {marks.marks.map((m, i) =>
        m.text.trim() && m.id !== marks.open ? (
          <button
            key={m.id}
            type="button"
            data-note
            tabIndex={-1}
            aria-hidden
            className="absolute flex max-w-56 items-center gap-1.5 truncate rounded-md bg-bg px-2 py-1 text-xs text-fg shadow-popover"
            style={{ ...screen(m.rect.x + m.rect.w, m.rect.y), marginLeft: 10 }}
            onClick={() => dispatch({ type: 'open', id: m.id })}
          >
            <Badge n={from + i + 1} />
            <span className="truncate">{m.text.trim()}</span>
          </button>
        ) : null,
      )}
      {current && (
        <NoteBox
          key={current.id}
          n={from + marks.marks.findIndex((m) => m.id === current.id) + 1}
          mark={current}
          dispatch={dispatch}
          style={{ left: noteLeft, top: noteTop }}
        />
      )}
    </div>
  );
}
