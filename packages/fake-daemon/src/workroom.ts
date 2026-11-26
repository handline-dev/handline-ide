// The fake daemon's workroom: agent.create, a long-running operation that registers an agent, makes its worktree and starts it,
// reported on the operations topic.
import type { AgentOptions, Operation, Repo, Timestamp } from '@handline/contract';
import { AGENTS, type AgentFixture } from '#src/fixtures.ts';
import { Failure, invalidParams, refused } from '#src/errors.ts';
import { newSession, view, type Session, type TaskCounter } from '#src/session.ts';
import { EFFORTS } from '#src/settings-defaults.ts';
import type { FakeWorktrees } from '#src/worktrees.ts';

/** What the workroom needs of the daemon that hosts it: its topics, its agents and the repos and worktrees of its admin surface. */
interface Host {
  emit(topic: 'operations' | 'agents', type: string, f: { agent?: string; title?: string; data: Record<string, unknown> }): void;
  sessions: Map<string, Session>;
  tasks: TaskCounter;
  admin: { worktrees: FakeWorktrees };
  accounts: { list(): { accounts: { name: string; login_state: string }[] } };
  spaces: {
    place(name: string, workspace: unknown, folder: unknown): { name: string; repo: Repo; git: boolean };
    defaultName(workspace: unknown): string;
    list(): { key: string }[];
    settle(agent: string, folder: string): void;
  };
}
type Accepted = (said: string, extra?: object) => unknown;

const MODELS = ['fable', 'mythos', 'opus', 'sonnet', 'haiku'];
const ROLE_NOTES: Record<string, string> = {
  implementer: 'Builds a change on a branch and opens the pull request.',
  architect: 'Plans and reviews the design; does not build.',
  auditor: 'Reviews a branch before it goes up.',
};
/** A new agent's role when none is typed, as the daemon's. */
const DEFAULT_ROLE = 'Helps with anything in this workspace.';
const STEPS = ['registering the agent', 'making its worktree', 'starting the agent'];
const METHODS = /^(agent\.create|agent\.options|operation\.get)$/;

export class FakeWork {
  /** How long each step of agent.create takes; a test that wants to see a step sets it. */
  stepMs = 0;
  /** Makes each step of agent.create wait until the test calls the returned function: one call lets one step finish. */
  stepByStep(): () => void {
    this.gates = [];
    return () => this.gates?.shift()?.();
  }
  private gates: (() => void)[] | null = null;
  private ops = new Map<string, Operation>();
  private count = 0;

  private host: Host;
  private now: () => Timestamp;

  constructor(host: Host, now: () => Timestamp) {
    this.host = host;
    this.now = now;
  }

  owns = (name: string): boolean => METHODS.test(name);
  lists = (): boolean => true;

  call(method: string, p: Record<string, unknown>, accepted: Accepted): unknown {
    switch (method) {
      case 'operation.get':
        return this.operation(p.operation);
      case 'agent.options':
        return this.options();
      default:
        return this.create(p, accepted);
    }
  }

  /** agent.options: what the new-agent form offers, from the fake's repos, agents and accounts, and each workspace's default name. */
  private options(): AgentOptions {
    const { sessions, admin } = this.host;
    return {
      repos: admin.worktrees.repos.map((r) => ({ ...r })),
      roles: [...new Set([...sessions.values()].map((s) => s.fx.role))].map((r) => ({ name: r, description: ROLE_NOTES[r] ?? r })),
      models: MODELS,
      efforts: EFFORTS,
      accounts: this.host.accounts.list().accounts.map((a) => ({ name: a.name, login_state: a.login_state })),
      default_names: Object.fromEntries(this.host.spaces.list().map((w) => [w.key, this.host.spaces.defaultName(w.key)])),
    };
  }

  private operation(id: unknown): Operation {
    if (typeof id !== 'string') throw invalidParams('operation is a string', 'operation');
    const op = this.ops.get(id);
    if (!op) throw notFound(`no operation ${id}`, 'operation');
    return { ...op };
  }

  private create(p: Record<string, unknown>, accepted: Accepted) {
    const { base } = p;
    const blank = (v: unknown) => v === undefined || (typeof v === 'string' && !v.trim());
    if (p.workspace === undefined) throw invalidParams('workspace is the one the agent is made in', 'workspace');
    const typed = blank(p.name) ? this.host.spaces.defaultName(p.workspace) : p.name;
    if (typeof typed !== 'string' || !/^[^./\s]+$/.test(typed))
      throw invalidParams('name is a plain name in its workspace: no dot, no / and no spaces', 'name');
    const role = blank(p.role) ? DEFAULT_ROLE : p.role;
    if (typeof role !== 'string') throw invalidParams('role is the agent’s role, a string', 'role');
    for (const k of ['model', 'base'] as const)
      if (p[k] !== undefined && typeof p[k] !== 'string') throw invalidParams(`${k} is a string`, k);
    if (p.effort !== undefined && !EFFORTS.includes(String(p.effort)))
      throw invalidParams(`effort is one of ${EFFORTS.join(', ')}`, 'effort');
    if (p.account !== undefined) throw invalidParams('a new agent’s account is chosen when it starts; none can be named yet', 'account');
    const placed = this.host.spaces.place(typed, p.workspace, p.folder);
    const { name, repo: found } = placed;
    if (this.host.sessions.has(name)) throw refused('state', `${name} is already an agent`);
    const fx: AgentFixture = {
      ...AGENTS[0]!,
      name,
      role,
      state: 'parked',
      account: null,
      task: null,
      // a plain folder is no repo: the agent works in the folder itself, on no branch
      worktree: placed.git ? `${found.path}.${name}` : found.path,
      branch: placed.git ? `agent/${name}` : '',
      session: null,
    };
    this.host.spaces.settle(name, found.path);
    const id = this.launch('agent.create', fx, typeof base === 'string' ? base : found.default_branch, found);
    return accepted(`Creating ${name}.`, { operation: id });
  }

  /** Starts the operation that registers the agent, makes its worktree and starts it; `fails` ends it at the first step, unregistered. */
  launch(kind: Operation['kind'], fx: AgentFixture, base: string, repo: Repo, fails?: string): string {
    const id = `op-${++this.count}`;
    const op: Operation = {
      id,
      kind,
      state: 'running',
      step: null,
      steps_done: 0,
      steps_total: STEPS.length,
      message: null,
      result: null,
      agent: fx.name,
    };
    this.ops.set(id, op);
    setImmediate(() => void this.run(op, fx, base, repo, fails));
    return id;
  }

  private async run(op: Operation, fx: AgentFixture, base: string, repo: Repo, fails?: string): Promise<void> {
    const report = (o: Partial<Operation>) => {
      Object.assign(op, o);
      this.host.emit('operations', 'operation.progress', { agent: fx.name, title: `Creating ${fx.name}`, data: { ...op } });
    };
    report({ step: STEPS[0] });
    await this.wait();
    if (fails) return report({ state: 'failed', message: fails, result: { agent: fx.name, registered: false } });
    const s = newSession(fx, this.now());
    this.host.sessions.set(fx.name, s);
    this.host.emit('agents', 'agent.changed', { agent: fx.name, data: view(s, this.host.tasks) as unknown as Record<string, unknown> });
    report({ steps_done: 1, step: STEPS[1] });
    await this.wait();
    if (base !== repo.default_branch)
      return report({ state: 'failed', message: `no branch ${base} in ${repo.name}`, result: { agent: fx.name, registered: true } });
    if (fx.branch) this.addRow(fx, repo);
    report({ steps_done: 2, step: STEPS[2] });
    await this.wait();
    const up = newSession({ ...fx, state: 'idle', session: `s-${fx.name}` }, this.now());
    this.host.sessions.set(fx.name, up);
    this.host.emit('agents', 'agent.changed', { agent: fx.name, data: view(up, this.host.tasks) as unknown as Record<string, unknown> });
    report({ state: 'done', steps_done: 3, step: null, message: null, result: { agent: fx.name, registered: true } });
  }

  /** The worktree the operation made: a row of worktrees.list. */
  private addRow(fx: AgentFixture, repo: Repo): void {
    this.host.admin.worktrees.rows.push({
      path: fx.worktree,
      repo: repo.name,
      branch: fx.branch,
      owner: fx.name,
      parent: null,
      children: [],
      status: 'todo',
      changed: [],
      base: 'main',
      files_changed: 0,
      commits_ahead: 0,
      size_bytes: null,
      pr: null,
    });
  }

  private wait = (): Promise<void> => new Promise((done) => (this.gates ? this.gates.push(done) : setTimeout(done, this.stepMs)));
}

const notFound = (message: string, what: string) => new Failure('not_found', message, { what });
