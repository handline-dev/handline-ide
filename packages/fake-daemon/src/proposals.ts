// proposals and the team: proposal.list, proposal.decide, proposal.apply_now, proposal.withdraw, team.get. Tests put proposals in with `script.proposals.raise`.
import type { Proposal } from '@handline/contract';
import { badRequest, Failure } from '#src/errors.ts';
import type { Call } from '#src/handlers.ts';
import type { Host } from '#src/script.ts';
import type { Session } from '#src/session.ts';

const WAITING = new Set(['open', 'apply_failed']);
const digest = (p: Proposal) => `d:${p.id}:${p.diff.length}:${p.reason ?? ''}`;

export class FakeProposals {
  readonly proposals: Proposal[] = [];
  /** Approving a proposal in this set fails to apply: it comes back apply_failed with this error. */
  readonly failApply = new Map<number, string>();
  /** Files edited by hand and not yet sealed: team.seal lists them in a dry run and seals them. */
  readonly unsealed = ['team.yaml: changed (it did not match its seal)', 'handline.new-builder/agent.yaml: new (no seal before)'];
  private host: Host;
  constructor(host: Host) {
    this.host = host;
  }
  /** A proposal an agent raised: stored open and announced on the proposals topic. */
  raise(p: Partial<Proposal> & { id: number; title: string; diff: string }): Proposal {
    const full: Proposal = {
      agent: 'handline.x',
      state: 'open',
      created: this.host.iso(),
      subject: '',
      requested_by: 'handline.architect',
      ...p,
    };
    full.subject ||= digest(full);
    this.proposals.push(full);
    this.host.emit('proposals', 'proposal.raised', { agent: full.agent, title: full.title, data: { proposal: full } });
    return full;
  }
  settle(p: Proposal, state: string, error: string | null = null, note?: string) {
    p.state = state;
    p.error = error;
    if (note) p.note = note;
    p.settled = this.host.iso();
    this.host.emit('proposals', 'proposal.settled', { agent: p.agent, title: p.title, data: { proposal: p } });
  }
  /** The start an approved proposal's agent was waiting for went through: its follow_up goes. False when none waited on `agent`. */
  started(agent: string): boolean {
    const p = this.proposals.find((x) => x.follow_up?.agent === agent);
    if (!p) return false;
    delete p.follow_up;
    this.host.emit('proposals', 'proposal.settled', { agent: p.agent, title: p.title, data: { proposal: p } });
    return true;
  }
  find(id: unknown): Proposal {
    const p = this.proposals.find((x) => x.id === id);
    if (!p) throw new Failure('not_found', 'no such proposal', { what: 'proposal' });
    return p;
  }
}

const gate = (g: string, msg: string, proposal: Proposal) => new Failure('refused', msg, { reason: msg, gate: g, proposal });

export const PROPOSAL_HANDLERS: Record<string, (h: Call) => unknown> = {
  'proposal.list': (h) => ({ proposals: h.script.proposals.proposals }),
  'proposal.decide': (h) => {
    const { p, script } = h;
    const book = script.proposals;
    const prop = book.find(p.proposal);
    if (p.decision !== 'approve' && p.decision !== 'reject') throw badRequest('decision is approve or reject', 'decision');
    if (p.decision === 'reject' && !(typeof p.note === 'string' && p.note.trim())) throw badRequest('a rejection needs a note', 'note');
    if (!WAITING.has(prop.state)) throw gate('proposal_settled', `the proposal is already ${prop.state}`, prop);
    if (typeof p.subject === 'string' && p.subject !== prop.subject)
      throw gate('proposal_changed', 'the proposal changed after you read it', prop);
    const note = typeof p.note === 'string' && p.note.trim() ? p.note : undefined;
    if (p.decision === 'reject') book.settle(prop, 'rejected', null, note);
    else if (book.failApply.has(prop.id)) book.settle(prop, 'apply_failed', book.failApply.get(prop.id), note);
    else book.settle(prop, 'approved', null, note);
    return script.accepted('Decided.');
  },
  'team.seal': ({ p, script }) => {
    const { unsealed } = script.proposals;
    if (p.dry_run) return { ok: true, plan: unsealed.length ? unsealed.join('\n') : 'nothing needs sealing' };
    const said = unsealed.length ? `sealed ${unsealed.length} files` : 'nothing needed sealing';
    unsealed.length = 0;
    return { ok: true, said };
  },
  // An applying proposal is applied at once: the turn it waited on is ended, so it settles approved and the wait is gone.
  'proposal.apply_now': (h) => {
    const { p, script } = h;
    const prop = script.proposals.find(p.proposal);
    if (prop.state !== 'applying') throw gate('proposal_settled', `the proposal is ${prop.state}, not waiting to be applied`, prop);
    delete prop.waiting_on;
    delete prop.waiting_since;
    script.proposals.settle(prop, 'approved');
    return script.accepted('Applied.');
  },
  'proposal.withdraw': (h) => {
    const { p, script } = h;
    const prop = script.proposals.find(p.proposal);
    if (!WAITING.has(prop.state)) throw gate('proposal_settled', `the proposal is already ${prop.state}`, prop);
    script.proposals.settle(prop, 'withdrawn');
    return script.accepted('Withdrawn.');
  },
};

/** team.get: every agent with its role and files, and the edges: a parent and its child may ask each other. */
export function teamOf(sessions: Session[], at: string) {
  const agents = sessions.map((s) => ({
    name: s.fx.name,
    role: s.fx.role,
    parent: s.fx.parent,
    role_file: { path: `${s.fx.worktree}/ROLE.md`, updated: at },
    base_file: { path: `${s.fx.worktree}/base.md`, updated: null },
  }));
  const edges = sessions.flatMap((s) =>
    s.fx.parent
      ? [
          { from: s.fx.parent, to: s.fx.name },
          { from: s.fx.name, to: s.fx.parent },
        ]
      : [],
  );
  return { agents, edges };
}
