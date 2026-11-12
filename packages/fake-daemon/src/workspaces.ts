// The fake daemon's workspaces: workspaces.list, and the owner's workspace.create, workspace.set and workspace.delete. A workspace is the
// first segment of its agents' keys (names.ts); a rename or a folder is kept here by segment, so keys never change.
import type { Actor, Repo, Topic, Workspace } from '@handline/contract';
import { AGENTS } from '#src/fixtures.ts';
import { Failure, invalidParams, refused } from '#src/errors.ts';
import { PEOPLE, segment, workspacesOf } from '#src/names.ts';
import type { Session } from '#src/session.ts';
import type { FakeWork } from '#src/workroom.ts';
import type { FakeWorktrees } from '#src/worktrees.ts';

interface Host {
  sessions: Map<string, Session>;
  work: FakeWork;
  held: Map<string, unknown[]>;
  admin: { worktrees: FakeWorktrees };
  emit(topic: Topic, type: string, f?: { agent?: string; actor?: Actor; data?: Record<string, unknown> }): void;
}
type Ok = (said: string, extra?: object) => object;

const METHODS = /^(workspaces\.list|workspace\.(create|set|delete))$/;
const NAME = /^[a-z][a-z0-9-]{0,31}$/;
const HOME = '/Users/me';
/** Folders that are there but in no git checkout: workspace.create refuses them with gate not_git. Any other unknown folder is not_found. */
const PLAIN_FOLDERS = [`${HOME}/Documents/recipe-site`];

const notFound = (message: string, what: string) => new Failure('not_found', message, { what });

export class FakeWorkspaces {
  private names = new Map<string, string>();
  private folders = new Map<string, string>();
  /** Segments whose lead is being made: their name and folder are taken before the lead is registered. */
  private starting = new Set<string>();
  private host: Host;

  constructor(host: Host) {
    this.host = host;
  }

  owns = (method: string): boolean => METHODS.test(method);

  list(): Workspace[] {
    return workspacesOf([...this.host.sessions.keys()], this.names, this.folderOf());
  }

  /** Each segment's folder: a seeded workspace's is the repo of its name. */
  private folderOf(): Map<string, string> {
    return new Map([...this.host.admin.worktrees.repos.map((r): [string, string] => [r.name, r.path]), ...this.folders]);
  }

  private segments = () => new Set([...this.host.sessions.keys()].map(segment).concat(...this.starting));

  call(method: string, p: Record<string, unknown>, ok: Ok, actor: Actor): unknown {
    switch (method) {
      case 'workspaces.list':
        return { workspaces: this.list() };
      case 'workspace.create':
        return this.create(p, ok);
      case 'workspace.set':
        return this.rename(p, ok);
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

  private agentsOf = (seg: string) => [...this.host.sessions.keys()].filter((k) => segment(k) === seg);

  private busy(seg: string): void {
    const open = this.agentsOf(seg).filter((k) => this.host.sessions.get(k)!.open);
    if (open.length)
      throw refused('state', `${open.join(', ')} ${open.length === 1 ? 'has a session' : 'have sessions'} running: close them first.`);
  }

  /** workspace.create: the checkout the folder is in, then agent.create's operation for its lead. Another workspace's folder fails it. */
  private create(p: Record<string, unknown>, ok: Ok) {
    const name = this.checkName(p.name);
    if (typeof p.folder !== 'string' || !p.folder) throw invalidParams('folder is an absolute path', 'folder');
    const folder = p.folder.replace(/^~(?=\/|$)/, HOME);
    const { repos, recent } = this.host.admin.worktrees;
    const known = [...repos, ...recent.map((r): Repo => ({ name: r.name, path: r.path, default_branch: 'main' }))];
    const repo = known.find((r) => folder === r.path || folder.startsWith(`${r.path}/`));
    if (!repo && PLAIN_FOLDERS.includes(folder)) throw refused('not_git', `${folder} is not in a git repository`);
    if (!repo) throw notFound(`${folder} is not a folder`, 'folder');
    const segs = this.segments();
    const other = [...this.folderOf()].find(([seg, f]) => f === repo.path && segs.has(seg))?.[0];
    this.names.delete(name);
    if (!other) this.starting.add(name);
    if (!other) this.folders.set(name, repo.path);
    const fx = {
      ...AGENTS[0]!,
      name,
      role: 'lead',
      state: 'parked' as const,
      account: null,
      task: null,
      parent: null,
      worktree: `${repo.path}.${name}`,
      branch: `agent/${name}`,
      session: null,
    };
    const fails = other && `${repo.path} is the folder of workspace ${this.names.get(other) ?? other}`;
    const id = this.host.work.launch('workspace.create', fx, repo.default_branch, repo, fails);
    return ok(`Creating workspace ${name}.`, { operation: id });
  }

  /** workspace.set: the new name; refused while one of its agents has a session running. */
  private rename(p: Record<string, unknown>, ok: Ok) {
    const { seg, ws } = this.find(p.workspace);
    const name = this.checkName(p.name, ws.name);
    if (name === ws.name) return ok(`${name} is its name already.`);
    this.busy(seg);
    this.names.set(seg, name);
    this.host.emit('workspaces', 'workspaces.changed', { data: {} });
    return ok(`${ws.name} is ${name} now.`);
  }

  /** workspace.delete: every agent retired, the lead last, or nothing at all. */
  private remove(p: Record<string, unknown>, ok: Ok, actor: Actor) {
    const { seg, ws } = this.find(p.workspace);
    if (p.worktree !== undefined && p.worktree !== 'save' && p.worktree !== 'force')
      throw invalidParams('worktree is save or force', 'worktree');
    this.busy(seg);
    const agents = this.agentsOf(seg);
    const outside = [...this.host.sessions.values()].filter(
      (s) => s.fx.parent && agents.includes(s.fx.parent) && segment(s.fx.name) !== seg,
    );
    if (outside.length)
      throw refused(
        'state',
        `${outside.map((s) => s.fx.name).join(', ')} of another workspace ${outside.length === 1 ? 'is' : 'are'} under it.`,
      );
    const held = agents.flatMap((a) => this.host.held.get(a) ?? []);
    if (held.length && !p.worktree) throw refused('dirty', `${ws.name} has work in its worktrees.`, undefined, { worktrees: held });
    for (const a of agents.sort((x, y) => Number(x === seg) - Number(y === seg))) {
      this.host.sessions.delete(a);
      this.host.held.delete(a);
      this.host.emit('agents', 'agent.removed', { agent: a, actor, data: { key: a } });
    }
    this.names.delete(seg);
    this.folders.delete(seg);
    this.starting.delete(seg);
    this.host.emit('workspaces', 'workspaces.changed', { data: {} });
    return ok(`${ws.name} is deleted.`);
  }
}
