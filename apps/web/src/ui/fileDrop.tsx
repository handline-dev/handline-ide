import { AnimatePresence, motion } from 'framer-motion';
import { ImagePlus } from 'lucide-react';
import { createContext, useRef, useState, type DragEvent, type ReactNode } from 'react';

/** Whether a drag carries files from outside the app (not a tab or a row dragged within it). */
export const carriesFiles = (e: DragEvent) => e.dataTransfer.types.includes('Files');

/** One thing dropped: a file, or a folder with every file under it at its path inside the folder. */
export type Dropped = { kind: 'file' | 'folder'; name: string; files: { path: string; file: File }[] };

/** Every file under a dropped entry, read through the folder tree (`webkitGetAsEntry`, the one way a browser reads a dropped folder). */
async function filesOf(entry: FileSystemEntry, at = ''): Promise<{ path: string; file: File }[]> {
  const path = at ? `${at}/${entry.name}` : entry.name;
  if (entry.isFile) {
    const file = await new Promise<File>((ok, fail) => (entry as FileSystemFileEntry).file(ok, fail));
    return [{ path, file }];
  }
  const reader = (entry as FileSystemDirectoryEntry).createReader();
  const children: FileSystemEntry[] = [];
  // readEntries answers in batches until it answers none.
  for (;;) {
    const batch = await new Promise<FileSystemEntry[]>((ok, fail) => reader.readEntries(ok, fail));
    if (!batch.length) break;
    children.push(...batch);
  }
  return (await Promise.all(children.map((c) => filesOf(c, path)))).flat();
}

/** What a drop or a paste holds, folders read through. Read before the event returns: its items are gone after. */
export function readDropped(data: DataTransfer): Promise<Dropped[]> {
  const entries = [...data.items].flatMap((i) => (i.kind === 'file' ? (i.webkitGetAsEntry() ?? []) : []));
  if (!entries.length)
    return Promise.resolve([...data.files].map((f) => ({ kind: 'file', name: f.name, files: [{ path: f.name, file: f }] })));
  return Promise.all(
    entries.map(async (e) => ({ kind: e.isDirectory ? ('folder' as const) : ('file' as const), name: e.name, files: await filesOf(e) })),
  );
}

/** An area that takes files dropped on it; `over` is true while files are held over it. A drag that is not
 * files (a tab, a row) passes through untouched. */
export function useFileDrop(take: (dropped: Dropped[]) => void) {
  const depth = useRef(0); // dragenter and dragleave also fire for the area's children
  const [over, setOver] = useState(false);
  return {
    over,
    handlers: {
      onDragEnter: (e: DragEvent) => {
        if (!carriesFiles(e)) return;
        // Its own statement: React Compiler 1.0 turns a ref's `x.current++` inside a comparison into a pre-increment.
        depth.current += 1;
        if (depth.current === 1) setOver(true);
      },
      onDragOver: (e: DragEvent) => {
        if (!carriesFiles(e)) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = 'copy';
      },
      onDragLeave: (e: DragEvent) => {
        if (!carriesFiles(e)) return;
        depth.current -= 1;
        if (depth.current === 0) setOver(false);
      },
      onDrop: (e: DragEvent) => {
        if (!carriesFiles(e)) return;
        e.preventDefault();
        depth.current = 0;
        setOver(false);
        void readDropped(e.dataTransfer).then(take);
      },
    },
  };
}

/** True while files are held over the enclosing FileDropArea: its message box lights up and says so. */
export const DropOver = createContext(false);

/** Covers the box that takes the drop while files are held over its area. */
export function DropHint({ shown, label = 'Drop files to attach' }: { shown: boolean; label?: string }) {
  return (
    <AnimatePresence>
      {shown && (
        <motion.div
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          className="pointer-events-none absolute inset-0 z-10 grid place-items-center rounded-xl border-2 border-dashed border-accent bg-bg/85"
        >
          <span className="flex items-center gap-2 text-sm text-fg">
            <ImagePlus size={16} />
            {label}
          </span>
        </motion.div>
      )}
    </AnimatePresence>
  );
}

/** An area that takes files dropped anywhere on it, the chat and a task's page; its message box shows the hint while they are held
 * over it. */
export function FileDropArea({
  take,
  className,
  children,
}: {
  take: (dropped: Dropped[]) => void;
  className: string;
  children: ReactNode;
}) {
  const drop = useFileDrop(take);
  return (
    <div {...drop.handlers} className={`relative ${className}`}>
      <DropOver.Provider value={drop.over}>{children}</DropOver.Provider>
    </div>
  );
}
