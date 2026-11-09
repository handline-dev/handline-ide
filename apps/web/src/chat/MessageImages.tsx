import { useAttachmentUrl } from '@handline/app-data';
import { createContext, useContext, useState } from 'react';
import { ImageViewer, type MarkedPicture, type Picture } from '#src/viewer/ImageViewer.tsx';

/** Where a picture marked up from the chat goes: the next message, its marked copy attached and its comments written in. */
export const MarkReply = createContext<((marked: MarkedPicture[]) => void) | null>(null);

/** A message's pictures as thumbnails; a click opens them in the viewer, where they can be marked up into a reply. */
export function MessageImages({ images }: { images: { id: string; name: string }[] }) {
  const [open, setOpen] = useState<number | null>(null); // not kept: the picture viewer, a window over the page
  const reply = useContext(MarkReply);
  if (!images.length) return null;
  const pictures: Picture[] = images.map((a) => ({ id: a.id, name: a.name }));
  return (
    <div className="flex max-w-150 flex-wrap gap-2">
      {images.map((a, i) => (
        <Thumb key={a.id} id={a.id} name={a.name} onOpen={() => setOpen(i)} />
      ))}
      {open !== null && (
        <ImageViewer
          pictures={pictures}
          start={open}
          onClose={() => setOpen(null)}
          onMarked={reply ?? undefined}
          doneLabel="Add to reply"
        />
      )}
    </div>
  );
}

function Thumb({ id, name, onOpen }: { id: string; name: string; onOpen: () => void }) {
  const { url, failed, removed, gone } = useAttachmentUrl(id, true);
  return (
    <button
      type="button"
      onClick={onOpen}
      title={name}
      className="size-40 overflow-hidden rounded-lg border border-border bg-surface outline-none focus-visible:ring-2 focus-visible:ring-accent"
    >
      {url ? (
        <img src={url} alt={name} className="size-full object-cover" />
      ) : (
        <span className="grid size-full place-items-center text-xs text-muted">
          {failed ? 'Did not load' : removed ? 'Deleted' : gone ? 'Not kept' : ''}
        </span>
      )}
    </button>
  );
}
