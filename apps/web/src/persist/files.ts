// Files kept per browser (IndexedDB), for what kv.ts cannot hold: the files of an unsent message. Each file is a Blob under a key;
// keys are grouped by a prefix (`chat:<agent>:`) so a group is read or dropped together. Without IndexedDB (a private window that
// blocks it) every call answers empty and nothing is kept.
const DB = 'handline-web';
const STORE = 'files';

let opened: Promise<IDBDatabase | null> | undefined;
function db(): Promise<IDBDatabase | null> {
  opened ??= new Promise((ok) => {
    try {
      const req = indexedDB.open(DB, 1);
      req.onupgradeneeded = () => req.result.createObjectStore(STORE);
      req.onsuccess = () => ok(req.result);
      req.onerror = () => ok(null);
    } catch {
      ok(null);
    }
  });
  return opened;
}

async function run<T>(mode: IDBTransactionMode, go: (s: IDBObjectStore) => IDBRequest<T>): Promise<T | undefined> {
  const d = await db();
  if (!d) return undefined;
  return new Promise((ok) => {
    try {
      const req = go(d.transaction(STORE, mode).objectStore(STORE));
      req.onsuccess = () => ok(req.result);
      req.onerror = () => ok(undefined);
    } catch {
      ok(undefined);
    }
  });
}

const group = (prefix: string) => IDBKeyRange.bound(prefix, `${prefix}￿`);

export const putFile = (key: string, blob: Blob) => run('readwrite', (s) => s.put(blob, key)).then(() => undefined);
export const getFile = (key: string) => run<Blob>('readonly', (s) => s.get(key));
export const dropFile = (key: string) => run('readwrite', (s) => s.delete(key)).then(() => undefined);
/** The keys under `prefix`. */
export const fileKeys = async (prefix: string) => ((await run('readonly', (s) => s.getAllKeys(group(prefix)))) ?? []).map(String);
