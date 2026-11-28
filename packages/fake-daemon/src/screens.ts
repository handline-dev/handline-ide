// The fake daemon's live screen sources (screen.attach and the rest): one fixed frame per source, who holds it, and, for an agent a
// seed marks busy, its clicks and typing sent to watchers every couple of seconds.
import type { Conn } from '#src/conn.ts';
import { badRequest } from '#src/errors.ts';
import { FRAME, TARGETS } from '#src/screenframe.ts';

type Holder = { kind: 'agent' | 'person' | 'device'; name: string; since: string | null };
type Notify = (c: Conn, method: string, params: unknown) => void;
const STEP_MS = 1800;

export class FakeScreens {
  private holders = new Map<string, Holder>();
  private watchers = new Map<string, Set<Conn>>();
  private timer: ReturnType<typeof setInterval> | undefined;
  private step = 0;
  /** `busy`: the agent and source whose demo clicks play while someone watches. */
  busy: { agent: string; source: string } | null = null;

  private notify: Notify;
  private iso: () => string;
  constructor(notify: Notify, iso: () => string) {
    this.notify = notify;
    this.iso = iso;
  }

  owns = (method: string) => ['screen.attach', 'screen.detach', 'screen.take', 'screen.give_back', 'screen.input'].includes(method);

  call(method: string, p: Record<string, unknown>, c: Conn, who: string, ok: (said: string, extra?: object) => object): unknown {
    const agent = String(p.agent);
    const source = String(p.source);
    if (source !== 'browser' && source !== 'sim') throw badRequest('source is browser or sim', 'source');
    const key = `${agent}/${source}`;
    const holder = () => this.holders.get(key) ?? { kind: 'agent' as const, name: agent, since: null };
    const tell = () => this.send(key, 'screen_holder', { agent, source, holder: holder() });
    if (method === 'screen.attach') {
      this.watchers.set(key, (this.watchers.get(key) ?? new Set()).add(c));
      this.tick();
      return { agent, source, seq: 0, frame: FRAME, holder: holder(), in_use: this.busy?.agent === agent };
    }
    if (method === 'screen.detach') {
      this.watchers.get(key)?.delete(c);
      return {};
    }
    if (method === 'screen.take') {
      this.holders.set(key, { kind: 'person', name: who, since: this.iso() });
      tell();
      return ok(`you have ${agent}'s ${source}`, { holder: holder() });
    }
    if (method === 'screen.give_back') {
      this.holders.delete(key);
      tell();
      return ok(`${agent} has its ${source} back`, { holder: holder() });
    }
    return ok('sent');
  }

  private send(key: string, method: string, params: object) {
    for (const c of this.watchers.get(key) ?? []) this.notify(c, method, params);
  }

  /** While someone watches the busy source and the agent holds it, its next click or typing goes out. */
  private tick() {
    if (this.timer || !this.busy) return;
    this.timer = setInterval(() => {
      const { agent, source } = this.busy!;
      const key = `${agent}/${source}`;
      if (this.holders.has(key) || !this.watchers.get(key)?.size) return;
      const t = TARGETS[this.step++ % TARGETS.length];
      this.send(key, 'screen_action', { agent, source, kind: t.kind, box: t.box, at: this.iso() });
    }, STEP_MS);
    this.timer.unref?.();
  }
}
