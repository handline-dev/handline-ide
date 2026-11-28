// The fake daemon's workspaces: workspaces.list, and the owner's workspace.create, workspace.set and workspace.delete. A workspace is the
// first segment of its agents' keys (names.ts); a rename or a folder is kept here by segment, so keys never change.
import type { Actor, Repo, Topic, Workspace } from '@handline/contract';
import { textRefusal } from '#src/agentcontext.ts';
import { AGENTS } from '#src/fixtures.ts';
import { Failure, invalidParams, refused } from '#src/errors.ts';
import { identOf, PEOPLE, segment, workspacesOf } from '#src/names.ts';
import type { Session } from '#src/session.ts';
import type { FakeWork } from '#src/workroom.ts';
import type { FakeWorktrees } from '#src/worktrees.ts';

interface Host {
  sessions: Map<string, Session>;
  work: FakeWork;
  held: Map<string, unknown[]>;
  admin: { worktrees: FakeWorktrees };
  context: { keep(agent: string, texts: Record<string, string>, by: string): unknown; texts: Map<string, unknown> };
  emit(topic: Topic, type: string, f?: { agent?: string; actor?: Actor; data?: Record<string, unknown> }): void;
}
type Ok = (said: string, extra?: object) => object;

const METHODS = /^(workspaces\.list|workspace\.(create|set|delete))$/;
const NAME = /^[a-z][a-z0-9-]{0,31}$/;
const HOME = '/Users/me';
/** Folders that are there but in no git checkout: taken as plain folders (git false). Any other unknown folder is not_found. */
const PLAIN_FOLDERS = [`${HOME}/Documents/recipe-site`];
/** The seeded handline workspace holds a second repo and a plain folder beside its own. */
const SEEDED_ADDED: [string, string[]][] = [['handline', ['/work/docs-site', PLAIN_FOLDERS[0]!]]];

/** The names a new agent gets when none is typed, as the daemon's names.Plain. */
export const PLAIN_NAMES = [
  'ada',
  'bea',
  'cal',
  'dot',
  'eli',
  'fay',
  'gus',
  'ivy',
  'jo',
  'kit',
  'lou',
  'mia',
  'ned',
  'ola',
  'pip',
  'quin',
  'rex',
  'sam',
  'tess',
  'uma',
  'val',
  'wes',
  'xia',
  'yan',
  'zoe',
];

/** A folder's name as a workspace name: lowercase, other runs as -, starting with a letter, at most 32. */
const nameFrom = (w: string) =>
  w
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^[-0-9]+|-+$/g, '')
    .slice(0, 32)
    .replace(/-+$/, '');

const notFound = (message: string, what: string) => new Failure('not_found', message, { what });

export class FakeWorkspaces {
  private names = new Map<string, string>();
  private folders = new Map<string, string>();
  private added = new Map<string, string[]>(SEEDED_ADDED);
  private modes = new Map<string, Workspace['session_mode']>();
  /** The folder each agent works in, when it is not its workspace's default. */
  private works = new Map<string, string>();
  /** Segments whose lead is being made: their name and folder are taken before the lead is registered. */
  private starting = new Set<string>();
  /** Plain folders: the seeded ones and those workspace.create made under ~/Handline. */
  private plain = new Set(PLAIN_FOLDERS);
  private host: Host;

  constructor(host: Host) {
    this.host = host;
  }

  owns = (method: string): boolean => METHODS.test(method);

  list(): Workspace[] {
    const seats = new Set([...this.host.sessions].filter(([, s]) => s.fx.view?.hired_from).map(([k]) => k));
    // A workspace being made is listed at once, as the daemon's row is written before its lead starts.
    const keys = [...this.host.sessions.keys(), ...this.unborn()];
    return workspacesOf(keys, seats, this.names, this.folderOf(), this.added, (f) => this.plain.has(f), this.modes);
  }

  /** The segments still starting: one whose lead is registered is no longer, so its lead can go as any lead does. */
  private unborn(): string[] {
    for (const seg of this.starting) if (this.host.sessions.has(seg)) this.starting.delete(seg);
    return [...this.starting];
  }

  /** The folder agent key works in: its own, else its workspace's default. */
  folderFor(key: string): string | null {
    const ws = this.list().find((w) => w.key === identOf(key).workspace);
    return this.works.get(key) ?? ws?.folder ?? null;
  }

  /** The session_mode of the workspace agent key belongs to. */
  modeOf(key: string): Workspace['session_mode'] {
    return this.modes.get(segment(key)) ?? 'terminal';
  }

  /** The name agent.create gives an agent of workspace (key or name) when none is typed: the first plain name free there, else ada-2, ada-3 and so on. */
  defaultName(workspace: unknown): string {
    const { seg } = this.find(workspace);
    const free = (n: string) => !this.host.sessions.has(`${seg}.${n}`);
    const named = PLAIN_NAMES.find(free);
    if (named) return named;
    for (let i = 2; ; i++) if (free(`${PLAIN_NAMES[0]}-${i}`)) return `${PLAIN_NAMES[0]}-${i}`; // as names.Free: the first free with a number
  }

  /** agent.create's workspace and folder: the dotted name it registers, and the folder of it (its default if none) as a repo. */
  place(name: string, workspace: unknown, folder: unknown): { name: string; repo: Repo; git: boolean } {
    if (name.includes('.')) throw invalidParams('name is a plain name in its workspace, with no dot', 'name');
    const { seg, ws } = this.find(workspace);
    if (!ws.folder) throw refused('state', `workspace ${ws.name} has no folder to work in yet`);
    const f = folder === undefined ? ws.folders[0] : ws.folders.find((x) => x.path === folder);
    if (!f) throw invalidParams(`folder is one of workspace ${ws.name}'s folders`, 'folder');
    return { name: `${seg}.${name}`, repo: this.repoAt(f.path), git: f.git };
  }

  /** Agent works in folder from now on: kept only when it is not its workspace's default. */
  settle(agent: string, folder: string): void {
    if (folder === this.list().find((w) => w.key === identOf(agent).workspace)?.folder) this.works.delete(agent);
    else this.works.set(agent, folder);
  }

  /** The repo a folder is: a known one, or a plain folder as one on main. */
  private repoAt(path: string): Repo {
    const { repos, recent } = this.host.admin.worktrees;
    const known = [...repos, ...recent].find((r) => r.path === path);
    return { default_branch: 'main', setup: { script: '', copy: [], last: null }, ...known, name: path.split('/').pop()!, path };
  }

  /** Each segment's folder: a seeded workspace's is the repo of its name. */
  private folderOf(): Map<string, string> {
    return new Map([...this.host.admin.worktrees.repos.map((r): [string, string] => [r.name, r.path]), ...this.folders]);
  }

  private segments = () => new Set([...this.host.sessions.keys()].map(segment).concat(...this.unborn()));

  call(method: string, p: Record<string, unknown>, ok: Ok, actor: Actor): unknown {
    switch (method) {
      case 'workspaces.list':
        return { workspaces: this.list() };
      case 'workspace.create':
        return this.create(p, ok, actor);
      case 'workspace.set':
        return this.set(p, ok);
      default:
        return this.remove(p, ok, actor);
    }
  }

  /** The workspace a key (w3) or a name names, as its segment. */
  private find(ref: unknown): { seg: string; ws: Workspace } {
    const ws = this.list().find((w) => w.key === ref || w.name === ref);
    const seg = ws && [...this.segments()].find((s) => (this.names.get(s) ?? s) === ws.name);
    if (ws && seg) return { seg, ws };
    throw notFound(`no workspace ${String(ref)}`, 'workspace');
  }

  private checkName(name: unknown, self?: string): string {
    if (typeof name !== 'string' || !NAME.test(name))
      throw invalidParams('name is lowercase letters, digits and -, 1 to 32 long, starting with a letter', 'name');
    const taken = this.list().some((w) => w.name === name) || this.segments().has(name) || PEOPLE.some((x) => x.handle === name);
    if (name !== self && taken) throw refused('state', `${name} is already a workspace's or a person's name`);
    return name;
  }

  /** base, else base-2, base-3 and so on: the first no workspace, agent segment or handle has. */
  private freeName(base: string): string {
    const free = (n: string) => !this.list().some((w) => w.name === n) && !this.segments().has(n) && !PEOPLE.some((x) => x.handle === n);
    for (let i = 1; ; i++) if (free(i === 1 ? base : `${base.slice(0, 28)}-${i}`)) return i === 1 ? base : `${base.slice(0, 28)}-${i}`;
  }

  private agentsOf = (seg: string) => [...this.host.sessions.keys()].filter((k) => segment(k) === seg);

  private busy(seg: string): void {
    const open = this.agentsOf(seg).filter((k) => this.host.sessions.get(k)!.open);
    if (open.length)
      throw refused('state', `${open.join(', ')} ${open.length === 1 ? 'has a session' : 'have sessions'} running: close them first.`);
  }

  /** workspace.create: the checkout the folder is in, then agent.create's operation for its lead. Another workspace's folder fails it.
   * No name: the folder's, else "workspace", the first free; no folder: a new one ~/Handline/<name>. A role and base given are the
   * lead's first versions, by the caller. */
  private create(p: Record<string, unknown>, ok: Ok, actor: Actor) {
    const blank = (v: unknown) => v === undefined || (typeof v === 'string' && !v.trim());
    if (!blank(p.folder) && typeof p.folder !== 'string') throw invalidParams('folder is an absolute path', 'folder');
    const texts: Record<string, string> = {};
    for (const k of ['role', 'base'] as const) {
      const v = p[k];
      if (v === undefined || v === null) continue;
      const why = typeof v === 'string' ? textRefusal(k, v) : `${k} is text`;
      if (why) throw invalidParams(why, k);
      texts[k] = v as string;
    }
    let at: string | undefined;
    if (!blank(p.folder)) {
      const folder = (p.folder as string).replace(/^~(?=\/|$)/, HOME);
      const { repos, recent } = this.host.admin.worktrees;
      const top = [...repos, ...recent].find((r) => folder === r.path || folder.startsWith(`${r.path}/`))?.path;
      at = top ?? (this.plain.has(folder) ? folder : undefined);
      if (!at) throw notFound(`${folder} is not a folder`, 'folder');
    }
    const name = blank(p.name) ? this.freeName(nameFrom(at?.split('/').pop() ?? '') || 'workspace') : this.checkName(p.name);
    if (!at) {
      at = `${HOME}/Handline/${name}`;
      this.plain.add(at);
    }
    const repo = this.repoAt(at);
    const segs = this.segments();
    const other = [...this.folderOf()].find(([seg, f]) => f === repo.path && segs.has(seg))?.[0];
    this.names.delete(name);
    if (!other) this.starting.add(name);
    if (!other) this.folders.set(name, repo.path);
    if (!other) this.host.context.keep(name, texts, actor.name);
    const fx = {
      ...AGENTS[0]!,
      name,
      role: 'lead',
      state: 'parked' as const,
      account: null,
      task: null,
      worktree: `${repo.path}.${name}`,
      branch: `agent/${name}`,
      session: null,
    };
    const fails = other && `${repo.path} is the folder of workspace ${this.names.get(other) ?? other}`;
    const id = this.host.work.launch('workspace.create', fx, repo.default_branch, repo, fails);
    // The daemon's workspaces table has the new row at once, and its watcher says so.
    if (!other) this.host.emit('workspaces', 'workspaces.changed', { data: {} });
    return ok(`Creating workspace ${name}.`, { operation: id });
  }

  /** workspace.set: a new name (its agents' sessions run on), a folder added or one taken away, or its session_mode. Every part is
   * checked before any is made. */
  private set(p: Record<string, unknown>, ok: Ok) {
    const { seg, ws } = this.find(p.workspace);
    for (const k of ['add_folder', 'remove_folder', 'session_mode'] as const)
      if (p[k] !== undefined && (typeof p[k] !== 'string' || !p[k])) throw invalidParams(`${k} is a non-empty string`, k);
    const add = p.add_folder as string | undefined;
    const gone = p.remove_folder as string | undefined;
    const mode = p.session_mode as Workspace['session_mode'] | undefined;
    if (p.name === undefined && !add && !gone && !mode)
      throw invalidParams('name, add_folder, remove_folder or session_mode says what changes', 'name');
    if (mode !== undefined && mode !== 'terminal' && mode !== 'chat')
      throw invalidParams('session_mode is one of terminal, chat', 'session_mode');
    const name = p.name === undefined ? ws.name : this.checkName(p.name, ws.name);
    const paths = ws.folders.map((f) => f.path);
    const adding = add !== undefined && !paths.includes(add);
    if (adding) {
      const { repos, recent } = this.host.admin.worktrees;
      if (!PLAIN_FOLDERS.includes(add) && ![...repos, ...recent].some((r) => r.path === add))
        throw refused('state', `${add} is not a folder`);
      if (this.list().some((w) => w.key !== ws.key && w.folders.some((f) => f.path === add)))
        throw refused('state', `${add} is another workspace's folder`);
    }
    if (gone !== undefined) {
      if (!paths.includes(gone)) throw refused('state', `${gone} is not a folder of this workspace`);
      const busy = this.agentsOf(seg).filter((a) => this.folderFor(a) === gone);
      if (busy.length) throw refused('state', `${busy.length} of its agents work in ${gone}: give them another folder first`);
    }
    const said: string[] = [];
    if (p.name !== undefined) said.push(name === ws.name ? `${name} is its name already.` : `${ws.name} is ${name} now.`);
    if (name !== ws.name) this.names.set(seg, name);
    let next = paths;
    if (adding) {
      next = [...next, add];
      said.push(`Added ${add}.`);
    }
    if (gone !== undefined) {
      next = next.filter((f) => f !== gone);
      said.push(`Took ${gone} away.`);
    }
    const moding = mode !== undefined && mode !== this.modeOf(seg);
    if (moding) {
      this.modes.set(seg, mode);
      said.push(`Its sessions run in ${mode} mode unless an agent says.`);
    }
    if (next !== paths) {
      this.folders.set(seg, next[0] ?? '');
      this.added.set(seg, next.slice(1));
    }
    if (name !== ws.name || next !== paths || moding) this.host.emit('workspaces', 'workspaces.changed', { data: {} });
    return ok(said.join(' ') || (add ? `${add} is a folder of ${ws.name} already.` : `${ws.name} runs in ${mode} mode already.`));
  }

  /** workspace.delete: every agent retired, the lead last, or nothing at all. */
  private remove(p: Record<string, unknown>, ok: Ok, actor: Actor) {
    const { seg, ws } = this.find(p.workspace);
    if (p.worktree !== undefined && p.worktree !== 'save' && p.worktree !== 'force')
      throw invalidParams('worktree is save or force', 'worktree');
    this.busy(seg);
    const agents = this.agentsOf(seg);
    const held = agents.flatMap((a) => this.host.held.get(a) ?? []);
    if (held.length && !p.worktree) throw refused('dirty', `${ws.name} has work in its worktrees.`, undefined, { worktrees: held });
    for (const a of agents.sort((x, y) => Number(x === seg) - Number(y === seg))) {
      this.host.sessions.delete(a);
      this.host.held.delete(a);
      this.host.context.texts.delete(a);
      this.host.emit('agents', 'agent.removed', { agent: a, actor, data: { key: a } });
    }
    this.names.delete(seg);
    this.folders.delete(seg);
    this.added.delete(seg);
    this.modes.delete(seg);
    for (const a of agents) this.works.delete(a);
    this.starting.delete(seg);
    this.host.emit('workspaces', 'workspaces.changed', { data: {} });
    return ok(`${ws.name} is deleted.`);
  }
}
