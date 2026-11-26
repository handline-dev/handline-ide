// Which daemon is the master now (docs/several-machines.md section 4): a master names itself and its number in hello
// (capabilities.master), and an app obeys only the highest it has seen. Runners do not listen, so only a master answers at all.
import { DaemonClient } from '#src/client.ts';
import type { ClientOptions, HelloResult } from '#src/client-options.ts';
import { RpcError } from '#src/errors.ts';
import type { Transport } from '#src/transport.ts';

export type MasterSays = { node: string; number: number };

/** What a hello says of its master; null from a daemon that names none (one from before masters could move). */
export function masterOf(hello: HelloResult | null): MasterSays | null {
  const m = hello?.capabilities.master;
  return m?.node && typeof m.number === 'number' ? { node: m.node, number: m.number } : null;
}

/** a outranks b: a higher number, or at the same number the higher node id (the daemon's own rule, hlclient.MasterSays.Above). */
export const above = (a: MasterSays, b: MasterSays): boolean => a.number > b.number || (a.number === b.number && a.node > b.node);

/** Throws when hello names a master below the one seen (an old master back before it learned), telling `below` first. */
export function belowFloor(gate: ClientOptions['master'], hello: HelloResult): void {
  const floor = gate?.floor();
  const said = floor ? masterOf(hello) : null;
  if (!gate || !floor || !said || !above(floor, said)) return;
  gate.below?.(said);
  throw new RpcError('transport', `master number ${said.number} is below the ${floor.number} this app obeys`);
}

/** How long one machine is given to answer while the app looks for the master. */
export const ASK_MS = 10_000;

/** Says hello at each url at once with token, and answers the one whose master outranks the others and is not below `floor`; null when
 * none answers so. */
export async function highestMaster(
  urls: string[],
  open: (url: string) => Promise<Transport>,
  token: string | undefined,
  client: string,
  floor: MasterSays | null = null,
  askMs = ASK_MS,
): Promise<{ url: string; master: MasterSays } | null> {
  const ask = async (url: string) => {
    const c = new DaemonClient({ client, token, connect: () => open(url), reconnect: false });
    let timer: ReturnType<typeof setTimeout> | undefined;
    try {
      const hello = await Promise.race([
        c.start(),
        new Promise<never>((_, no) => (timer = setTimeout(() => no(new Error('no answer')), askMs))),
      ]);
      const master = masterOf(hello);
      return master && !(floor && above(floor, master)) ? { url, master } : null;
    } catch {
      return null;
    } finally {
      clearTimeout(timer);
      c.close();
    }
  };
  const answers = await Promise.all(urls.map(ask));
  let best: { url: string; master: MasterSays } | null = null;
  for (const a of answers) if (a && (!best || above(a.master, best.master))) best = a;
  return best;
}
