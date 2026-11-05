// What a message carries besides its text. Files of any type go up to the daemon as they are added (attachment.put), at most 10 MB
// each and 6 to a message; each is kept per browser (keptFiles.ts) until sent or removed, so a reload brings it back. A folder goes up as one attachment (attachment.folder, then each file into it); a daemon that does not
// take folders holds it as a chip, and `sendable` is false while one is held or going up, so nothing is dropped unseen. `access` is
// the state a path the agent may not read will take (access_needed).
import { keptState, MAX_PER_MESSAGE, uploadFolder, useImageDrafts, useServes, useWrite, type ImageDraft } from '@handline/app-data';
import { File as FileIcon, Folder, RotateCw, X } from 'lucide-react';
import { useEffect, type ReactNode } from 'react';
import { dropAll, dropEntry, keepEntry, takeKept } from '#src/chat/keptFiles.ts';
import type { Dropped } from '#src/ui/fileDrop.tsx';

const IMAGE = /^image\/(png|jpeg|gif|webp)$/;

/** A folder with the message: going up (`progress` of its bytes), up (`id`), failed (`error`), or held back and why. */
export type FolderDraft = Dropped & {
  key: number;
  state: 'uploading' | 'up' | 'failed' | 'unsupported' | 'access';
  progress: number;
  id?: string;
  error?: string;
};

let nextKey = 1;
/** Per agent: the kept entry (keptFiles.ts) of each chip, by `u<key>` for a file and `f<key>` for a folder. */
const entries = new Map<string, Map<string, string>>();
const restored = new Set<string>();
const useHeld = keptState<{ folders: FolderDraft[] }>(() => ({ folders: [] }));

/** An agent's attachments for its next message, kept while the founder looks elsewhere. */
export function useAttachments(agent: string) {
  const uploads = useImageDrafts(MAX_PER_MESSAGE, agent, true);
  const held = useHeld(agent);
  const { folders } = held.state;
  const takes = useServes('attachment.folder').served;
  const put = useWrite('attachment.put');
  const open = useWrite('attachment.folder');
  const patch = (key: number, p: Partial<FolderDraft>) =>
    held.set((s) => ({ folders: s.folders.map((f) => (f.key === key ? { ...f, ...p } : f)) }));
  const send = (d: FolderDraft) => {
    patch(d.key, { state: 'uploading', progress: 0, error: undefined });
    const call = (m: string, p: unknown) => (m === 'attachment.folder' ? open.mutateAsync(p as never) : put.mutateAsync(p as never));
    const files = d.files.map((f) => ({ path: f.path, file: f.file, mime: (f.file.type || 'application/octet-stream') as never }));
    uploadFolder(call, d.name, files, (sent, total) => patch(d.key, { progress: total ? sent / total : 1 })).then(
      (a) => patch(d.key, { state: 'up', id: a.id }),
      (e: unknown) => patch(d.key, { state: 'failed', error: e instanceof Error ? e.message : String(e) }),
    );
  };
  const kept = entries.get(agent) ?? new Map<string, string>();
  entries.set(agent, kept);
  const forget = (chip: string) => {
    const id = kept.get(chip);
    if (id) dropEntry(agent, id);
    kept.delete(chip);
  };
  const add = (dropped: Dropped[]) => {
    const singles = dropped.filter((d) => d.kind === 'file');
    const files = singles.flatMap((d) => d.files.map((f) => f.file));
    const keys = uploads.add(
      files.map((file) => ({
        blob: file,
        name: file.name,
        mime: file.type || 'application/octet-stream',
        // Only an image has a picture to show while it goes up.
        preview: IMAGE.test(file.type) ? URL.createObjectURL(file) : '',
      })),
    );
    keys.forEach((k, i) =>
      kept.set(`u${k}`, keepEntry(agent, { kind: 'file', name: files[i]!.name, files: [{ path: files[i]!.name, file: files[i]! }] })),
    );
    const more = dropped
      .filter((d) => d.kind === 'folder')
      .map((d): FolderDraft => ({ ...d, key: nextKey++, state: takes ? 'uploading' : 'unsupported', progress: 0 }));
    if (!more.length) return;
    for (const d of more) kept.set(`f${d.key}`, keepEntry(agent, d));
    held.set((s) => ({ folders: [...s.folders, ...more] }));
    if (takes) more.forEach(send);
  };
  // What a reload left behind comes back once per page, and goes up again.
  useEffect(() => {
    if (restored.has(agent)) return;
    restored.add(agent);
    void takeKept(agent).then((back) => back.length && add(back));
    // add reads this render's hooks; it runs once per agent
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agent]);
  return {
    uploads: uploads.drafts,
    folders,
    problem: uploads.problem,
    add,
    remove: (key: number) => {
      forget(`u${key}`);
      uploads.remove(key);
    },
    retry: uploads.retry,
    retryFolder: (key: number) => {
      const d = folders.find((f) => f.key === key);
      if (d) send(d);
    },
    removeFolder: (key: number) => {
      forget(`f${key}`);
      held.set((s) => ({ folders: s.folders.filter((f) => f.key !== key) }));
    },
    clear: () => {
      kept.clear();
      void dropAll(agent);
      uploads.clear();
      held.set({ folders: [] });
    },
    /** Uploaded in full: the ids to name in the text. */
    ids: [...uploads.ids, ...folders.flatMap((f) => (f.id ? [f.id] : []))],
    /** Every file and folder is up and nothing is held back. */
    sendable: !uploads.busy && folders.every((f) => f.state === 'up'),
  };
}

export type Attachments = ReturnType<typeof useAttachments>;

const size = (bytes: number) =>
  bytes < 1024 ? `${bytes} B` : bytes < 1024 ** 2 ? `${Math.round(bytes / 1024)} KB` : `${(bytes / 1024 ** 2).toFixed(1)} MB`;

/** A file on its way with the message: an image as its picture (a click opens it to look or mark up), another file as its name; faded until it is up, with remove
 * and, after a failure, retry. */
export function UploadChip({
  draft,
  onOpen,
  onRemove,
  onRetry,
}: {
  draft: ImageDraft;
  onOpen?: () => void;
  onRemove: () => void;
  onRetry: () => void;
}) {
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
        <button type="button" aria-label={`Open ${draft.name}`} onClick={onOpen} className="size-full outline-none">
          <img src={draft.preview} alt={draft.name} className={`size-full object-cover ${up ? '' : 'opacity-50'}`} />
        </button>
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

const why = { unsupported: 'Folders are not taken yet', access: 'Needs access', failed: 'Did not upload' };

/** A folder with the message: its name, its file count and size; faded with a bar while it goes up, and why when it failed or is held. */
export function FolderChip({
  draft,
  onRemove,
  onRetry,
  onAllow,
}: {
  draft: FolderDraft;
  onRemove: () => void;
  onRetry: () => void;
  onAllow?: () => void;
}) {
  const bytes = draft.files.reduce((n, f) => n + f.file.size, 0);
  const facts = `${draft.files.length} files, ${size(bytes)}`;
  return (
    <Card icon={<Folder size={20} />} name={draft.name} title={draft.error ?? draft.name} faded={draft.state !== 'up'}>
      {draft.state === 'up' || draft.state === 'uploading' ? (
        <span className="truncate text-xs text-muted">{facts}</span>
      ) : draft.state === 'access' && onAllow ? (
        <button type="button" onClick={onAllow} className="self-start text-xs text-accent hover:text-fg">
          Allow access
        </button>
      ) : (
        <span className="truncate text-xs text-warning">{why[draft.state]}</span>
      )}
      {draft.state === 'uploading' && (
        <div className="absolute inset-x-0 bottom-0 h-1 bg-border">
          <div className="h-full bg-accent transition-all" style={{ width: `${draft.progress * 100}%` }} />
        </div>
      )}
      {draft.state === 'failed' && (
        <button type="button" aria-label="Retry" onClick={onRetry} className="absolute inset-0 grid place-items-center text-danger">
          <RotateCw size={16} />
        </button>
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
