import { useAttachmentUrl } from '@handline/app-data';
import { Play } from 'lucide-react';
import { createContext, useContext, useState } from 'react';
import type { Attachments } from '#src/chat/attachments.tsx';
import { playVideo } from '#src/viewer/VideoPlayer.tsx';
import { ImageViewer, type MarkedPicture, type Picture } from '#src/viewer/ImageViewer.tsx';

/** Where a picture marked up from a chat or a task goes: the box below, its marked copy attached and its comments written in. */
export const MarkReply = createContext<{ add: (marked: MarkedPicture[]) => void; label: string } | null>(null);

/** Marked pictures into a box: each marked copy attached as `name-marked.png`, the comments after the draft. */
export function markInto(attach: Attachments, setDraft: (f: (d: string) => string) => void) {
  return (marked: MarkedPicture[]) => {
    if (!marked.length) return;
    attach.add(
      marked.map((m) => {
        const name = `${m.picture.name.replace(/\.[^.]+$/, '')}-marked.png`;
        return { kind: 'file' as const, name, files: [{ path: name, file: new File([m.image], name, { type: 'image/png' }) }] };
      }),
    );
    setDraft((d) => [d.trimEnd(), ...marked.map((m) => m.text)].filter(Boolean).join('\n\n'));
  };
}

/** A message's pictures as thumbnails; a click opens them in the viewer, where they can be marked up into the box below. `small`
 * halves them, for a result whose pictures are shown larger beside it. */
export function MessageImages({
  images,
  small = false,
}: {
  images: { id: string; name: string; video?: { id: string; name: string } }[];
  small?: boolean;
}) {
  const [open, setOpen] = useState<number | null>(null);
  const reply = useContext(MarkReply);
  if (!images.length) return null;
  const pictures: Picture[] = images.map((a) => ({ id: a.id, name: a.name }));
  return (
    <div className="flex max-w-150 flex-wrap gap-2">
      {images.map((a, i) => (
        <Thumb
          key={a.id}
          id={a.id}
          name={a.name}
          play={!!a.video}
          small={small}
          onOpen={() => (a.video ? playVideo(a.video.id, a.video.name) : setOpen(i))}
        />
      ))}
      {open !== null && (
        <ImageViewer pictures={pictures} start={open} onClose={() => setOpen(null)} onMarked={reply?.add} doneLabel={reply?.label} />
      )}
    </div>
  );
}

function Thumb({ id, name, play, small, onOpen }: { id: string; name: string; play?: boolean; small: boolean; onOpen: () => void }) {
  const { url, failed, removed, gone } = useAttachmentUrl(id, true);
  return (
    <button
      type="button"
      onClick={onOpen}
      title={name}
      className={`relative ${url ? '' : small ? 'w-20' : 'w-40'} ${small ? 'h-20' : 'h-40'} overflow-hidden rounded-lg border border-border bg-surface outline-none focus-visible:ring-2 focus-visible:ring-accent`}
    >
      {/* The picture's own shape at one height, its width kept between 80 and 320: a long one shows its top. */}
      {url ? (
        <img src={url} alt={name} className="h-full w-auto min-w-20 max-w-80 object-cover object-top" />
      ) : (
        <span className="grid size-full place-items-center text-xs text-muted">
          {failed ? 'Did not load' : removed ? 'Deleted' : gone ? 'Not kept' : ''}
        </span>
      )}
      {play && (
        <span className="absolute inset-0 m-auto flex size-8 items-center justify-center rounded-full bg-fg/60 text-bg">
          <Play size={14} />
        </span>
      )}
    </button>
  );
}
