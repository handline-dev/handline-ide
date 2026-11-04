// What a message carries besides its text. Files of any type go up to the daemon as they are added (attachment.put), at most 10 MB
// each and 6 to a message. Folders are held as chips until the daemon takes them (attachment.folder, T8506): `sendable` is false
// while one is held, so nothing is dropped unseen. `access` is the state a path the agent may not read will take (access_needed).
import { keptState, MAX_PER_MESSAGE, useImageDrafts, type ImageDraft } from '@handline/app-data';
import { File as FileIcon, Folder, RotateCw, X } from 'lucide-react';
import type { ReactNode } from 'react';
import type { Dropped } from '#src/ui/fileDrop.tsx';

const IMAGE = /^image\/(png|jpeg|gif|webp)$/;

/** A folder held back from the message, and why. */
export type FolderDraft = Dropped & { key: number; state: 'unsupported' | 'access' };

let nextKey = 1;
const useHeld = keptState<{ folders: FolderDraft[] }>(() => ({ folders: [] }));

/** An agent's attachments for its next message, kept while the founder looks elsewhere. */
export function useAttachments(agent: string) {
  const uploads = useImageDrafts(MAX_PER_MESSAGE, agent, true);
  const held = useHeld(agent);
  const { folders } = held.state;
  return {
    uploads: uploads.drafts,
    folders,
    problem: uploads.problem,
    add: (dropped: Dropped[]) => {
      const files = dropped.flatMap((d) => (d.kind === 'file' ? d.files.map((f) => f.file) : []));
      uploads.add(
        files.map((file) => ({
          blob: file,
          name: file.name,
          mime: file.type || 'application/octet-stream',
          // Only an image has a picture to show while it goes up.
          preview: IMAGE.test(file.type) ? URL.createObjectURL(file) : '',
        })),
      );
      const more = dropped.filter((d) => d.kind === 'folder').map((d): FolderDraft => ({ ...d, key: nextKey++, state: 'unsupported' }));
      if (more.length) held.set((s) => ({ folders: [...s.folders, ...more] }));
    },
    remove: uploads.remove,
    retry: uploads.retry,
    removeFolder: (key: number) => held.set((s) => ({ folders: s.folders.filter((f) => f.key !== key) })),
    clear: () => {
      uploads.clear();
      held.set({ folders: [] });
    },
    /** Uploaded in full: the ids to name in the text. */
    ids: uploads.ids,
    /** Every file is up and nothing is held back. */
    sendable: !uploads.busy && folders.length === 0,
  };
}

export type Attachments = ReturnType<typeof useAttachments>;

const size = (bytes: number) =>
  bytes < 1024 ? `${bytes} B` : bytes < 1024 ** 2 ? `${Math.round(bytes / 1024)} KB` : `${(bytes / 1024 ** 2).toFixed(1)} MB`;

/** A file on its way with the message: an image as its picture, another file as its name; faded until it is up, with remove
 * and, after a failure, retry. */
export function UploadChip({ draft, onRemove, onRetry }: { draft: ImageDraft; onRemove: () => void; onRetry: () => void }) {
  const up = Boolean(draft.attachment);
  const bar = !up && !draft.error && (
    <div className="absolute inset-x-0 bottom-0 h-1 bg-border">
      <div className="h-full bg-accent transition-all" style={{ width: `${draft.progress * 100}%` }} />
    </div>
  );
  const retry = draft.error && (
    <button type="button" aria-label="Retry" onClick={onRetry} className="absolute inset-0 grid place-items-center text-danger">
      <RotateCw size={16} />
    </button>
  );
  if (draft.preview)
    return (
      <div className="group relative size-16 overflow-hidden rounded-md border border-border" title={draft.error ?? draft.name}>
        <img src={draft.preview} alt={draft.name} className={`size-full object-cover ${up ? '' : 'opacity-50'}`} />
        {bar}
        {retry}
        <Remove onClick={onRemove} />
      </div>
    );
  return (
    <Card icon={<FileIcon size={20} />} name={draft.name} title={draft.error ?? draft.name} faded={!up}>
      <span className="truncate text-xs text-muted">{draft.error ?? (up ? size(draft.attachment!.bytes) : 'Uploading')}</span>
      {bar}
      {retry}
      <Remove onClick={onRemove} />
    </Card>
  );
}

const why = { unsupported: 'Folders are not taken yet', access: 'Needs access' };

/** A folder with the message: its name, its file count and size, and why it is held. */
export function FolderChip({ draft, onRemove, onAllow }: { draft: FolderDraft; onRemove: () => void; onAllow?: () => void }) {
  const bytes = draft.files.reduce((n, f) => n + f.file.size, 0);
  return (
    <Card icon={<Folder size={20} />} name={draft.name} title={draft.name}>
      <span className="truncate text-xs text-muted">{`${draft.files.length} files, ${size(bytes)}`}</span>
      {draft.state === 'access' && onAllow ? (
        <button type="button" onClick={onAllow} className="self-start text-xs text-accent hover:text-fg">
          Allow access
        </button>
      ) : (
        <span className="truncate text-xs text-warning">{why[draft.state]}</span>
      )}
      <Remove onClick={onRemove} />
    </Card>
  );
}

function Card({
  icon,
  name,
  title,
  faded = false,
  children,
}: {
  icon: ReactNode;
  name: string;
  title: string;
  faded?: boolean;
  children: ReactNode;
}) {
  return (
    <div
      className="group relative flex h-16 max-w-60 items-center gap-2 overflow-hidden rounded-md border border-border px-3"
      title={title}
    >
      <span className={`text-muted ${faded ? 'opacity-50' : ''}`}>{icon}</span>
      <span className="flex min-w-0 flex-col">
        <span className="truncate text-sm text-fg">{name}</span>
        {children}
      </span>
    </div>
  );
}

function Remove({ onClick }: { onClick: () => void }) {
  return (
    <button
      type="button"
      aria-label="Remove"
      onClick={onClick}
      className="absolute top-1 right-1 grid size-4 place-items-center rounded-full bg-fg text-bg opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
    >
      <X size={12} />
    </button>
  );
}
