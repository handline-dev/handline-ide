// The one picture viewer of the web, for looking and for marking up: full screen on a dark scrim, zoom and pan (Stage.tsx), the
// pictures stepped with the arrows. Where the caller takes comments (`onMarked`), the pen turns a drag into a numbered mark with a
// comment at it; Done hands each marked picture back as its marks and the picture, for the caller to keep until its message goes
// (chat/marked.tsx draws them in then). Cmd+Enter does the same and asks the caller to send at once.
import {
  initialMarks,
  keptMarks,
  letterOf,
  marksReducer,
  seedMarks,
  type Mark,
  type MarksAction,
  type MarksState,
} from '@handline/app-core';
import { useAttachmentUrl } from '@handline/app-data';
import { ChevronLeft, ChevronRight, Copy, Download, Maximize, Minus, PenLine, Plus, Undo2, X } from 'lucide-react';
import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { count } from '#src/format/count.ts';
import { useWindowLayer } from '#src/layout/windowLayer.tsx';
import { copy } from '#src/ui/copy.ts';
import { toast } from '#src/ui/toast.tsx';
import { bake } from '#src/viewer/bake.ts';
import { Stage, type StageControls } from '#src/viewer/Stage.tsx';

/** A picture to show: a daemon attachment by `id`, or any `url` (a blob, a capture). */
export type Picture = { id?: string; url?: string; name: string };

/** A picture as marked: which of the viewer's pictures (`index`), its marks with words, and the picture itself as a PNG. */
export type MarkedPicture = { picture: Picture; index: number; marks: Mark[]; source: Blob };

export function ImageViewer({
  pictures,
  start = 0,
  onClose,
  onMarked,
  doneLabel = 'Add to message',
  sendBare = false,
  seed,
  from = 0,
  pen: penAtOpen = false,
  canSend = false,
}: {
  pictures: Picture[];
  start?: number;
  onClose: () => void;
  /** Takes the marked pictures on Done (`send` on Cmd+Enter); without it the viewer only shows. */
  onMarked?: (marked: MarkedPicture[], send: boolean) => void;
  doneLabel?: string;
  /** The done button shows with no mark too, and hands the pictures over as they are (a screenshot sent to the chat). */
  sendBare?: boolean;
  /** Marks a picture already carries, to change: a seeded picture comes back on Done even with none left. */
  seed?: (Mark[] | undefined)[];
  /** Marks before these pictures in the message: the numbers continue from them. */
  from?: number;
  /** The pen is on as the viewer opens (a picture about to be sent). */
  pen?: boolean;
  /** Cmd+Enter sends the message; without it Cmd+Enter is Done. */
  canSend?: boolean;
}) {
  const [at, setAt] = useState(Math.min(start, pictures.length - 1));
  const pic = pictures[at];
  const full = useAttachmentUrl(pic.id ?? '', false);
  const url = pic.url ?? full.url;
  const canMark = !!onMarked;
  const [pen, setMarking] = useState(penAtOpen && canMark);
  // The pen waits for the picture: stepping to one still loading lays it down until it is here.
  const marking = pen && !!url;
  const [states, setStates] = useState<Record<number, MarksState>>({});
  const stateOf = (all: Record<number, MarksState>, i: number) => all[i] ?? (seed?.[i] ? seedMarks(seed[i]) : initialMarks());
  const marks = stateOf(states, at);
  const dispatch = useCallback(
    (a: MarksAction) => setStates((all) => ({ ...all, [at]: marksReducer(stateOf(all, at), a) })),
    // stateOf reads only the seed, fixed while the viewer is open
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [at],
  );
  const [zoom, setZoom] = useState(1);
  const image = useRef<HTMLImageElement>(null);
  const images = useRef<Record<number, HTMLImageElement>>({});
  const controls = useRef<StageControls>(null);
  const root = useRef<HTMLDivElement>(null);
  const keptAt = (i: number) => keptMarks(stateOf(states, i).marks).length;
  // Numbers run on across the pictures: this one's start after the kept marks of those before it.
  const fromOf = (i: number) => from + pictures.slice(0, i).reduce((n, _, j) => n + keptAt(j), 0);
  const kept = pictures.reduce((n, _, i) => n + keptAt(i), 0);

  useEffect(() => {
    if (image.current) images.current[at] = image.current;
  });

  const done = async (send: boolean) => {
    const out: MarkedPicture[] = [];
    for (const [i, p] of pictures.entries()) {
      const m = keptMarks(stateOf(states, i).marks);
      const img = images.current[i];
      // A seeded picture not opened is unchanged; one opened comes back even with its marks all gone.
      if (!(m.length || (seed?.[i] && states[i])) || !img) continue;
      const source = await bake(img, [], 0);
      if (source) out.push({ picture: p, index: i, marks: m, source });
    }
    // Nothing marked: each picture as it is, for a caller that takes them so (sendBare).
    if (!out.length && sendBare)
      for (const [i, p] of pictures.entries()) {
        const img = images.current[i];
        const source = img && (await bake(img, [], 0));
        if (source) out.push({ picture: p, index: i, marks: [], source });
      }
    onMarked?.(out, send);
    onClose();
  };

  const step = (by: number) => setAt((a) => Math.min(pictures.length - 1, Math.max(0, a + by)));
  const live = useRef({ step, onClose, dispatch, canMark, marks, url, done, canSend });
  useEffect(() => void (live.current = { step, onClose, dispatch, canMark, marks, url, done, canSend }));
  useEffect(() => {
    const before = document.activeElement as HTMLElement | null;
    root.current?.focus();
    const key = (e: KeyboardEvent) => {
      const { step, onClose, dispatch, canMark, marks, url, done, canSend } = live.current;
      // Cmd+Enter, in a note too: the marks go into the message, and it is sent where the caller can.
      if ((e.metaKey || e.ctrlKey) && e.key === 'Enter' && canMark && !e.isComposing) {
        e.preventDefault();
        return void done(canSend);
      }
      if (e.target instanceof Element && e.target.closest('input, textarea')) return;
      // Letters by the key pressed (letterOf): with a Korean input on, M types another letter.
      if ((e.metaKey || e.ctrlKey) && letterOf(e) === 'z') {
        e.preventDefault();
        dispatch({ type: 'undo' });
      } else if (e.key === 'Escape') {
        if (marks.open !== null) dispatch({ type: 'open', id: null });
        else onClose();
      } else if (e.key === 'ArrowLeft') step(-1);
      else if (e.key === 'ArrowRight') step(1);
      else if (letterOf(e) === 'm' && !e.metaKey && !e.ctrlKey && canMark && url) setMarking((v) => !v);
      else if (!url) return;
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

  const copyPicture = async () => {
    if (!url) return;
    const blob = await fetch(url).then(
      (r) => r.blob(),
      () => null,
    );
    if (blob) void copy(blob, 'the picture', { say: true });
    else toast('Could not copy the picture.', { tone: 'warning' });
  };

  useWindowLayer(true);
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
        {/* The tools stay in their places while the picture loads, dimmed until it is here. */}
        <Tool label="Zoom out (-)" disabled={!url} onClick={() => controls.current?.zoomBy(0.8)}>
          <Minus size={16} />
        </Tool>
        <button
          type="button"
          title="Fit (0)"
          disabled={!url}
          onClick={() => controls.current?.fit()}
          className="h-8 w-14 rounded-md text-xs text-muted tabular-nums outline-none hover:bg-hover focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-20"
        >
          {`${Math.round(zoom * 100)}%`}
        </button>
        <Tool label="Zoom in (+)" disabled={!url} onClick={() => controls.current?.zoomBy(1.25)}>
          <Plus size={16} />
        </Tool>
        <Tool label="Fit (0)" disabled={!url} onClick={() => controls.current?.fit()}>
          <Maximize size={15} />
        </Tool>
        <span className="mx-1 h-5 w-px bg-border" />
        {canMark && (
          <>
            <Tool label="Mark (M)" on={marking} disabled={!url} onClick={() => setMarking((v) => !v)}>
              <PenLine size={16} />
            </Tool>
            <Tool label="Undo (⌘Z)" disabled={!marks.past.length} onClick={() => dispatch({ type: 'undo' })}>
              <Undo2 size={16} />
            </Tool>
          </>
        )}
        <Tool label="Copy" disabled={!url} onClick={() => void copyPicture()}>
          <Copy size={15} />
        </Tool>
        {url ? (
          <a
            href={url}
            download={pic.name}
            title="Save"
            aria-label="Save"
            className="grid size-8 place-items-center rounded-md text-muted outline-none hover:bg-hover hover:text-fg focus-visible:ring-2 focus-visible:ring-accent"
          >
            <Download size={15} />
          </a>
        ) : (
          <Tool label="Save" disabled>
            <Download size={15} />
          </Tool>
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
        {canMark && (kept > 0 || sendBare || (seed && Object.keys(states).length > 0)) && (
          <>
            {kept > 0 && <span className="tabular-nums">{count(kept, 'comment')}</span>}
            <button
              type="button"
              title={canSend ? `${doneLabel} (⌘Enter sends)` : `${doneLabel} (⌘Enter)`}
              onClick={() => void done(false)}
              className="h-8 rounded-md bg-accent px-3 text-sm text-accent-fg outline-none hover:opacity-90 focus-visible:ring-2 focus-visible:ring-fg"
            >
              {doneLabel}
            </button>
          </>
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
  onClick?: () => void;
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
      className="grid size-8 place-items-center rounded-md text-muted outline-none hover:bg-hover hover:text-fg focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-20 aria-pressed:bg-selected aria-pressed:text-fg"
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
