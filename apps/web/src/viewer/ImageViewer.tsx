// The one picture viewer of the web, for looking and for marking up: full screen on a dark scrim, zoom and pan (Stage.tsx), the
// pictures stepped with the arrows. Where the caller takes comments (`onMarked`), Mark turns a drag into a numbered mark with a comment
// at it; Done hands each marked picture back with the marks drawn in and their words, for the caller to send (a chat reply, a
// capture from a simulator, a task's picture).
import {
  exportAnnotated,
  initialMarks,
  keptMarks,
  markData,
  marksReducer,
  marksToShapes,
  type Bitmap,
  type Mark,
  type MarksAction,
  type MarksState,
} from '@handline/app-core';
import { useAttachmentUrl } from '@handline/app-data';
import { ChevronLeft, ChevronRight, Copy, Download, Maximize, Minus, PenLine, Plus, Undo2, X } from 'lucide-react';
import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { Stage, type StageControls } from '#src/viewer/Stage.tsx';

/** A picture to show: a daemon attachment by `id`, or any `url` (a blob, a capture). */
export type Picture = { id?: string; url?: string; name: string };

/** A picture as marked: the marks with words, the picture with them drawn in, and the lines that say them (`1. words`, numbered
 * across the pictures of one Done) with a `marks` block of their rects for an agent that crops. */
export type MarkedPicture = { picture: Picture; marks: Mark[]; image: Blob; text: string };

/** The picture with the shapes drawn into its pixels, as a PNG. */
async function drawn(img: HTMLImageElement, marks: Mark[], from: number): Promise<Blob | null> {
  const c = document.createElement('canvas');
  c.width = img.naturalWidth;
  c.height = img.naturalHeight;
  const g = c.getContext('2d');
  if (!g) return null;
  g.drawImage(img, 0, 0);
  const src = g.getImageData(0, 0, c.width, c.height);
  const shapes = marksToShapes(marks, Math.max(3, Math.round(c.width / 250)), false, from);
  const out = exportAnnotated({ width: src.width, height: src.height, data: src.data } as Bitmap, shapes);
  g.putImageData(new ImageData(out.data as Uint8ClampedArray<ArrayBuffer>, out.width, out.height), 0, 0);
  return new Promise((ok) => c.toBlob(ok, 'image/png'));
}

const linesOf = (marks: Mark[], from: number) => {
  const data = markData(marks, from);
  return [data.map((d) => `${d.n}. ${d.text}`).join('\n'), '```marks', JSON.stringify(data), '```'].join('\n');
};

export function ImageViewer({
  pictures,
  start = 0,
  onClose,
  onMarked,
  doneLabel = 'Add to message',
}: {
  pictures: Picture[];
  start?: number;
  onClose: () => void;
  /** Takes the marked pictures on Done; without it the viewer only shows. */
  onMarked?: (marked: MarkedPicture[]) => void;
  doneLabel?: string;
}) {
  const [at, setAt] = useState(Math.min(start, pictures.length - 1));
  const pic = pictures[at]!;
  const full = useAttachmentUrl(pic.id ?? '', false);
  const url = pic.url ?? full.url;
  const canMark = !!onMarked;
  const [marking, setMarking] = useState(false);
  const [states, setStates] = useState<Record<number, MarksState>>({});
  const marks = states[at] ?? initialMarks();
  const dispatch = useCallback(
    (a: MarksAction) => setStates((all) => ({ ...all, [at]: marksReducer(all[at] ?? initialMarks(), a) })),
    [at],
  );
  const [zoom, setZoom] = useState(1);
  const image = useRef<HTMLImageElement>(null);
  const images = useRef<Record<number, HTMLImageElement>>({});
  const controls = useRef<StageControls>(null);
  const root = useRef<HTMLDivElement>(null);
  // Numbers run on across the pictures: this one's start after the kept marks of those before it.
  const fromOf = (i: number) => pictures.slice(0, i).reduce((n, _, j) => n + keptMarks(states[j]?.marks ?? []).length, 0);
  const kept = pictures.reduce((n, _, i) => n + keptMarks(states[i]?.marks ?? []).length, 0);

  useEffect(() => {
    if (image.current) images.current[at] = image.current;
  });

  const done = async () => {
    const out: MarkedPicture[] = [];
    for (const [i, p] of pictures.entries()) {
      const m = keptMarks(states[i]?.marks ?? []);
      const img = images.current[i];
      if (!m.length || !img) continue;
      const blob = await drawn(img, states[i]!.marks, fromOf(i));
      if (blob) out.push({ picture: p, marks: m, image: blob, text: linesOf(states[i]!.marks, fromOf(i)) });
    }
    onMarked?.(out);
    onClose();
  };

  const step = (by: number) => setAt((a) => Math.min(pictures.length - 1, Math.max(0, a + by)));
  const live = useRef({ step, onClose, dispatch, canMark, marks });
  useEffect(() => void (live.current = { step, onClose, dispatch, canMark, marks }));
  useEffect(() => {
    const before = document.activeElement as HTMLElement | null;
    root.current?.focus();
    const key = (e: KeyboardEvent) => {
      const { step, onClose, dispatch, canMark, marks } = live.current;
      if (e.target instanceof Element && e.target.closest('input, textarea')) return;
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'z') {
        e.preventDefault();
        dispatch({ type: 'undo' });
      } else if (e.key === 'Escape') {
        if (marks.open !== null) dispatch({ type: 'open', id: null });
        else onClose();
      } else if (e.key === 'ArrowLeft') step(-1);
      else if (e.key === 'ArrowRight') step(1);
      else if (e.key === 'm' && canMark) setMarking((v) => !v);
      else if (e.key === '0') controls.current?.fit();
      else if (e.key === '=' || e.key === '+') controls.current?.zoomBy(1.25);
      else if (e.key === '-') controls.current?.zoomBy(0.8);
    };
    addEventListener('keydown', key);
    return () => {
      removeEventListener('keydown', key);
      before?.focus?.();
    };
  }, []);

  const copy = async () => {
    if (!url) return;
    const blob = await (await fetch(url)).blob();
    await navigator.clipboard.write([new ClipboardItem({ [blob.type]: blob })]);
  };

  return createPortal(
    <div
      ref={root}
      role="dialog"
      aria-modal="true"
      aria-label={pic.name}
      tabIndex={-1}
      className="fixed inset-0 z-50 flex flex-col dark bg-bg text-fg outline-none"
    >
      <div className="flex h-12 flex-none items-center gap-1 px-3 text-sm">
        <span className="min-w-0 truncate px-2 text-muted">{pic.name}</span>
        {pictures.length > 1 && <span className="text-muted tabular-nums">{`${at + 1} / ${pictures.length}`}</span>}
        <span className="flex-1" />
        <Tool label="Zoom out (-)" onClick={() => controls.current?.zoomBy(0.8)}>
          <Minus size={16} />
        </Tool>
        <button
          type="button"
          title="Fit (0)"
          onClick={() => controls.current?.fit()}
          className="h-8 w-14 rounded-md text-xs text-muted tabular-nums outline-none hover:bg-hover focus-visible:ring-2 focus-visible:ring-accent"
        >
          {`${Math.round(zoom * 100)}%`}
        </button>
        <Tool label="Zoom in (+)" onClick={() => controls.current?.zoomBy(1.25)}>
          <Plus size={16} />
        </Tool>
        <Tool label="Fit (0)" onClick={() => controls.current?.fit()}>
          <Maximize size={15} />
        </Tool>
        <span className="mx-1 h-5 w-px bg-border" />
        {canMark && (
          <>
            <Tool label="Mark (M)" on={marking} onClick={() => setMarking((v) => !v)}>
              <PenLine size={16} />
            </Tool>
            <Tool label="Undo (⌘Z)" disabled={!marks.past.length} onClick={() => dispatch({ type: 'undo' })}>
              <Undo2 size={16} />
            </Tool>
          </>
        )}
        {url && (
          <>
            <Tool label="Copy" onClick={() => void copy().catch(() => undefined)}>
              <Copy size={15} />
            </Tool>
            <a
              href={url}
              download={pic.name}
              title="Save"
              aria-label="Save"
              className="grid size-8 place-items-center rounded-md text-muted outline-none hover:bg-hover hover:text-fg focus-visible:ring-2 focus-visible:ring-accent"
            >
              <Download size={15} />
            </a>
          </>
        )}
        <span className="mx-1 h-5 w-px bg-border" />
        <Tool label="Close (Esc)" onClick={onClose}>
          <X size={18} />
        </Tool>
      </div>
      <div className="relative flex min-h-0 flex-1">
        {url ? (
          <Stage
            key={at}
            url={url}
            name={pic.name}
            marking={marking}
            marks={marks}
            from={fromOf(at)}
            dispatch={dispatch}
            image={image}
            onZoom={setZoom}
            controls={controls}
            onScrim={onClose}
          />
        ) : (
          <p className="m-auto text-sm text-muted">
            {full.failed
              ? 'The image did not load.'
              : full.removed
                ? 'Image deleted.'
                : full.gone
                  ? 'This image is no longer kept.'
                  : 'Loading…'}
          </p>
        )}
        {at > 0 && <Side side="left" onClick={() => step(-1)} />}
        {at < pictures.length - 1 && <Side side="right" onClick={() => step(1)} />}
      </div>
      <div className="flex h-14 flex-none items-center justify-center gap-3 px-4 text-sm text-muted">
        {canMark && kept > 0 ? (
          <>
            <span className="tabular-nums">{`${kept} ${kept === 1 ? 'comment' : 'comments'}`}</span>
            <button
              type="button"
              onClick={() => void done()}
              className="h-8 rounded-md bg-accent px-3 text-sm text-accent-fg outline-none hover:opacity-90 focus-visible:ring-2 focus-visible:ring-fg"
            >
              {doneLabel}
            </button>
          </>
        ) : canMark ? (
          marking ? (
            'Click to pin, drag to box, circle to mark an area · Space-drag to pan'
          ) : (
            'Scroll to zoom · Double-click to zoom · Drag to pan · M to mark'
          )
        ) : (
          'Scroll to zoom · Double-click to zoom · Drag to pan'
        )}
      </div>
    </div>,
    document.body,
  );
}

function Tool({
  label,
  on,
  disabled,
  onClick,
  children,
}: {
  label: string;
  on?: boolean;
  disabled?: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      aria-pressed={on}
      disabled={disabled}
      onClick={onClick}
      className="grid size-8 place-items-center rounded-md text-muted outline-none hover:bg-hover hover:text-fg focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-30 aria-pressed:bg-selected aria-pressed:text-fg"
    >
      {children}
    </button>
  );
}

function Side({ side, onClick }: { side: 'left' | 'right'; onClick: () => void }) {
  return (
    <button
      type="button"
      aria-label={side === 'left' ? 'Previous image' : 'Next image'}
      onClick={onClick}
      className={`absolute top-1/2 ${side === 'left' ? 'left-3' : 'right-3'} grid size-10 -translate-y-1/2 place-items-center rounded-full bg-surface text-fg outline-none hover:bg-hover focus-visible:ring-2 focus-visible:ring-accent`}
    >
      {side === 'left' ? <ChevronLeft size={20} /> : <ChevronRight size={20} />}
    </button>
  );
}
