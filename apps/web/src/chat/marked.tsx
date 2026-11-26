// Pictures marked up for the next message, kept as their marks until it goes: the chip shows the picture and a count, a click
// reopens it to change the marks, and the send draws them in and writes their lines, numbered across the message's pictures
// (app-core markedText). Held in memory per box, like a picture still going up.
import { keptMarks, markStarts, markedText, type Mark } from '@handline/app-core';
import { keptState, useUploadImage } from '@handline/app-data';
import { X } from 'lucide-react';
import { useEffect, useRef, type RefObject } from 'react';
import type { MarkedPicture } from '#src/viewer/ImageViewer.tsx';
import { bake } from '#src/viewer/bake.ts';

export type MarkedDraft = { key: number; name: string; source: Blob; url: string; marks: Mark[] };

let nextKey = 1;
const useHeld = keptState<{ list: MarkedDraft[] }>(() => ({ list: [] }));
// Each box's open composers, newest last: an ask to send goes to the newest only, so a chat open in two panes sends once.
const senders = new Map<string, RefObject<() => void>[]>();
const askSend = (box: string) => setTimeout(() => senders.get(box)?.at(-1)?.current());

/** The composer of `box` sends when a viewer asks (Cmd+Enter), with its state as it is then. */
export function useSendOnAsk(box: string, send: () => void) {
  const ref = useRef(send);
  useEffect(() => {
    ref.current = send;
  });
  useEffect(() => {
    senders.set(box, [...(senders.get(box) ?? []), ref]);
    return () =>
      void senders.set(
        box,
        (senders.get(box) ?? []).filter((r) => r !== ref),
      );
  }, [box]);
}

/** A box's marked pictures. `add` with `send` asks the box to send at once (Cmd+Enter in the viewer). */
export function useMarked(box: string) {
  const held = useHeld(box);
  const { list } = held.state;
  const drop = (gone: MarkedDraft[]) => gone.forEach((d) => URL.revokeObjectURL(d.url));
  return {
    box,
    list,
    /** Kept marks across the chips: where a picture marked next starts its numbers. */
    count: list.reduce((n, d) => n + keptMarks(d.marks).length, 0),
    add: (marked: MarkedPicture[], send = false) => {
      held.set((s) => ({
        list: [
          ...s.list,
          ...marked.map((m) => ({
            key: nextKey++,
            name: `${m.picture.name.replace(/\.[^.]+$/, '')}-marked.png`,
            source: m.source,
            url: URL.createObjectURL(m.source),
            marks: m.marks,
          })),
        ],
      }));
      if (send) askSend(box);
    },
    remark: (changes: Map<number, Mark[]>, send = false) => {
      held.set((s) => ({ list: s.list.map((d) => (changes.has(d.key) ? { ...d, marks: changes.get(d.key)! } : d)) }));
      if (send) askSend(box);
    },
    remove: (key: number) =>
      held.set((s) => {
        drop(s.list.filter((d) => d.key === key));
        return { ...s, list: s.list.filter((d) => d.key !== key) };
      }),
    clear: () =>
      held.set((s) => {
        drop(s.list);
        return { ...s, list: [] };
      }),
  };
}

export type Marked = ReturnType<typeof useMarked>;

/** What the send adds for the marked pictures: each drawn in with its numbers and uploaded, in chip order, and the text with their lines. */
export function useBakeMarked() {
  const upload = useUploadImage();
  return async (draft: string, list: MarkedDraft[]): Promise<{ text: string; ids: string[] }> => {
    const starts = markStarts(list.map((d) => d.marks));
    const ids: string[] = [];
    for (const [i, d] of list.entries()) {
      const png = await bake(d.source, keptMarks(d.marks), starts[i]);
      if (!png) throw new Error(`${d.name} could not be drawn`);
      ids.push((await upload(png, d.name, 'image/png')).id);
    }
    return {
      text: markedText(
        draft,
        list.map((d) => d.marks),
      ),
      ids,
    };
  };
}

/** A marked picture with the message: the picture and the numbers its marks go out with (`from` is the marks before it); a click
 * reopens it to change them. */
export function MarkedChip({
  draft,
  from,
  onOpen,
  onRemove,
}: {
  draft: MarkedDraft;
  from: number;
  onOpen: () => void;
  onRemove: () => void;
}) {
  const n = keptMarks(draft.marks).length;
  const numbers = n > 1 ? `${from + 1}–${from + n}` : String(from + 1);
  return (
    <div className="group relative size-16 overflow-hidden rounded-md border border-border" title={draft.name}>
      <button type="button" aria-label={`Edit marks on ${draft.name}`} onClick={onOpen} className="size-full outline-none">
        <img src={draft.url} alt={draft.name} className="size-full object-cover" />
      </button>
      {n > 0 && (
        <span
          aria-label={`Marks ${numbers}`}
          className="pointer-events-none absolute bottom-1 left-1 grid h-4 min-w-4 place-items-center rounded-full bg-accent px-1 text-xs leading-none font-semibold text-accent-fg"
        >
          {numbers}
        </span>
      )}
      <button
        type="button"
        aria-label="Remove"
        onClick={onRemove}
        className="absolute top-1 right-1 grid size-4 place-items-center rounded-full bg-fg text-bg opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
      >
        <X size={12} />
      </button>
    </div>
  );
}
