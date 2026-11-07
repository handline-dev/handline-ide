// The files of an agent's unsent message, kept per browser so they are still there after a reload: each added file or folder is an
// entry, its blobs in persist/files.ts and the list in persist/kv.ts. Restored entries go up again; the daemon drops an unsent upload
// of its own after a day.
import { read, write } from '#src/persist/kv.ts';
import { dropFile, fileKeys, getFile, putFile } from '#src/persist/files.ts';
import type { Dropped } from '#src/ui/fileDrop.tsx';
import { uuid } from '#src/ui/uuid.ts';

type Entry = { id: string; kind: Dropped['kind']; name: string; files: { path: string; name: string; type: string }[] };

const listOf = (agent: string) => `chat-files/${agent}`;
const prefix = (agent: string, id = '') => `chat:${agent}:${id}`;
const blobKey = (agent: string, id: string, i: number) => `${prefix(agent, id)}:${i}`;

/** Keeps a dropped file or folder; answers its entry id. */
export function keepEntry(agent: string, d: Dropped): string {
  const id = uuid();
  d.files.forEach((f, i) => void putFile(blobKey(agent, id, i), f.file));
  const entry: Entry = {
    id,
    kind: d.kind,
    name: d.name,
    files: d.files.map((f) => ({ path: f.path, name: f.file.name, type: f.file.type })),
  };
  write(listOf(agent), [...read<Entry[]>(listOf(agent), []), entry]);
  return id;
}

export function dropEntry(agent: string, id: string) {
  const list = read<Entry[]>(listOf(agent), []);
  const e = list.find((x) => x.id === id);
  if (!e) return;
  e.files.forEach((_, i) => void dropFile(blobKey(agent, id, i)));
  write(
    listOf(agent),
    list.filter((x) => x.id !== id),
  );
}

export async function dropAll(agent: string) {
  write(listOf(agent), undefined);
  for (const k of await fileKeys(prefix(agent))) void dropFile(k);
}

/** What was kept for `agent`, read back as dropped entries and taken off the list (adding them keeps them again). An entry whose
 * blobs are gone is left out. */
export async function takeKept(agent: string): Promise<Dropped[]> {
  const list = read<Entry[]>(listOf(agent), []);
  if (!list.length) return [];
  const out: Dropped[] = [];
  for (const e of list) {
    const blobs = await Promise.all(e.files.map((_, i) => getFile(blobKey(agent, e.id, i))));
    if (blobs.every(Boolean))
      out.push({
        kind: e.kind,
        name: e.name,
        files: e.files.map((f, i) => ({ path: f.path, file: new File([blobs[i]!], f.name, { type: f.type }) })),
      });
  }
  await dropAll(agent);
  return out;
}
