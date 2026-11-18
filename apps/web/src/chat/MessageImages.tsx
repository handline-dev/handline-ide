import { useAttachmentUrl, useReadAttachment } from '@handline/app-data';
import { File as FileIcon, FileArchive, FileAudio, FileCode, FileSpreadsheet, FileText, FileVideo, Play } from 'lucide-react';
import { createContext, useContext, useState } from 'react';
import type { Attachments } from '#src/chat/attachments.tsx';
import { docOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { toast } from '#src/ui/toast.tsx';
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

/** One attachment: `mime` and `size` when the sender knows them; a reference in a text has neither, and the read answers them. */
export type Shown = { id: string; name: string; mime?: string; size?: number; video?: { id: string; name: string } };

/** A file's size in B, KB or MB. */
export const formatSize = (bytes: number) =>
  bytes < 1024 ? `${bytes} B` : bytes < 1024 ** 2 ? `${Math.round(bytes / 1024)} KB` : `${(bytes / 1024 ** 2).toFixed(1)} MB`;

const kinds = {
  video: FileVideo,
  audio: FileAudio,
  archive: FileArchive,
  sheet: FileSpreadsheet,
  code: FileCode,
  text: FileText,
  other: FileIcon,
};

/** A file's kind, by its type or else its extension. */
function kindOf(name: string, mime?: string | null): keyof typeof kinds {
  const ext = name.split('.').pop()?.toLowerCase() ?? '';
  const t = mime ?? '';
  if (t.startsWith('video/')) return 'video';
  if (t.startsWith('audio/')) return 'audio';
  if (/zip|tar|gzip|compressed|7z|rar/.test(t) || ['zip', 'tar', 'gz', 'tgz', '7z', 'rar'].includes(ext)) return 'archive';
  if (/csv|spreadsheet|excel/.test(t) || ['csv', 'tsv', 'xls', 'xlsx', 'numbers'].includes(ext)) return 'sheet';
  if (
    /json|javascript|typescript|x-sh|xml|yaml/.test(t) ||
    ['json', 'js', 'ts', 'tsx', 'go', 'py', 'sh', 'yaml', 'yml', 'xml', 'html', 'css'].includes(ext)
  )
    return 'code';
  if (t.startsWith('text/') || t === 'application/pdf' || ['md', 'txt', 'pdf', 'doc', 'docx', 'rtf'].includes(ext)) return 'text';
  return 'other';
}

/** The icon for a file's kind. */
export function FileKindIcon({ name, mime, size = 20 }: { name: string; mime?: string | null; size?: number }) {
  const Icon = kinds[kindOf(name, mime)];
  return <Icon size={size} className="shrink-0 text-muted" />;
}

const isPicture = (mime: string | null | undefined) => !mime || mime.startsWith('image/');

/** Every place's attachments: a picture as a thumbnail (opens the viewer), a video as its poster, any other file as a chip that saves it; `small` halves thumbnails. */
export function MessageImages({ images, small = false }: { images: Shown[]; small?: boolean }) {
  const [open, setOpen] = useState<string | null>(null);
  const reply = useContext(MarkReply);
  if (!images.length) return null;
  const pictures: Picture[] = images.filter((a) => !a.video && isPicture(a.mime)).map((a) => ({ id: a.id, name: a.name }));
  const start = pictures.findIndex((p) => p.id === open);
  // Files and pictures in rows of their own: a short file chip beside a tall picture looks out of place.
  const files = images.filter((a) => !isPicture(a.mime));
  const shown = images.filter((a) => isPicture(a.mime));
  return (
    // Never wider than the column it sits in: in a right-aligned message a wider row would run out on the left, where it is cut.
    <div className="flex max-w-full min-w-0 flex-col gap-2">
      <div className="flex max-w-150 min-w-0 flex-col gap-2">
        {files.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {files.map((a) => (
              <FileChip key={a.id} id={a.id} name={a.name} mime={a.mime} bytes={a.size} />
            ))}
          </div>
        )}
        {shown.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {shown.map((a) => (
              <Thumb key={a.id} a={a} small={small} onOpen={() => (a.video ? playVideo(a.video.id, a.video.name) : setOpen(a.id))} />
            ))}
          </div>
        )}
      </div>
      {start >= 0 && (
        <ImageViewer pictures={pictures} start={start} onClose={() => setOpen(null)} onMarked={reply?.add} doneLabel={reply?.label} />
      )}
    </div>
  );
}

/** Saves an attachment as a file in the browser's downloads. */
export const saveAttachment = (read: (id: string) => Promise<Blob>) => async (id: string, name: string) => {
  const blob = await read(id).catch(
    (e: unknown) => void toast(`${name} did not save: ${e instanceof Error ? e.message : String(e)}`, { tone: 'warning' }),
  );
  if (!blob) return;
  const url = URL.createObjectURL(blob);
  Object.assign(document.createElement('a'), { href: url, download: name }).click();
  setTimeout(() => URL.revokeObjectURL(url), 60_000); // a browser may still be starting the download
};

const isMarkdown = (name: string, mime?: string | null) => mime === 'text/markdown' || /\.(md|markdown)$/i.test(name);

/** A file that is not a picture: an icon for its kind, its name, its extension and size. A click opens a Markdown file in a tab of
 * its own (tools/DocTab) and saves any other. */
function FileChip({ id, name, mime, bytes }: { id: string; name: string; mime?: string | null; bytes?: number | null }) {
  const read = useReadAttachment();
  const ext = name.includes('.') ? name.split('.').pop()!.toUpperCase() : '';
  const md = isMarkdown(name, mime);
  const open = useOpenable(md ? docOf(id, name) : null);
  const save = saveAttachment(read);
  return (
    <button
      type="button"
      {...(md ? open : { onClick: () => void save(id, name) })}
      title={md ? `Open ${name}` : `Save ${name}`}
      className="flex h-16 max-w-60 items-center gap-2 overflow-hidden rounded-md border border-border px-3 text-left outline-none hover:bg-hover focus-visible:ring-2 focus-visible:ring-accent"
    >
      <FileKindIcon name={name} mime={mime} />
      <span className="flex min-w-0 flex-col">
        <span className="truncate text-sm text-fg">{name}</span>
        <span className="text-xs text-muted">{[ext, bytes != null ? formatSize(bytes) : ''].filter(Boolean).join(' · ')}</span>
      </span>
    </button>
  );
}

function Thumb({ a, small, onOpen }: { a: Shown; small: boolean; onOpen: () => void }) {
  const { url, mime, size, failed, removed, gone } = useAttachmentUrl(a.id, true);
  // A reference in a text names no type: one that reads back as another kind of file is drawn as a file.
  if (!a.video && !isPicture(mime)) return <FileChip id={a.id} name={a.name} mime={mime} bytes={size} />;
  return (
    <button
      type="button"
      onClick={onOpen}
      title={a.name}
      className={`relative max-w-full ${url ? '' : small ? 'w-20' : 'w-40'} ${small ? 'h-20' : 'h-40'} overflow-hidden rounded-lg border border-border bg-surface outline-none focus-visible:ring-2 focus-visible:ring-accent`}
    >
      {/* The picture's own shape at one height, its width kept between 80 and 320: a long one shows its top. In a column narrower
          than that the button clips it, from the right. */}
      {url ? (
        <img src={url} alt={a.name} className="h-full w-auto max-w-80 min-w-20 object-cover object-top" />
      ) : (
        <span className="grid size-full place-items-center text-xs text-muted">
          {failed ? 'Did not load' : removed ? 'Deleted' : gone ? 'Not kept' : ''}
        </span>
      )}
      {a.video && (
        <span className="absolute inset-0 m-auto flex size-8 items-center justify-center rounded-full bg-fg/60 text-bg">
          <Play size={14} />
        </span>
      )}
    </button>
  );
}
