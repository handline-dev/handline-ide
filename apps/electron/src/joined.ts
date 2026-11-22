// A home that joined a master, as a cluster engine's does (clusters.ts; docs/story4-design.md section 4): what it keeps of its masters,
// and the shell's connection to the master it obeys, with the device token it joined with. Only a master listens.
import fs from 'node:fs';
import path from 'node:path';
import { highestMaster, listenerUrl, webSocketTransport, type MasterSays, type Transport } from '@handline/client';
import { pinnedWebSocketTransport } from '@handline/client-node';
import { doorsOf, type Door, type Master } from '#src/mode.ts';
import { protectedDir } from '#src/pair.ts';

/** What a runner's home keeps of its masters: the one it obeys (its credential, with this machine's device token there) and the
 * owner's other machines, as its last heartbeat's answer listed them (master-doors.json). */
export interface Joined {
  master: Master;
  /** The master number it obeys: an app goes to none below it. */
  floor: MasterSays | null;
  doors: Door[];
}

const readJson = (file: string): unknown => {
  try {
    return JSON.parse(fs.readFileSync(file, 'utf8'));
  } catch {
    return null;
  }
};

/** The master this home joined, null when it joined none (a master's home, or one that never lent itself). */
export function joinedOf(userHome: string, home: string): Joined | null {
  const m = readJson(path.join(protectedDir(userHome, home), 'master', 'credential')) as Record<string, unknown> | null;
  if (!m || typeof m.token !== 'string' || !m.token || typeof m.device !== 'string') return null;
  const [door] = doorsOf([{ addr: m.master, fp: m.fp }]) ?? [];
  if (!door) return null;
  let self = '';
  try {
    self = fs.readFileSync(path.join(home, 'node_id'), 'utf8').trim();
  } catch {
    /* none yet */
  }
  const listed = readJson(path.join(home, 'master-doors.json'));
  const others = (Array.isArray(listed) ? listed : []) as { node?: unknown }[];
  return {
    master: { host: door.addr, fingerprint: door.fp || null, token: m.token, device: m.device },
    floor: typeof m.node === 'string' && m.node && typeof m.number === 'number' ? { node: m.node, number: m.number } : null,
    doors: others.filter((d) => !self || d.node !== self).flatMap((d) => doorsOf([d]) ?? []),
  };
}

const open = (url: string, fp: string): Promise<Transport> => (fp ? pinnedWebSocketTransport(url, fp) : webSocketTransport(url));

/** Says hello at the master it obeys and at each of the owner's machines at once, each certificate pinned, and answers the one that
 * is master with the highest number, not below the one obeyed; null when none answers so. */
export async function findMaster(j: Joined, ask = highestMaster): Promise<Master | null> {
  const fps = new Map<string, string>();
  for (const d of [{ addr: j.master.host, fp: j.master.fingerprint ?? '' }, ...j.doors]) fps.set(listenerUrl(d.addr, d.fp !== ''), d.fp);
  const best = await ask([...fps.keys()], (url) => open(url, fps.get(url) ?? ''), j.master.token, 'electron', j.floor);
  if (!best) return null;
  const fp = fps.get(best.url) ?? '';
  return { ...j.master, host: new URL(best.url).host, fingerprint: fp || null };
}

/** The shell's own connection to the master: each connect looks for it again, so it follows the master when it moves. */
export const masterTransport = (j: Joined) => async (): Promise<Transport> => {
  const m = await findMaster(j);
  if (!m) throw new Error('no master answers');
  return open(listenerUrl(m.host, m.fingerprint !== null), m.fingerprint ?? '');
};
